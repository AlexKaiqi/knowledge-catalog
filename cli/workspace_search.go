package cli

import (
	"context"
	"fmt"
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
			revision, ok := cx.WS.Index.StateView(member.Repository, member.Commit)
			if !ok {
				return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
					"State projection for %s is not prepared; run operations projection sync", member.Repository)
			}
			if out.SearchView.ProjectionRevisions == nil {
				out.SearchView.ProjectionRevisions = map[kernel.RepositoryID]string{}
			}
			out.SearchView.ProjectionRevisions[member.Repository] = revision
		}
	}
	queryDigest := retrieval.SearchQueryDigest(req)
	viewDigest := retrieval.SearchViewDigest(out.SearchView)
	cursors := make([]workspaceSearchCursor, len(plan.Specs))
	for i, spec := range plan.Specs {
		cursors[i] = workspaceSearchCursor{spec: spec}
	}
	if req.Continuation != "" {
		state, decodeErr := retrieval.DecodeContinuation(req.Continuation)
		if decodeErr != nil || state.Scope != "workspace" || state.Query != queryDigest || state.SearchView != viewDigest || len(state.Members) != len(cursors) {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "continuation does not match this SearchView")
		}
		for i, saved := range state.Members {
			if saved.Repository != cursors[i].spec.Repository {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "continuation does not match this SearchView")
			}
			cursors[i].position = saved.Position
			cursors[i].exhausted = saved.Exhausted
		}
	}
	req.Continuation = ""
	pageLimit := req.Limit
	if pageLimit == 0 {
		pageLimit = retrieval.DefaultSearchLimit
	}

	fetchHead := func(cursor *workspaceSearchCursor) error {
		for !cursor.exhausted && cursor.head == nil {
			repo, requireErr := cx.WS.Reader.Require(cursor.spec.Repository, kernel.ErrUsageInvalid)
			if requireErr != nil {
				return requireErr
			}
			memberReq := req
			memberReq.Limit = 1
			memberReq.Continuation = cursor.position
			var member retrieval.SearchResult
			var searchErr error
			if stateMembers[cursor.spec.Repository] {
				member, searchErr = cx.WS.Index.SearchStateAtRevision(repo, cursor.spec.Commit, out.SearchView.ProjectionRevisions[cursor.spec.Repository], memberReq)
			} else {
				member, searchErr = cx.WS.Index.SearchAt(repo, cursor.spec.Commit, memberReq)
			}
			if searchErr != nil {
				if kernel.CodeOf(searchErr) == kernel.ErrCapabilityUnsatisfied {
					return kernel.Fail(kernel.ErrCapabilityUnsatisfied,
						"workspace member %s cannot satisfy SEARCH: %v; run kc describe-access --workspace %s and ensure schema/* declares the required text/filter/sort access",
						cursor.spec.Repository, searchErr, visiblePin.WorkspaceID)
				}
				return searchErr
			}
			if member.Completeness == retrieval.CompletenessPartial {
				out.Completeness = retrieval.CompletenessPartial
			}
			out.Claims = appendUniqueClaims(out.Claims, member.Claims...)
			if len(member.Hits) > 1 {
				return kernel.Fail(kernel.ErrPreconditionFailed, "member search ignored limit=1")
			}
			if len(member.Hits) == 0 {
				if member.Continuation == "" {
					cursor.exhausted = true
					return nil
				}
				if member.Continuation == cursor.position {
					return kernel.Fail(kernel.ErrPreconditionFailed, "member search returned a non-advancing continuation")
				}
				cursor.position = member.Continuation
				continue
			}
			hit := member.Hits[0]
			cursor.nextPosition = member.Continuation
			cursor.exhaustAfterHead = member.Continuation == ""
			if !allowedRepoRead(cx.Home, cx.Flags, string(hit.Knowledge.Repository), string(hit.Knowledge.Address.ObjectID)) {
				cursor.position = cursor.nextPosition
				cursor.exhausted = cursor.exhaustAfterHead
				continue
			}
			if !stateMembers[cursor.spec.Repository] {
				hit, searchErr = hydrateSearchHit(cx.Context, logical, hit)
				if searchErr != nil {
					return searchErr
				}
			}
			cursor.head = &hit
		}
		return nil
	}

	for i := range cursors {
		if err := fetchHead(&cursors[i]); err != nil {
			return nil, err
		}
	}
	for len(out.Hits) < pageLimit {
		best := bestWorkspaceHead(cursors, req)
		if best < 0 {
			break
		}
		cursor := &cursors[best]
		out.Hits = append(out.Hits, *cursor.head)
		cursor.head = nil
		cursor.position = cursor.nextPosition
		cursor.exhausted = cursor.exhaustAfterHead
		if len(out.Hits) < pageLimit {
			if err := fetchHead(cursor); err != nil {
				return nil, err
			}
		}
	}
	if workspaceSearchHasMore(cursors) {
		members := make([]retrieval.MemberContinuation, len(cursors))
		for i, cursor := range cursors {
			members[i] = retrieval.MemberContinuation{
				Repository: cursor.spec.Repository, Position: cursor.position, Exhausted: cursor.exhausted,
			}
		}
		out.Continuation = retrieval.EncodeContinuation(retrieval.ContinuationState{
			Scope: "workspace", Query: queryDigest, SearchView: viewDigest, Members: members,
		})
	}
	return out, nil
}

