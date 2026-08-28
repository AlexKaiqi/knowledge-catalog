package cli

import (
	"context"
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
	"kc/retrieval"
)

func searchWorkspace(cx *invocation) (any, error) {
	serving, cat, err := openServing(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	logical, err := logicalWorkspaceServing(cx, serving)
	if err != nil {
		return nil, err
	}
	visiblePin, omitted := searchVisiblePin(cx.Home, cx.Flags, serving.Pin())
	if len(visiblePin.Repositories) == 0 {
		return nil, kernel.Fail(kernel.ErrForbidden, "workspace search has no authorized members")
	}
	plan, err := retrieval.PlanAccess(cx.WS.Reader.Lookup(cat.Require), visiblePin)
	if err != nil {
		return nil, err
	}
	req, err := searchRequestFromFlags(cx.Flags)
	if err != nil {
		return nil, err
	}
	out := retrieval.SearchResult{
		SearchView:   retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{}},
		Completeness: retrieval.CompletenessComplete, Hits: []retrieval.KnowledgeHit{},
	}
	if omitted > 0 {
		out.Completeness = retrieval.CompletenessPartial
		out.Claims = append(out.Claims, "some workspace members were omitted by authorization")
	}
	for _, spec := range plan.Specs {
		out.SearchView.Snapshots[spec.Repository] = spec.Commit
	}
	stateMembers := map[kernel.RepositoryID]bool{}
	{
		for _, member := range plan.Specs {
			repo, err := cx.WS.Reader.Require(member.Repository, kernel.ErrUsageInvalid)
			if err != nil {
				return nil, err
			}
			required, err := cx.WS.Index.RequiresState(repo, member.Commit, req)
			if err != nil {
				return nil, err
			}
			if !required {
				continue
			}
			stateMembers[member.Repository] = true
			revision, observations, ok := cx.WS.Index.StateView(member.Repository, member.Commit)
			if !ok {
				return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
					"State projection for %s is not prepared; run operations projection sync", member.Repository)
			}
			if out.SearchView.ProjectionRevisions == nil {
				out.SearchView.ProjectionRevisions = map[kernel.RepositoryID]string{}
				out.SearchView.Observations = map[kernel.RepositoryID][]knowledge.UnitObservation{}
			}
			out.SearchView.ProjectionRevisions[member.Repository] = revision
			out.SearchView.Observations[member.Repository] = observations
		}
	}
	queryDigest := retrieval.SearchQueryDigest(req)
	viewDigest := retrieval.SearchViewDigest(out.SearchView)
	startMember := 0
	memberContinuation := ""
	if req.Continuation != "" {
		state, err := retrieval.DecodeContinuation(req.Continuation)
		if err != nil || state.Scope != "workspace" || state.Query != queryDigest || state.SearchView != viewDigest || state.Member < 0 || state.Member >= len(plan.Specs) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "continuation does not match this SearchView")
		}
		startMember = state.Member
		memberContinuation = state.Position
	}
	req.Continuation = ""
	tried, unsat := 0, 0
	var firstUnsatisfied error
	for memberIndex := startMember; memberIndex < len(plan.Specs); memberIndex++ {
		spec := plan.Specs[memberIndex]
		repo, err := cx.WS.Reader.Require(spec.Repository, kernel.ErrUsageInvalid)
		if err != nil {
			return nil, err
		}
		for {
			memberReq := req
			memberReq.Continuation = memberContinuation
			if req.Limit > 0 {
				memberReq.Limit = req.Limit - len(out.Hits)
				if memberReq.Limit <= 0 {
					break
				}
			}
			tried++
			var member retrieval.SearchResult
			if stateMembers[spec.Repository] {
				member, err = cx.WS.Index.SearchStateAtRevision(repo, spec.Commit, out.SearchView.ProjectionRevisions[spec.Repository], memberReq)
			} else {
				member, err = cx.WS.Index.SearchAt(repo, spec.Commit, memberReq)
			}
			if err != nil {
				if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
					if firstUnsatisfied == nil {
						firstUnsatisfied = err
					}
					unsat++
					out.Completeness = retrieval.CompletenessPartial
					out.Claims = append(out.Claims, "member does not satisfy search: "+string(spec.Repository))
					memberContinuation = ""
					break
				}
				return nil, err
			}
			if member.Completeness == retrieval.CompletenessPartial {
				out.Completeness = retrieval.CompletenessPartial
			}
			out.Claims = append(out.Claims, member.Claims...)
			for _, hit := range member.Hits {
				if allowedRepoRead(cx.Home, cx.Flags, string(hit.Knowledge.Repository), string(hit.Knowledge.Address.ObjectID)) {
					if stateMembers[spec.Repository] {
						out.Hits = append(out.Hits, hit)
					} else {
						hydrated, err := hydrateSearchHit(cx.Context, logical, hit)
						if err != nil {
							return nil, err
						}
						out.Hits = append(out.Hits, hydrated)
					}
				}
			}
			memberContinuation = member.Continuation
			if req.Limit > 0 && len(out.Hits) >= req.Limit {
				nextMember := memberIndex
				if memberContinuation == "" {
					nextMember++
				}
				if nextMember < len(plan.Specs) {
					out.Continuation = retrieval.EncodeContinuation(retrieval.ContinuationState{
						Scope: "workspace", Query: queryDigest, SearchView: viewDigest,
						Member: nextMember, Position: memberContinuation,
					})
				}
				return out, nil
			}
			if memberContinuation == "" {
				break
			}
		}
		memberContinuation = ""
	}
	if tried > 0 && unsat == tried {
		if firstUnsatisfied != nil {
			if strings.Contains(firstUnsatisfied.Error(), "SEARCH requires an OpenSearch projection") || strings.Contains(firstUnsatisfied.Error(), "State Binding fields require") {
				return nil, firstUnsatisfied
			}
		}
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"no authorized member index satisfies this search; run kc describe-access --workspace %s and ensure schema/* declares the required text/filter/sort access",
			visiblePin.WorkspaceID)
	}
	return out, nil
}

