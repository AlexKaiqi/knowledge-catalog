package index

import (
	"context"

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
	Repository        kernel.RepositoryID `json:"repository"`
	BasisCommit       kernel.CommitID     `json:"basisCommit"`
	Revision          string              `json:"revision"`
	AccessDigest      kernel.Digest       `json:"accessDigest"`
	ObservationDigest kernel.Digest       `json:"observationDigest"`
	ObjectCount       int                 `json:"objectCount"`
	Updated           int                 `json:"updated"`
	Removed           int                 `json:"removed"`
	Mode              string              `json:"mode"`
}

// stateProjection is Serving State for one published provider revision. It is
// immutable after publication and is never a Knowledge Repository.
type stateProjection struct {
	revision          string
	accessDigest      kernel.Digest
	observationDigest kernel.Digest
	store             *stateServingStore
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
	for _, clause := range retrieval.SearchClauses(req) {
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
	idx.stateBuildMu.Lock()
	defer idx.stateBuildMu.Unlock()
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
	key := stateStoreKey(repo.ID(), commit)
	store, err := openStateServingStore(idx.dir, key, "building")
	if err != nil {
		return StateSync{}, err
	}
	keepStore := false
	defer func() {
		if !keepStore {
			_ = store.CloseAndRemove()
		}
	}()
	next := &stateProjection{accessDigest: spec.AccessDigest, store: store}
	const batchSize = 500
	records := make(map[knowledge.ObjectID]stateRecord, batchSize)
	count := 0
	flushRecords := func() error {
		if len(records) == 0 {
			return nil
		}
		if err := store.PutBatch(records); err != nil {
			return err
		}
		clear(records)
		return nil
	}
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
		doc, err := compileProjectionDocumentObserved(repo, value, hydrated.Observations, spec)
		if err != nil {
			return err
		}
		records[value.Address.ObjectID] = stateRecord{
			Value: value, Observations: append([]knowledge.UnitObservation(nil), hydrated.Observations...), Doc: doc,
		}
		count++
		if len(records) == batchSize {
			return flushRecords()
		}
		return nil
	})
	if err == nil {
		err = flushRecords()
	}
	if err != nil {
		return StateSync{}, err
	}
	next.observationDigest, err = store.Digest(commit)
	if err != nil {
		return StateSync{}, err
	}
	next.revision = string(next.observationDigest)

	eng, err := idx.stateEngineAt(repo.ID(), commit)
	if err != nil {
		return StateSync{}, err
	}
	providerMeta := projectionMeta(eng, commit, spec.AccessDigest, IndexModeRebuild, "observation")
	providerMeta.ObservationDigest = next.observationDigest
	providerMeta.Revision = next.revision
	streaming, ok := eng.(StreamingProjectionMaintainer)
	if !ok {
		return StateSync{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"dynamic State projection requires bounded streaming rebuild support")
	}
	session, err := streaming.BeginRebuild(providerMeta)
	if err != nil {
		return StateSync{}, err
	}
	if err := store.WalkDocs(batchSize, session.Append); err != nil {
		_ = session.Abort(err)
		return StateSync{}, err
	}

	// Searches continue using the old READY generation while compilation runs.
	// The short publication critical section switches provider and Serving State
	// together, so a request never combines revisions.
	idx.stateMu.Lock()
	if err := session.Commit(); err != nil {
		idx.stateMu.Unlock()
		return StateSync{}, err
	}
	previous := idx.states[key]
	idx.states[key] = next
	keepStore = true
	idx.stateMu.Unlock()
	if previous != nil {
		_ = previous.store.CloseAndRemove()
	}
	return StateSync{
		Repository: repo.ID(), BasisCommit: commit, Revision: next.revision,
		AccessDigest: spec.AccessDigest, ObservationDigest: next.observationDigest,
		ObjectCount: count, Updated: count, Mode: IndexModeRebuild,
	}, nil
}

// StateView returns the basis of the currently published in-process Serving
// State. Callers use it to validate continuation before any refresh can advance
// the observation basis.
func (idx *Index) StateView(repo kernel.RepositoryID, commit kernel.CommitID) (string, bool) {
	idx.stateMu.RLock()
	defer idx.stateMu.RUnlock()
	state := idx.states[stateStoreKey(repo, commit)]
	if state == nil {
		return "", false
	}
	return state.revision, true
}
