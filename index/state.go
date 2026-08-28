package index

import (
	"context"
	"sort"

	"kc/kernel"
	"kc/knowledge"
	knowledgemaintenance "kc/knowledge/maintenance"
	"kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
	"kc/retrieval"
)

// StateSync reports one refresh of the dynamic State projection. The
// declaration basis remains a Repository commit; Revision identifies the
// provider generation published for the accompanying observations.
type StateSync struct {
	Repository   kernel.RepositoryID         `json:"repository"`
	BasisCommit  kernel.CommitID             `json:"basisCommit"`
	Revision     string                      `json:"revision"`
	AccessDigest kernel.Digest               `json:"accessDigest"`
	Observations []knowledge.UnitObservation `json:"observations"`
	ObjectCount  int                         `json:"objectCount"`
	Updated      int                         `json:"updated"`
	Removed      int                         `json:"removed"`
	Mode         string                      `json:"mode"`
}

type stateValue struct {
	value        knowledge.KnowledgeValue
	observations []knowledge.UnitObservation
}

// stateProjection is Serving State for one published provider revision. It is
// immutable after publication and is never a Knowledge Repository.
type stateProjection struct {
	revision          string
	accessDigest      kernel.Digest
	observationDigest kernel.Digest
	observations      []knowledge.UnitObservation
	values            map[knowledge.ObjectID]stateValue
	docs              map[knowledge.ObjectID]CompiledDoc
	boundSchemas      map[knowledge.ObjectID]struct{}
}

// RequiresState reports whether the request touches AccessSpec fields supplied
// by a State/Stream Binding at the fixed commit. Static-only requests continue
// to use the Snapshot projection and do not depend on runtime availability.
func (idx *Index) RequiresState(repo knowledge.Repository, commit kernel.CommitID, req retrieval.SearchRequest) (bool, error) {
	spec, err := specAtCommit(repo, commit)
	if err != nil {
		return false, err
	}
	locator, ok := repo.(knowledge.BindingLocator)
	if !ok {
		return false, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s does not provide Binding schema location", repo.ID())
	}
	ids, err := locator.BindingSchemaObjectIDs(commit)
	if err != nil {
		return false, err
	}
	boundSchemas := make(map[knowledge.ObjectID]struct{}, len(ids))
	for _, id := range ids {
		boundSchemas[id] = struct{}{}
	}
	if len(boundSchemas) == 0 {
		return false, nil
	}
	for _, clause := range req.Clauses {
		if clause.Op == retrieval.OpMatch && clause.Path == "" && clause.Field == nil {
			for _, field := range spec.Fields {
				_, bound := boundSchemas[field.Schema]
				if bound && field.Has(reader.HintText) {
					return true, nil
				}
			}
			continue
		}
		ref := retrieval.FieldRef{Path: clause.Path}
		if clause.Field != nil {
			ref = *clause.Field
		}
		if ref.Path == "" {
			continue
		}
		field, err := spec.ResolveField(ref)
		if err != nil {
			return false, err
		}
		if _, bound := boundSchemas[field.Schema]; bound {
			return true, nil
		}
	}
	return false, nil
}