func logicalWorkspaceServing(cx *invocation, declarations *reader.Serving) (*knowledgeserving.Service, error) {
	request, err := stateRequestContextFrom(cx)
	if err != nil {
		return nil, err
	}
	return knowledgeserving.OpenRequest(declarations, cx.State, request), nil
}

func stateRequestContextFrom(cx *invocation) (knowledgeserving.RequestContext, error) {
	identity, err := identityContextFrom(cx.Flags)
	if err != nil {
		return knowledgeserving.RequestContext{}, err
	}
	trace, err := traceContextFrom(cx.Flags)
	if err != nil {
		return knowledgeserving.RequestContext{}, err
	}
	requestID, err := requestIDFrom(cx.Flags)
	if err != nil {
		return knowledgeserving.RequestContext{}, err
	}
	return knowledgeserving.RequestContext{
		Identity: identity, Trace: trace, RequestID: requestID,
	}, nil
}

func hydrateSearchHit(ctx context.Context, logical *knowledgeserving.Service, hit retrieval.KnowledgeHit) (retrieval.KnowledgeHit, error) {
	raw := hit.Knowledge
	federated := reader.FederatedValue{
		KnowledgeRef: raw.KnowledgeRef, Repository: raw.Repository, Commit: raw.Commit,
		ObjectID: raw.Address.ObjectID, Address: raw.Address, Value: raw.Value,
		Provenance: raw.Provenance, Units: raw.Units, Declarations: raw.Declarations,
	}
	hydrated, err := logical.Hydrate(ctx, federated, nil)
	if err != nil {
		return retrieval.KnowledgeHit{}, err
	}
	hit.Knowledge.Value = hydrated.Value
	hit.Version.Observations = append([]knowledge.UnitObservation(nil), hydrated.Observations...)
	return hit, nil
}