type workspaceSearchCursor struct {
	spec             retrieval.AccessSpec
	position         string
	exhausted        bool
	head             *retrieval.KnowledgeHit
	nextPosition     string
	exhaustAfterHead bool
}

func workspaceSearchHasMore(cursors []workspaceSearchCursor) bool {
	for _, cursor := range cursors {
		if cursor.head != nil || !cursor.exhausted {
			return true
		}
	}
	return false
}

func bestWorkspaceHead(cursors []workspaceSearchCursor, req retrieval.SearchRequest) int {
	best := -1
	for i := range cursors {
		if cursors[i].head == nil {
			continue
		}
		if best < 0 || workspaceHitLess(*cursors[i].head, *cursors[best].head, req) {
			best = i
		}
	}
	return best
}

func workspaceHitLess(left, right retrieval.KnowledgeHit, req retrieval.SearchRequest) bool {
	order, sorted := workspaceSortOrder(req)
	if sorted {
		leftValue, leftOK := providerOrderValue(left)
		rightValue, rightOK := providerOrderValue(right)
		if leftOK != rightOK {
			return leftOK // missing values are last for both asc and desc
		}
		if leftOK {
			if cmp := compareOrderValue(leftValue, rightValue); cmp != 0 {
				if order == "desc" {
					return cmp > 0
				}
				return cmp < 0
			}
		}
	} else if workspaceHasMatch(req) {
		leftRank, rightRank := localRank(left), localRank(right)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
	}
	if left.Knowledge.Repository != right.Knowledge.Repository {
		return left.Knowledge.Repository < right.Knowledge.Repository
	}
	return left.Knowledge.Address.ObjectID < right.Knowledge.Address.ObjectID
}

func workspaceSortOrder(req retrieval.SearchRequest) (string, bool) {
	for _, clause := range req.Clauses {
		if clause.Op == retrieval.OpSort {
			order := strings.ToLower(strings.TrimSpace(clause.Order))
			if order == "" {
				order = "asc"
			}
			return order, true
		}
	}
	return "", false
}

func workspaceHasMatch(req retrieval.SearchRequest) bool {
	for _, clause := range req.Clauses {
		if clause.Op == retrieval.OpMatch {
			return true
		}
	}
	return false
}

func providerOrderValue(hit retrieval.KnowledgeHit) (any, bool) {
	for _, evidence := range hit.Evidence {
		if len(evidence.ProviderOrder) > 0 && evidence.ProviderOrder[0] != nil {
			return evidence.ProviderOrder[0], true
		}
	}
	return nil, false
}

func localRank(hit retrieval.KnowledgeHit) int {
	for _, evidence := range hit.Evidence {
		if evidence.LocalRank > 0 {
			return evidence.LocalRank
		}
	}
	return int(^uint(0) >> 1)
}

func compareOrderValue(left, right any) int {
	if leftNumber, ok := orderNumber(left); ok {
		if rightNumber, rightOK := orderNumber(right); rightOK {
			switch {
			case leftNumber < rightNumber:
				return -1
			case leftNumber > rightNumber:
				return 1
			default:
				return 0
			}
		}
	}
	leftText, rightText := fmt.Sprint(left), fmt.Sprint(right)
	return strings.Compare(leftText, rightText)
}

func orderNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case int:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case float32:
		return float64(number), true
	case float64:
		return number, true
	default:
		return 0, false
	}
}

func appendUniqueClaims(existing []string, claims ...string) []string {
	for _, claim := range claims {
		seen := false
		for _, current := range existing {
			if current == claim {
				seen = true
				break
			}
		}
		if !seen {
			existing = append(existing, claim)
		}
	}
	return existing
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