// RefreshState pulls every State Binding at a fixed declaration commit,
// compiles complete object documents through the normal compiler, and only
// then publishes both Serving State and the provider revision. A failed lookup
// leaves the previously published revision untouched.
func (idx *Index) RefreshState(ctx context.Context, repo knowledge.Repository, commit kernel.CommitID, lookup knowledgeserving.StateLookup, request knowledgeserving.RequestContext) (StateSync, error) {
	idx.stateMu.Lock()
	defer idx.stateMu.Unlock()
	if lookup == nil {
		return StateSync{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dynamic State projection requires a Materialization Runtime")
	}
	if commit == "" {
		return StateSync{}, kernel.Fail(kernel.ErrUsageInvalid, "State projection requires a declaration commit")
	}
	spec, err := specAtCommit(repo, commit)
	if err != nil {
		return StateSync{}, err
	}
	next := &stateProjection{
		accessDigest: spec.AccessDigest,
		values:       map[knowledge.ObjectID]stateValue{},
		docs:         map[knowledge.ObjectID]CompiledDoc{},
		boundSchemas: map[knowledge.ObjectID]struct{}{},
	}
	valueDigests := map[knowledge.ObjectID]kernel.Digest{}
	err = knowledgemaintenance.WalkRepository(repo, commit, func(raw knowledge.KnowledgeValue) error {
		if knowledge.IsSchemaObject(raw.Address.ObjectID) {
			return nil
		}
		hydrated, err := knowledgeserving.HydrateRepositoryValue(ctx, repo, raw, lookup, request)
		if err != nil {
			return err
		}
		value := knowledge.KnowledgeValue{
			KnowledgeRef: hydrated.KnowledgeRef, Repository: hydrated.Repository, Commit: hydrated.Commit,
			Address: hydrated.Address, Value: hydrated.Value, Provenance: hydrated.Provenance,
			Units: hydrated.Units, Declarations: hydrated.Declarations,
		}
		for _, declaration := range value.Declarations {
			if !isBindingUnit(declaration) {
				continue
			}
			if parsed, ok := knowledge.ParseSchemaRef(declaration.SchemaRef); ok {
				next.boundSchemas[parsed.Object] = struct{}{}
			}
		}
		doc, err := compileProjectionDocumentObserved(repo, value, hydrated.Observations, spec)
		if err != nil {
			return err
		}
		observations := append([]knowledge.UnitObservation(nil), hydrated.Observations...)
		next.values[value.Address.ObjectID] = stateValue{value: value, observations: observations}
		next.docs[value.Address.ObjectID] = doc
		next.observations = append(next.observations, observations...)
		valueDigests[value.Address.ObjectID] = kernel.CanonicalDigest(value.Value)
		return nil
	})
	if err != nil {
		return StateSync{}, err
	}
	sort.Slice(next.observations, func(i, j int) bool {
		return knowledge.AddressKey(next.observations[i].Address) < knowledge.AddressKey(next.observations[j].Address)
	})
	next.observationDigest = kernel.CanonicalDigest(map[string]any{
		"commit": commit, "observations": next.observations, "values": valueDigests,
	})

	eng, err := idx.stateEngineAt(repo.ID(), commit)
	if err != nil {
		return StateSync{}, err
	}
	key := engineKey{repo: repo.ID(), commit: commit, lane: "state"}
	idx.mu.Lock()
	previous := idx.states[key]
	idx.mu.Unlock()
	meta, err := eng.LoadMeta()
	if err != nil {
		return StateSync{}, err
	}
	providerMeta := projectionMeta(eng, commit, spec.AccessDigest, IndexModeRebuild, "observation")
	providerMeta.ObservationDigest = next.observationDigest
	providerMeta.Revision = string(next.observationDigest)

	mode := IndexModeRebuild
	updated, removed := len(next.docs), 0
	if previous != nil && meta.Basis == commit && meta.AccessDigest == spec.AccessDigest && physicalMatches(eng, meta) {
		upserts, deletes := diffStateDocs(previous.docs, next.docs)
		updated, removed = len(upserts), len(deletes)
		mode = IndexModeIncremental
		providerMeta.Mode = mode
		if err := eng.Apply(upserts, deletes, providerMeta); err != nil {
			return StateSync{}, err
		}
	} else {
		docs := make([]CompiledDoc, 0, len(next.docs))
		for _, doc := range next.docs {
			docs = append(docs, doc)
		}
		sort.Slice(docs, func(i, j int) bool { return docs[i].ObjectID < docs[j].ObjectID })
		if err := eng.Rebuild(docs, providerMeta); err != nil {
			return StateSync{}, err
		}
	}
	published, err := eng.LoadMeta()
	if err != nil {
		return StateSync{}, err
	}
	next.revision = published.Revision
	if next.revision == "" {
		next.revision = published.Generation
	}
	if next.revision == "" {
		next.revision = string(next.observationDigest)
	}
	idx.mu.Lock()
	idx.states[key] = next
	idx.mu.Unlock()
	return StateSync{
		Repository: repo.ID(), BasisCommit: commit, Revision: next.revision,
		AccessDigest: spec.AccessDigest, Observations: append([]knowledge.UnitObservation(nil), next.observations...),
		ObjectCount: len(next.docs), Updated: updated, Removed: removed, Mode: mode,
	}, nil
}

func diffStateDocs(previous, next map[knowledge.ObjectID]CompiledDoc) ([]CompiledDoc, []knowledge.ObjectID) {
	upserts := []CompiledDoc{}
	deletes := []knowledge.ObjectID{}
	for id, doc := range next {
		old, ok := previous[id]
		if !ok || old.ObjectDigest != doc.ObjectDigest {
			upserts = append(upserts, doc)
		}
	}
	for id := range previous {
		if _, ok := next[id]; !ok {
			deletes = append(deletes, id)
		}
	}
	sort.Slice(upserts, func(i, j int) bool { return upserts[i].ObjectID < upserts[j].ObjectID })
	sort.Slice(deletes, func(i, j int) bool { return deletes[i] < deletes[j] })
	return upserts, deletes
}

func (idx *Index) stateAt(repo kernel.RepositoryID, commit kernel.CommitID) *stateProjection {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.states[engineKey{repo: repo, commit: commit, lane: "state"}]
}

// StateView returns the basis of the currently published in-process Serving
// State. Callers use it to validate continuation before any refresh can advance
// the observation basis.
func (idx *Index) StateView(repo kernel.RepositoryID, commit kernel.CommitID) (string, []knowledge.UnitObservation, bool) {
	state := idx.stateAt(repo, commit)
	if state == nil {
		return "", nil, false
	}
	return state.revision, append([]knowledge.UnitObservation(nil), state.observations...), true
}
