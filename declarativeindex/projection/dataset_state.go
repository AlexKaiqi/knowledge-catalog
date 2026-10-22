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

// stateFields lists the request's condition fields whose values come from a
// binding, that is the conditions answered from external observations.
func (s datasetSpace) stateFields(request retrieval.SearchRequest) ([]retrieval.FieldRef, error) {
	resolved, err := retrieval.ResolveSearch(request, s.spec)
	if err != nil {
		return nil, err
	}
	bound := map[objectmodel.ObjectID]bool{}
	for id, at := range s.pin.Repositories {
		member, err := space.Member(s.lookup, s.pin, id)
		if err != nil {
			return nil, err
		}
		locator := member.(objectmodel.BindingLocator)
		ids, err := locator.BindingSchemaObjectIDs(at)
		if err != nil {
			return nil, err
		}
		for _, schema := range ids {
			bound[schema] = true
		}
	}
	var fields []retrieval.FieldRef
	seen := map[string]bool{}
	add := func(field retrieval.FieldRef) {
		if bound[field.Schema] && !seen[field.Key()] {
			seen[field.Key()] = true
			fields = append(fields, field)
		}
	}
	for _, clause := range retrieval.SearchClauses(resolved) {
		if clause.Op == retrieval.OpMatch && clause.Path == "" && clause.Field == nil {
			for _, field := range s.spec.Fields {
				if field.Has(reader.HintText) {
					add(field.FieldRef)
				}
			}
			continue
		}
		field, err := s.spec.ResolveClauseField(clause)
		if err != nil {
			return nil, err
		}
		add(field.FieldRef)
	}
	return fields, nil
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
	s, _, release, err := idx.preparedDataset(ctx, lookup, pin, datasetContinuation{})
	if err != nil {
		return ports.StateSync{}, err
	}
	release()
	eng, err := idx.stateEngineAt(s.id, s.basis)
	if err != nil {
		return ports.StateSync{}, err
	}
	var publishObservation func(func() (ports.PreparedResource, error)) error
	_, managed := eng.(ports.ResourcePreparer)
	if idx.observationPrepare != nil && !pin.Temporary && managed {
		publishObservation, err = idx.observationPrepare(ctx, pin.SetID, pin.Revision)
		if err != nil {
			return ports.StateSync{}, err
		}
		if publishObservation == nil {
			return ports.StateSync{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset authority returned no observation acceptance")
		}
	}
	values, err := s.values(ctx, source)
	if err != nil {
		return ports.StateSync{}, err
	}
	key := stateStoreKey(s.id, s.basis)
	store, err := idx.openObservationStore(key)
	if err != nil {
		return ports.StateSync{}, err
	}
	keep := false
	defer func() {
		if !keep {
			if store.sealed {
				_ = store.release()
			} else {
				_ = store.CloseAndRemove()
			}
		}
	}()
	records := map[objectmodel.ObjectID]stateRecord{}
	oldest := oldestObservation(values)
	for _, value := range values {
		doc, err := compileObservedValue(ctx, eng, s.member, value.value, value.observations, s.spec)
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
	next := &stateProjection{authority: s.id, store: store, accessDigest: s.spec.AccessDigest, observationDigest: digest, revision: string(digest), oldestObservedAt: oldest}
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
	if err := store.seal(key, s.id, next); err != nil {
		_ = session.Abort(err)
		return ports.StateSync{}, err
	}

	// A managed publication is accepted by the actual Dataset authority after
	// preparation protection. The serving swap follows acceptance, never ROOT.
	var previous *stateProjection
	attempted := false
	prepare := func() (ports.PreparedResource, error) {
		attempted = true
		if _, managed := eng.(ports.ResourcePreparer); managed {
			protected, ok := session.(ports.PreparedRebuildSession)
			if !ok {
				return nil, session.Abort(kernel.Fail(kernel.ErrCapabilityUnsatisfied, "managed observation build requires continuous resource protection"))
			}
			ticket, err := protected.CommitResource()
			if err == nil && ticket == nil {
				err = kernel.Fail(kernel.ErrCapabilityUnsatisfied, "managed observation build returned no resource ticket")
			}
			return ticket, err
		}
		return nil, session.Commit()
	}
	accept := func() error {
		if publishObservation != nil {
			if err := publishObservation(prepare); err != nil {
				return err
			}
		} else {
			if err := session.Commit(); err != nil {
				attempted = true
				return err
			}
			attempted = true
		}
		idx.stateMu.Lock()
		previous = idx.states[key]
		idx.states[key] = next
		keep = true
		idx.stateMu.Unlock()
		return nil
	}
	if err := idx.withCurrentDataset(pin, accept); err != nil {
		if !attempted {
			_ = session.Abort(err)
		}
		return ports.StateSync{}, err
	}
	if previous != nil {
		_ = previous.store.release()
	}
	return ports.StateSync{Repository: s.id, BasisCommit: s.basis, Revision: next.revision, AccessDigest: s.spec.AccessDigest, ObservationDigest: digest, ObjectCount: len(records), Updated: len(records), Mode: ports.IndexModeRebuild}, nil
}

func (idx *Index) requireCurrentDataset(pin reader.DatasetPin) error {
	return idx.withCurrentDataset(pin, nil)
}

func (idx *Index) withCurrentDataset(pin reader.DatasetPin, accept func() error) error {
	if idx.datasetRevisionFence == nil {
		if accept == nil {
			return nil
		}
		return accept()
	}
	ran := false
	err := idx.datasetRevisionFence(pin.SetID, pin.Revision, func() error {
		ran = true
		if accept == nil {
			return nil
		}
		return accept()
	})
	if err != nil && !ran && kernel.CodeOf(err) == kernel.ErrPreconditionFailed {
		return kernel.Fail(kernel.ErrPreconditionFailed, "only the current Dataset revision may refresh external observations: %v", err)
	}
	return err
}

func (idx *Index) searchDatasetState(ctx context.Context, s datasetSpace, request retrieval.SearchRequest, external []retrieval.FieldRef) (retrieval.SearchResult, error) {
	eng, state, release, err := idx.acquireObservation(ctx, stateStoreKey(s.id, s.basis), s.id, request, &s)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	defer release()
	if state == nil {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset %s revision %d observations are not prepared", s.pin.SetID, s.pin.Revision)
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
		for _, field := range external {
			result.ExternalConditions = append(result.ExternalConditions, retrieval.ExternalCondition{Field: field, OldestObservedAt: state.oldestObservedAt})
		}
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
		hit, failures, checked, err := ports.RecheckCandidate(ctx, hit)
		if err != nil {
			return err
		}
		if checked {
			conditional, err := s.conditionUnitFailed(request, failures)
			if err != nil {
				return err
			}
			// A failed reread is neither a match nor a business miss
			// (discovery#U1): every failed unit is named beside the result.
			for _, failure := range failures {
				out.Claims = append(out.Claims, retrieval.UnitFailureClaim(failure))
			}
			if conditional {
				// The match itself is undecided: the candidate leaves the page
				// and the answer can no longer claim to be complete.
				out.Stats.Dropped++
				out.Completeness = retrieval.CompletenessPartial
				out.Stats.MarkPartial("hydrate")
				continue
			}
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

// conditionUnitFailed reports whether a reread failure touches an aspect the
// request's conditions read; a failure elsewhere cannot change the match.
func (s datasetSpace) conditionUnitFailed(request retrieval.SearchRequest, failures []objectmodel.UnitReadFailure) (bool, error) {
	if len(failures) == 0 {
		return false, nil
	}
	resolved, err := retrieval.ResolveSearch(request, s.spec)
	if err != nil {
		return false, err
	}
	aspects := map[string]bool{}
	for _, clause := range retrieval.SearchClauses(resolved) {
		if clause.Op == retrieval.OpMatch && clause.Path == "" && clause.Field == nil {
			for _, field := range s.spec.Fields {
				if field.Has(reader.HintText) {
					aspects[field.Aspect] = true
				}
			}
			continue
		}
		field, err := s.spec.ResolveClauseField(clause)
		if err != nil {
			return false, err
		}
		aspects[field.Aspect] = true
	}
	for _, failure := range failures {
		if aspects[failure.Address.AspectName] {
			return true, nil
		}
	}
	return false, nil
}
