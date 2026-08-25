package cli

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/reader"
)

func searchWorkspace(ws *Home, home string, flags map[string]FlagValue) (any, error) {
	serving, cat, err := openServing(ws, flags)
	if err != nil {
		return nil, err
	}
	visiblePin, omitted := searchVisiblePin(home, flags, serving.Pin())
	if len(visiblePin.Repositories) == 0 {
		return nil, kernel.Fail(kernel.ErrForbidden, "workspace search has no authorized members")
	}
	plan, err := reader.PlanAccess(knowledge.Lookup(cat.Require), visiblePin)
	if err != nil {
		return nil, err
	}
	req, err := searchRequestFromFlags(flags)
	if err != nil {
		return nil, err
	}
	out := reader.SearchResult{
		View:         reader.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{}},
		Completeness: reader.CompletenessComplete, Hits: []reader.KnowledgeHit{},
	}
	if omitted > 0 {
		out.Completeness = reader.CompletenessPartial
		out.Claims = append(out.Claims, "some workspace members were omitted by authorization")
	}
	for _, spec := range plan.Specs {
		out.View.Snapshots[spec.Repository] = spec.Commit
	}
	queryDigest := reader.SearchQueryDigest(req)
	viewDigest := reader.SearchViewDigest(out.View)
	startMember := 0
	memberContinuation := ""
	if req.Continuation != "" {
		state, err := reader.DecodeContinuation(req.Continuation)
		if err != nil || state.Scope != "workspace" || state.Query != queryDigest || state.View != viewDigest || state.Member < 0 || state.Member >= len(plan.Specs) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "continuation does not match this search view")
		}
		startMember = state.Member
		memberContinuation = state.Position
	}
	req.Continuation = ""
	tried, unsat := 0, 0
	for memberIndex := startMember; memberIndex < len(plan.Specs); memberIndex++ {
		spec := plan.Specs[memberIndex]
		repo, err := knowledge.Require(ws.Store, spec.Repository, kernel.ErrUsageInvalid)
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
			member, err := ws.Index.SearchAt(repo, spec.Commit, memberReq)
			if err != nil {
				if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
					unsat++
					out.Completeness = reader.CompletenessPartial
					out.Claims = append(out.Claims, "member does not satisfy search: "+string(spec.Repository))
					memberContinuation = ""
					break
				}
				return nil, err
			}
			if member.Completeness == reader.CompletenessPartial {
				out.Completeness = reader.CompletenessPartial
			}
			out.Claims = append(out.Claims, member.Claims...)
			for _, hit := range member.Hits {
				if allowedRepoRead(home, flags, string(hit.Knowledge.Repository), string(hit.Knowledge.Address.ObjectID)) {
					out.Hits = append(out.Hits, hit)
				}
			}
			memberContinuation = member.Continuation
			if req.Limit > 0 && len(out.Hits) >= req.Limit {
				nextMember := memberIndex
				if memberContinuation == "" {
					nextMember++
				}
				if nextMember < len(plan.Specs) {
					out.Continuation = reader.EncodeContinuation(reader.ContinuationState{
						Scope: "workspace", Query: queryDigest, View: viewDigest,
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
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"no authorized member index satisfies this search; run kc describe-access --workspace %s and ensure schema/* declares the required text/filter/sort access",
			visiblePin.WorkspaceID)
	}
	return out, nil
}
