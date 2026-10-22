package projection

import (
	"context"
	"sort"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/discovery/space"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// datasetSpace is a derived retrieval basis, never a Snapshot Store. Candidate
// IDs are interpreted through the fixed Dataset reader before body hydration.
type datasetSpace struct {
	pin     reader.DatasetPin
	lookup  reader.MemberLookup
	serving *reader.Serving
	member  objectmodel.Repository
	spec    retrieval.AccessSpec
	id      kernel.RepositoryID
	basis   kernel.CommitID
	// unobserved holds the binding units whose values may not enter this
	// index; they are never observed for it (see datasetCheck).
	unobserved map[string]bool
}

func (idx *Index) openDatasetSpace(lookup reader.MemberLookup, pin reader.DatasetPin) (datasetSpace, error) {
	if lookup == nil || pin.SetID == "" || pin.Revision <= 0 || len(pin.Items) == 0 || len(pin.Repositories) == 0 {
		return datasetSpace{}, kernel.Fail(kernel.ErrUsageInvalid, "search requires a published Dataset revision")
	}
	ids := make([]kernel.RepositoryID, 0, len(pin.Repositories))
	authorities := map[kernel.RepositoryID]kernel.Digest{}
	for id := range pin.Repositories {
		ids = append(ids, id)
		repo, err := lookup(id)
		if err != nil {
			return datasetSpace{}, err
		}
		if identified, ok := repo.(objectmodel.StoreIdentified); ok {
			authorities[id] = identified.StoreDigest()
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	member, err := space.Member(lookup, pin, ids[0])
	if err != nil {
		return datasetSpace{}, err
	}
	digest := kernel.CanonicalDigest([]any{pin, authorities})
	return datasetSpace{pin: pin, lookup: lookup, serving: reader.Open(lookup, pin), member: member, id: kernel.RepositoryID("dataset-index/" + pin.SetID + "/" + string(digest)), basis: kernel.CommitID(digest)}, nil
}

// PrepareDataset prepares one retrieval space with one corpus and scoring
// basis. All objects use only the selected declarations, across source repos.
func (idx *Index) PrepareDataset(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin) (ports.IndexSync, error) {
	if !pin.Temporary {
		if err := idx.recoverPublishedDatasets(ctx, pin.SetID, pin.Revision); err != nil {
			return ports.IndexSync{}, err
		}
	}
	return idx.prepareDataset(ctx, lookup, pin, false)
}

func (idx *Index) prepareDataset(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin, force bool) (ports.IndexSync, error) {
	result, _, err := idx.prepareDatasetProtected(ctx, lookup, pin, force, false)
	return result, err
}

// PrepareDatasetResource returns the same protection installed during a managed
// build, rather than reacquiring whichever generation is current afterward.
func (idx *Index) PrepareDatasetResource(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin) (ports.PreparedResource, error) {
	_, ticket, err := idx.prepareDatasetProtected(ctx, lookup, pin, false, true)
	return ticket, err
}

func (idx *Index) prepareDatasetProtected(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin, force, protect bool) (ports.IndexSync, ports.PreparedResource, error) {
	if err := ctx.Err(); err != nil {
		return ports.IndexSync{}, nil, err
	}
	s, err := idx.openDatasetSpace(lookup, pin)
	if err != nil {
		return ports.IndexSync{}, nil, err
	}
	check, units, err := idx.datasetCheck(s)
	if err != nil {
		return ports.IndexSync{}, nil, err
	}
	s.unobserved = units
	eng, err := idx.engineAt(s.id, s.basis)
	if err != nil {
		return ports.IndexSync{}, nil, err
	}
	// A prepared revision is answered from metadata alone, without the build
	// gate and without compiling the declaration contract: its corpus is fixed
	// by the pin, so nothing can move underneath it, and compiling the contract
	// costs one address resolution per referencing declaration.
	if !force {
		meta, err := loadMetaContext(ctx, eng)
		if err != nil {
			return ports.IndexSync{}, nil, err
		}
		if digest, ready := idx.readyDigestAt(eng, meta, s.basis); ready {
			return idx.reusePreparedDataset(ctx, lookup, pin, s, eng, digest, protect)
		}
	}
	var values []datasetValue
	s.spec, values, err = idx.prepareDatasetSpec(ctx, s, check)
	if err != nil {
		return ports.IndexSync{}, nil, err
	}
	release, err := idx.enterFixedBuild(ctx, engineKey{repo: s.id, commit: s.basis})
	if err != nil {
		return ports.IndexSync{}, nil, err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return ports.IndexSync{}, nil, err
	}
	// Another caller may have finished this same projection while we waited.
	meta, err := loadMetaContext(ctx, eng)
	if err != nil {
		return ports.IndexSync{}, nil, err
	}
	if !force && idx.projectionMatches(eng, meta, s.basis, s.spec.AccessDigest) {
		return idx.reusePreparedDataset(ctx, lookup, pin, s, eng, s.spec.AccessDigest, protect)
	}
	if values == nil {
		values, err = s.values(ctx, nil)
		if err != nil {
			return ports.IndexSync{}, nil, err
		}
	}
	docs := make([]ports.CompiledDoc, 0, len(values))
	for _, value := range values {
		doc, include, err := compileValueComputed(ctx, eng, s.member, value.value, s.spec)
		if err != nil {
			return ports.IndexSync{}, nil, err
		}
		if include {
			doc.SourceRepository, doc.SourceCommit = value.value.Repository, value.value.Commit
			docs = append(docs, doc)
		}
	}
	meta = idx.projectionMeta(eng, s.basis, s.spec, ports.IndexModeRebuild, ports.IndexCauseCold)
	if err := ctx.Err(); err != nil {
		return ports.IndexSync{}, nil, err
	}
	var ticket ports.PreparedResource
	if _, managed := eng.(ports.ResourcePreparer); protect && managed {
		ticket, err = buildDatasetResource(eng, docs, meta)
	} else {
		err = eng.Rebuild(docs, meta)
	}
	if err != nil {
		return ports.IndexSync{}, ticket, err
	}
	return ports.IndexSync{Repository: s.id, BasisCommit: s.basis, Mode: ports.IndexModeRebuild, Cause: ports.IndexCauseCold, AccessDigest: s.spec.AccessDigest, PhysicalDigest: meta.PhysicalDigest, ObjectCount: len(docs), Updated: len(docs)}, ticket, nil
}

// reusePreparedDataset answers an already prepared revision without rebuilding
// it, and installs the same resource protection a managed build would have.
func (idx *Index) reusePreparedDataset(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin, s datasetSpace, eng ports.Engine, digest kernel.Digest, protect bool) (ports.IndexSync, ports.PreparedResource, error) {
	sync, err := idx.readySync(s.id, s.basis, digest, eng)
	if err != nil {
		return sync, nil, err
	}
	if _, managed := eng.(ports.ResourcePreparer); protect && managed {
		ticket, err := idx.ProtectDatasetResource(ctx, lookup, pin)
		return sync, ticket, err
	}
	return sync, nil, nil
}

// SearchDataset executes one physical query on the already prepared revision.
// It never builds or scans a source, and re-reads each candidate in that scope.
func (idx *Index) SearchDataset(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin, req retrieval.SearchRequest) (retrieval.SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return retrieval.SearchResult{}, err
	}
	ctx, cancel := ports.WithSearchBudget(ctx, ports.SearchBudget{})
	defer cancel()
	nativeContinuation := ""
	if req.Continuation != "" {
		cursor, err := retrieval.DecodeContinuation(req.Continuation)
		if err != nil || cursor.Scope != "repository" {
			return retrieval.SearchResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "invalid Dataset search continuation")
		}
		if cursor.SearchView == retrieval.SearchViewDigest(retrieval.SearchView{Snapshots: pin.Repositories}) {
			nativeContinuation = cursor.Position
		}
	}
	s, eng, release, err := idx.preparedDataset(ctx, lookup, pin, datasetContinuation{positionToken: nativeContinuation})
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	defer release()
	req, empty, err := retrieval.PruneInapplicable(req, s.spec)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	if empty {
		return retrieval.SearchResult{SearchView: retrieval.SearchView{Snapshots: pin.Repositories}, Completeness: retrieval.CompletenessComplete, Hits: []retrieval.ObjectHit{}, Dataset: &retrieval.DatasetBasis{ID: pin.SetID, Revision: pin.Revision}}, nil
	}
	external, err := s.stateFields(req)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	if len(external) > 0 {
		return idx.searchDatasetState(ctx, s, req, external)
	}
	physical := s.spec
	physical.Repository = s.id
	physical.Commit = s.basis
	result, err := idx.searchEngineAtContext(ctx, nil, eng, s.basis, req, retrieval.SearchView{Snapshots: pin.Repositories}, nil, searchEngineScope{spec: physical,
		filter: s.filterCandidates,
		append: func(page ports.CandidatePage, resolved retrieval.SearchRequest, residual bool, out *retrieval.SearchResult, verifier retrieval.ResidualMatchVerifier) error {
			return s.appendPage(page, resolved, residual, out, verifier)
		},
	})
	if err == nil {
		result.Dataset = &retrieval.DatasetBasis{ID: pin.SetID, Revision: pin.Revision}
	}
	return result, err
}

func (idx *Index) preparedDataset(ctx context.Context, lookup reader.MemberLookup, pin reader.DatasetPin, continuation datasetContinuation) (datasetSpace, ports.Engine, func(), error) {
	if err := ctx.Err(); err != nil {
		return datasetSpace{}, nil, nil, err
	}
	s, err := idx.openDatasetSpace(lookup, pin)
	if err != nil {
		return s, nil, nil, err
	}
	eng, release, err := idx.acquireDatasetPublication(ctx, s, continuation)
	if err != nil {
		return s, nil, nil, err
	}
	meta, err := loadMetaContext(ctx, eng)
	if err != nil {
		release()
		return s, nil, nil, err
	}
	if meta.State != ports.ProjectionStateReady || meta.Basis != s.basis {
		release()
		return s, nil, nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset %s revision %d search is not prepared", pin.SetID, pin.Revision)
	}
	s, err = idx.validateDatasetInterpretation(s, eng, meta)
	if err != nil {
		release()
		return s, nil, nil, err
	}

	return s, eng, release, nil
}

func (s datasetSpace) filterCandidates(page ports.CandidatePage) (ports.CandidatePage, error) {
	filtered := make([]ports.CandidateRef, 0, len(page.Candidates))
	for _, candidate := range page.Candidates {
		var allowed bool
		var err error
		if candidate.Repository == s.id && candidate.Basis == s.basis {
			for id := range s.pin.Repositories {
				allowed, err = s.contains(id, candidate.ObjectID)
				if err != nil || allowed {
					break
				}
			}
		} else {
			if s.pin.Repositories[candidate.Repository] != candidate.Basis || candidate.Basis == "" {
				return ports.CandidatePage{}, kernel.Fail(kernel.ErrPreconditionFailed, "candidate is outside Dataset revision")
			}
			allowed, err = s.contains(candidate.Repository, candidate.ObjectID)
		}
		if err != nil {
			return ports.CandidatePage{}, err
		}
		if !allowed {
			return ports.CandidatePage{}, kernel.Fail(kernel.ErrPreconditionFailed, "Dataset corpus candidate %s is not held in its fixed scope", candidate.ObjectID)
		}
		filtered = append(filtered, candidate)
	}
	page.Candidates = filtered
	return page, nil
}

func (s datasetSpace) read(id objectmodel.ObjectID) (objectmodel.ObjectValue, error) {
	values := []objectmodel.ObjectValue{}
	ids := make([]kernel.RepositoryID, 0, len(s.pin.Repositories))
	for source := range s.pin.Repositories {
		ids = append(ids, source)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, source := range ids {
		member, err := space.Member(s.lookup, s.pin, source)
		if err != nil {
			return objectmodel.ObjectValue{}, err
		}
		value, err := member.Read(id, s.pin.Repositories[source])
		if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
			continue
		}
		if err != nil {
			return objectmodel.ObjectValue{}, err
		}
		values = append(values, value)
	}
	return objectmodel.ComposeHoldingValues(string(id), values)
}

func (s datasetSpace) contains(source kernel.RepositoryID, id objectmodel.ObjectID) (bool, error) {
	repo, err := s.lookup(source)
	if err != nil {
		return false, err
	}
	locator, ok := repo.(objectmodel.UnitLocator)
	if !ok {
		return false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset source cannot locate candidate units")
	}
	paths, err := locator.ObjectUnitPaths(id, s.pin.Repositories[source])
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if len(paths) == 0 {
		return false, nil
	}
	complete, err := s.serving.Contains(source, id)
	if err != nil || complete {
		return complete, err
	}
	member, err := s.serving.Member(source)
	if err != nil {
		return false, err
	}
	planner, ok := member.(objectmodel.ReferencePlanner)
	if !ok {
		return false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset source cannot plan candidate units")
	}
	addresses, err := planner.PlanReference(objectmodel.Reference{Repository: source, Object: id}, s.pin.Repositories[source])
	return len(addresses) > 0, err
}

func (s datasetSpace) appendPage(page ports.CandidatePage, req retrieval.SearchRequest, residual bool, out *retrieval.SearchResult, verifier retrieval.ResidualMatchVerifier) error {
	for _, candidate := range page.Candidates {
		value, err := s.read(candidate.ObjectID)
		if err != nil {
			return kernel.Fail(kernel.ErrPreconditionFailed, "Dataset candidate %s cannot be read at its fixed revision: %v", candidate.ObjectID, err)
		}
		out.Stats.Hydrated++
		if residual {
			matches, err := retrieval.MatchesResidual(s.member, value, nil, req, s.spec, verifier)
			if err != nil {
				return err
			}
			if !matches {
				out.Stats.Dropped++
				continue
			}
		}
		members, err := s.memberEvidence(value, req, nil)
		if err != nil {
			return err
		}
		evidence := append([]retrieval.LaneEvidence(nil), candidate.Evidence...)
		if len(evidence) == 0 {
			evidence = append(evidence, retrieval.LaneEvidence{Provider: "declarative-index", Lane: "dataset"})
		}
		evidence[0].Members = members
		out.Hits = append(out.Hits, retrieval.ObjectHit{Object: value, Version: retrieval.VersionOf(value), Evidence: evidence})
		if req.Limit > 0 && len(out.Hits) >= req.Limit {
			break
		}
	}
	return nil
}
