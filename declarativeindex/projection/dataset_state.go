package projection

import (
	"context"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/discovery/space"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

func (s datasetSpace) requiresState(request retrieval.SearchRequest) (bool, error) {
	resolved, err := retrieval.ResolveSearch(request, s.spec)
	if err != nil {
		return false, err
	}
	bound := map[objectmodel.ObjectID]bool{}
	for id, at := range s.pin.Repositories {
		member, err := space.Member(s.lookup, s.pin, id)
		if err != nil {
			return false, err
		}
		locator := member.(objectmodel.BindingLocator)
		ids, err := locator.BindingSchemaObjectIDs(at)
		if err != nil {
			return false, err
		}
		for _, schema := range ids {
			bound[schema] = true
		}
	}
	for _, clause := range retrieval.SearchClauses(resolved) {
		if clause.Op == retrieval.OpMatch && clause.Path == "" && clause.Field == nil {
			for _, field := range s.spec.Fields {
				if bound[field.Schema] && field.Has(reader.HintText) {
					return true, nil
				}
			}
			continue
		}
		field, err := s.spec.ResolveClauseField(clause)
		if err != nil {
			return false, err
		}
		if bound[field.Schema] {
			return true, nil
		}
	}
	return false, nil
}

// RefreshDatasetState prepares the observation space of one Dataset revision.
// Observations are held outside its frozen file and declaration authority.
func (idx *Index) RefreshDatasetState(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin, source ports.ObservationSource) (ports.StateSync, error) {
	if err := ctx.Err(); err != nil {
		return ports.StateSync{}, err
	}
	if source == nil {
		return ports.StateSync{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset observations require a runtime")
	}
	if err := idx.requireCurrentDataset(pin); err != nil {
		return ports.StateSync{}, err
	}
	idx.stateBuildMu.Lock()
	defer idx.stateBuildMu.Unlock()
	if err := ctx.Err(); err != nil {
		return ports.StateSync{}, err
	}
	if err := idx.requireCurrentDataset(pin); err != nil {
		return ports.StateSync{}, err
	}
	s, _, release, err := idx.preparedDataset(ctx, lookup, pin)
	if err != nil {
		return ports.StateSync{}, err
	}
	release()
	values, err := s.values(ctx, source)
	if err != nil {
		return ports.StateSync{}, err
	}
	key := stateStoreKey(s.id, s.basis)
	store, err := openStateServingStore(idx.dir, key, "building")
	if err != nil {
		return ports.StateSync{}, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = store.CloseAndRemove()
		}
	}()
	records := map[objectmodel.ObjectID]stateRecord{}
	for _, value := range values {
		doc, err := compileProjectionDocumentObserved(s.member, value.value, value.observations, s.spec)
		if err != nil {
			return ports.StateSync{}, err
		}
		doc.SourceRepository, doc.SourceCommit = value.value.Repository, value.value.Commit
		records[value.value.Address.ObjectID] = stateRecord{Value: value.value, Observations: value.observations, Doc: doc}
	}
	if err := store.PutBatch(records); err != nil {
		return ports.StateSync{}, err
	}
	digest, err := store.Digest(s.basis)
	if err != nil {
		return ports.StateSync{}, err
	}
	next := &stateProjection{store: store, accessDigest: s.spec.AccessDigest, observationDigest: digest, revision: string(digest)}
	eng, err := idx.stateEngineAt(s.id, s.basis)
	if err != nil {
		return ports.StateSync{}, err
	}
	meta := idx.projectionMeta(eng, s.basis, s.spec, ports.IndexModeRebuild, "observation")
	meta.ObservationDigest = digest
	meta.Revision = next.revision
	streaming, ok := eng.(ports.StreamingProjectionMaintainer)
	if !ok {
		return ports.StateSync{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset observations require streaming publication")
	}
	session, err := streaming.BeginRebuild(meta)
	if err != nil {
		return ports.StateSync{}, err
	}
	if err := store.WalkDocs(500, session.Append); err != nil {
		_ = session.Abort(err)
		return ports.StateSync{}, err
	}
	if err := ctx.Err(); err != nil {
		_ = session.Abort(err)
		return ports.StateSync{}, err
	}
	idx.stateMu.Lock()
	if err := idx.requireCurrentDataset(pin); err != nil {
		idx.stateMu.Unlock()
		_ = session.Abort(err)
		return ports.StateSync{}, err
	}
	if err := session.Commit(); err != nil {
		idx.stateMu.Unlock()
		return ports.StateSync{}, err
	}
	previous := idx.states[key]
	idx.states[key] = next
	keep = true
	idx.stateMu.Unlock()
	if previous != nil {
		_ = previous.store.CloseAndRemove()
	}
	return ports.StateSync{Repository: s.id, BasisCommit: s.basis, Revision: next.revision, AccessDigest: s.spec.AccessDigest, ObservationDigest: digest, ObjectCount: len(records), Updated: len(records), Mode: ports.IndexModeRebuild}, nil
}

func (idx *Index) requireCurrentDataset(pin reader.DatasetPin) error {
	if idx.currentDatasetRevision == nil {
		return nil
	}
	revision, err := idx.currentDatasetRevision(pin.SetID)
	if err != nil {
		return err
	}
	if revision != pin.Revision {
		return kernel.Fail(kernel.ErrPreconditionFailed, "only the current Dataset revision may refresh external observations")
	}
	return nil
}

func (idx *Index) searchDatasetState(ctx context.Context, s datasetSpace, request retrieval.SearchRequest) (retrieval.SearchResult, error) {
	idx.stateMu.RLock()
	defer idx.stateMu.RUnlock()
	state := idx.states[stateStoreKey(s.id, s.basis)]
	if state == nil {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset %s revision %d observations are not prepared", s.pin.SetID, s.pin.Revision)
	}
	eng, err := idx.stateEngineAt(s.id, s.basis)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	meta, err := loadMetaContext(ctx, eng)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	if !idx.stateProjectionMatches(eng, meta, s.basis, state) {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "Dataset observation basis changed")
	}
	physical := s.spec
	physical.Repository = s.id
	physical.Commit = s.basis
	view := retrieval.SearchView{Snapshots: s.pin.Repositories, ProjectionRevisions: map[kernel.RepositoryID]string{}}
	for id := range s.pin.Repositories {
		view.ProjectionRevisions[id] = state.revision
	}
	result, err := idx.searchEngineAtContext(ctx, nil, eng, s.basis, request, view, nil, searchEngineScope{spec: physical, filter: s.filterCandidates,
		append: func(page ports.CandidatePage, req retrieval.SearchRequest, residual bool, out *retrieval.SearchResult, verifier retrieval.ResidualMatchVerifier) error {
			return s.appendStatePage(ctx, state, page, req, residual, out, verifier)
		},
	})
	if err == nil {
		result.Dataset = &retrieval.DatasetBasis{ID: s.pin.SetID, Revision: s.pin.Revision}
	}
	return result, err
}

func (s datasetSpace) appendStatePage(ctx context.Context, state *stateProjection, page ports.CandidatePage, request retrieval.SearchRequest, residual bool, out *retrieval.SearchResult, verifier retrieval.ResidualMatchVerifier) error {
	for _, candidate := range page.Candidates {
		record, found, err := state.store.Get(candidate.ObjectID)
		if err != nil {
			return err
		}
		if !found {
			return kernel.Fail(kernel.ErrPreconditionFailed, "Dataset candidate has no retained observation")
		}
		out.Stats.Hydrated++
		if residual {
			matched, err := retrieval.MatchesResidual(s.member, record.Value, record.Observations, request, s.spec, verifier)
			if err != nil {
				return err
			}
			if !matched {
				out.Stats.Dropped++
				continue
			}
		}
		version := retrieval.VersionOf(record.Value)
		version.Observations = record.Observations
		members, err := s.memberEvidence(record.Value, request, record.Observations)
		if err != nil {
			return err
		}
		evidence := append([]retrieval.LaneEvidence(nil), candidate.Evidence...)
		if len(evidence) == 0 {
			evidence = append(evidence, retrieval.LaneEvidence{Provider: "declarative-index", Lane: "dataset-state"})
		}
		evidence[0].Members = members
		hit := retrieval.ObjectHit{Object: record.Value, Version: version, Evidence: evidence, IndexObservations: record.Observations}
		hit, checked, err := ports.RecheckCandidate(ctx, hit)
		if err != nil {
			return err
		}
		if checked {
			matched, err := retrieval.MatchesResidual(s.member, hit.Object, hit.Version.Observations, request, s.spec, verifier)
			if err != nil {
				return err
			}
			if !matched {
				out.Stats.Dropped++
				continue
			}
		}
		out.Hits = append(out.Hits, hit)
		if request.Limit > 0 && len(out.Hits) >= request.Limit {
			break
		}
	}
	return nil
}
