package projection

import (
	"context"
	ports "kc/discovery/projection"
	"os"

	retrieval "kc/discovery"
	"kc/kernel"
	objectmodel "kc/object"
	objectmaintenance "kc/object/maintenance"
	"kc/object/reader"
)

// stateProjection is Serving State for one published provider revision. It is
// immutable after publication and is never an Object Repository.
type stateProjection struct {
	revision          string
	accessDigest      kernel.Digest
	observationDigest kernel.Digest
	store             *stateServingStore
}

// RequiresState reports whether the request touches AccessSpec fields supplied
// by a State/Stream Binding at the fixed commit. BindingLocator is optional:
// a Snapshot-only Repository (including System) is not a State query.
// Static-only requests continue to use the Snapshot projection and do not
// depend on runtime availability.
func (idx *Index) RequiresState(repo objectmodel.Repository, commit kernel.CommitID, req retrieval.SearchRequest) (bool, error) {
	locator, ok := repo.(objectmodel.BindingLocator)
	if !ok {
		return false, nil
	}
	ids, err := locator.BindingSchemaObjectIDs(commit)
	if err != nil {
		return false, err
	}
	boundSchemas := make(map[objectmodel.ObjectID]struct{}, len(ids))
	for _, id := range ids {
		boundSchemas[id] = struct{}{}
	}
	if len(boundSchemas) == 0 {
		return false, nil
	}
	spec, maintained, err := idx.maintainedAccessSpecAt(repo, commit)
	if err != nil {
		return false, err
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
		if err != nil && !maintained {
			// No ready projection retained the declarations yet: this is not
			// "undeclared". The Snapshot search reports the projection as not
			// prepared instead.
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if _, bound := boundSchemas[field.Schema]; bound {
			return true, nil
		}
	}
	return false, nil
}

// RefreshState enumerates every object at the declaration commit and pulls
// Bound State. Use RefreshStateObjects after a change notice so only affected
// Addresses hit the runtime. A failed lookup leaves the published revision.
func (idx *Index) RefreshState(ctx context.Context, repo objectmodel.Repository, commit kernel.CommitID, source ports.ObservationSource) (ports.StateSync, error) {
	return idx.refreshState(ctx, repo, commit, source, nil)
}

// RefreshStateObjects pulls the named objects. With no published Serving State
// the call falls back to a full enumerate (cold start).
func (idx *Index) RefreshStateObjects(ctx context.Context, repo objectmodel.Repository, commit kernel.CommitID, source ports.ObservationSource, objectIDs []objectmodel.ObjectID) (ports.StateSync, error) {
	return idx.refreshState(ctx, repo, commit, source, objectIDs)
}

func (idx *Index) HasStateBindings(repo objectmodel.Repository, commit kernel.CommitID) (bool, error) {
	locator, ok := repo.(objectmodel.BindingLocator)
	if !ok {
		return false, nil
	}
	ids, err := locator.BindingSchemaObjectIDs(commit)
	if err != nil {
		return false, err
	}
	return len(ids) > 0, nil
}

func (idx *Index) refreshState(ctx context.Context, repo objectmodel.Repository, commit kernel.CommitID, source ports.ObservationSource, objectIDs []objectmodel.ObjectID) (ports.StateSync, error) {
	idx.stateBuildMu.Lock()
	defer idx.stateBuildMu.Unlock()
	if source == nil {
		return ports.StateSync{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dynamic State projection requires a Materialization Runtime")
	}
	if commit == "" {
		return ports.StateSync{}, kernel.Fail(kernel.ErrUsageInvalid, "State projection requires a declaration commit")
	}
	spec, err := idx.specAtCommit(repo, commit)
	if err != nil {
		return ports.StateSync{}, err
	}
	key := stateStoreKey(repo.ID(), commit)
	idx.stateMu.RLock()
	previous := idx.states[key]
	idx.stateMu.RUnlock()
	incremental := len(objectIDs) > 0 && previous != nil
	store, err := openStateServingStore(idx.dir, key, "building")
	if err != nil {
		return ports.StateSync{}, err
	}
	keepStore := false
	defer func() {
		if !keepStore {
			_ = store.CloseAndRemove()
		}
	}()
	next := &stateProjection{accessDigest: spec.AccessDigest, store: store}
	const batchSize = 500
	records := make(map[objectmodel.ObjectID]stateRecord, batchSize)
	count := 0
	updated := 0
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
	hydrate := func(raw objectmodel.ObjectValue) error {
		if objectmodel.IsSchemaObject(raw.Address.ObjectID) {
			return nil
		}
		hydrated, err := source.ObserveRepositoryValue(ctx, repo, raw)
		if err != nil {
			return err
		}
		value := objectmodel.ObjectValue{
			ObjectRef: hydrated.ObjectRef, Repository: hydrated.Repository, Commit: hydrated.Commit,
			Address: hydrated.Address, Value: hydrated.Value, Provenance: hydrated.Provenance,
			Units: hydrated.Units, Declarations: hydrated.Declarations,
		}
		if err := objectmodel.ValidateHydratedObject(repo.ID(), commit, raw.Address.ObjectID, value); err != nil {
			return err
		}
		doc, err := compileProjectionDocumentObserved(repo, value, hydrated.Observations, spec)
		if err != nil {
			return err
		}
		records[value.Address.ObjectID] = stateRecord{
			Value: value, Observations: append([]objectmodel.UnitObservation(nil), hydrated.Observations...), Doc: doc,
		}
		if incremental {
			if _, found, getErr := previous.store.Get(raw.Address.ObjectID); getErr != nil {
				return getErr
			} else if !found {
				count++
			}
		} else {
			count++
		}
		updated++
		if len(records) == batchSize {
			return flushRecords()
		}
		return nil
	}
	if incremental {
		if err = previous.store.WalkRecords(batchSize, func(batch map[objectmodel.ObjectID]stateRecord) error {
			count += len(batch)
			return store.PutBatch(batch)
		}); err != nil {
			return ports.StateSync{}, err
		}
		seen := map[objectmodel.ObjectID]struct{}{}
		for _, id := range objectIDs {
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			raw, readErr := repo.Read(id, commit)
			if readErr != nil {
				err = readErr
				break
			}
			if err = hydrate(raw); err != nil {
				break
			}
		}
	} else {
		err = objectmaintenance.WalkRepository(repo, commit, hydrate)
	}
	if err == nil {
		err = flushRecords()
	}
	if err != nil {
		return ports.StateSync{}, err
	}
	next.observationDigest, err = store.Digest(commit)
	if err != nil {
		return ports.StateSync{}, err
	}
	next.revision = string(next.observationDigest)

	eng, err := idx.stateEngineAt(authorityEngineID(repo), commit)
	if err != nil {
		return ports.StateSync{}, err
	}
	mode := ports.IndexModeRebuild
	if incremental {
		mode = ports.IndexModeIncremental
	}
	providerMeta := idx.projectionMeta(eng, commit, spec, mode, "observation")
	providerMeta.ObservationDigest = next.observationDigest
	providerMeta.Revision = next.revision
	streaming, ok := eng.(ports.StreamingProjectionMaintainer)
	if !ok {
		return ports.StateSync{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"dynamic State projection requires bounded streaming rebuild support")
	}
	session, err := streaming.BeginRebuild(providerMeta)
	if err != nil {
		return ports.StateSync{}, err
	}
	if err := store.WalkDocs(batchSize, session.Append); err != nil {
		_ = session.Abort(err)
		return ports.StateSync{}, err
	}

	// Searches continue using the old READY generation while compilation runs.
	// The short publication critical section switches provider and Serving State
	// together, so a request never combines revisions.
	idx.stateMu.Lock()
	if err := session.Commit(); err != nil {
		idx.stateMu.Unlock()
		return ports.StateSync{}, err
	}
	previous = idx.states[key]
	idx.states[key] = next
	keepStore = true
	idx.stateMu.Unlock()
	if previous != nil {
		_ = previous.store.CloseAndRemove()
	}
	return ports.StateSync{
		Repository: repo.ID(), BasisCommit: commit, Revision: next.revision,
		AccessDigest: spec.AccessDigest, ObservationDigest: next.observationDigest,
		ObjectCount: count, Updated: updated, Mode: mode,
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

// StateProjectionReady observes the published Serving State and its physical
// projection without retrieving candidates, hydrating, or refreshing values.
// An absent medium is recoverable; provider and local I/O errors remain errors.
func (idx *Index) StateProjectionReady(ctx context.Context, repo objectmodel.Repository, commit kernel.CommitID) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if commit == "" {
		return false, kernel.Fail(kernel.ErrUsageInvalid, "State projection readiness requires an explicit fixed commit")
	}
	idx.stateMu.RLock()
	defer idx.stateMu.RUnlock()
	state := idx.states[stateStoreKey(repo.ID(), commit)]
	if state == nil || state.store == nil || state.store.db == nil {
		return false, nil
	}
	info, err := os.Stat(state.store.file)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	eng, err := idx.stateEngineAt(authorityEngineID(repo), commit)
	if err != nil {
		return false, err
	}
	meta, err := loadMetaContext(ctx, eng)
	if err != nil {
		return false, err
	}
	return idx.stateProjectionMatches(eng, meta, commit, state), nil
}

// stateProjectionMatches checks one provider metadata read against the
// immutable resident observation. Query and maintenance share this condition.
func (idx *Index) stateProjectionMatches(eng ports.Engine, meta ports.Meta, commit kernel.CommitID, state *stateProjection) bool {
	return state != nil && meta.State == ports.ProjectionStateReady && idx.projectionMatches(eng, meta, commit, state.accessDigest) &&
		meta.Revision == state.revision && meta.ObservationDigest == state.observationDigest
}
