package cli

import (
	"context"
	"strings"
	"sync"
	"time"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
	"kc/retrieval"
)

func searchWorkspace(cx *invocation) (any, error) {
	ctx, cancel := index.WithSearchBudget(cx.Context, index.SearchBudget{})
	defer cancel()
	serving, cat, err := openServing(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	logical, err := logicalWorkspaceServing(cx, serving)
	if err != nil {
		return nil, err
	}
	visiblePin, _ := searchVisiblePin(cx.Home, cx.Flags, serving.Pin())
	if len(visiblePin.Repositories) == 0 {
		return nil, kernel.Fail(kernel.ErrForbidden, "workspace has no members")
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
					"State projection for %s is not prepared", member.Repository)
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
		if clause, sorted := retrieval.SearchSortClause(req); sorted {
			resolved, err := retrieval.ResolveSearchClause(clause, spec)
			if err != nil {
				return nil, err
			}
			field, err := spec.ResolveField(*resolved.Field)
			if err != nil {
				return nil, err
			}
			cursors[i].sortType = workspaceScalarType(field.Type)
			if i > 0 && cursors[i].sortType != cursors[0].sortType {
				return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Workspace SORT fields have incompatible logical types")
			}
		}
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
			if saved.Offset < 0 {
				return nil, kernel.Fail(kernel.ErrPreconditionFailed, "invalid member continuation offset")
			}
			cursors[i].offset = saved.Offset
			cursors[i].position = saved.Position
			cursors[i].exhausted = saved.Exhausted
		}
	}
	initialMembers := workspaceMemberContinuations(cursors)
	req.Continuation = ""
	pageLimit := req.Limit
	if pageLimit == 0 {
		pageLimit = retrieval.DefaultSearchLimit
	}

	fetch := func(ctx context.Context, cursor *workspaceSearchCursor) (retrieval.SearchResult, error) {
		repo, err := cx.WS.Reader.Require(cursor.spec.Repository, kernel.ErrUsageInvalid)
		if err != nil {
			return retrieval.SearchResult{}, err
		}
		memberReq := req
		memberReq.Limit = cursor.batchLimit()
		memberReq.Continuation = cursor.position
		var member retrieval.SearchResult
		if stateMembers[cursor.spec.Repository] {
			member, err = cx.WS.Index.SearchStateAtRevisionContext(ctx, repo, cursor.spec.Commit, out.SearchView.ProjectionRevisions[cursor.spec.Repository], memberReq)
		} else {
			member, err = cx.WS.Index.SearchAtContext(ctx, repo, cursor.spec.Commit, memberReq)
		}
		if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
			return member, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "workspace member %s cannot satisfy SEARCH: %v; schema/* must declare the required text/filter/sort access", cursor.spec.Repository, err)
		}
		return member, err
	}
	if err := primeWorkspaceHeads(ctx, cursors, fetch); err != nil {
		return nil, err
	}
	for len(out.Hits) < pageLimit {
		if ctx.Err() != nil {
			if !index.SearchBudgetExhausted(ctx) {
				return nil, ctx.Err()
			}
			out.Completeness = retrieval.CompletenessPartial
			out.Stats.MarkPartial("budget")
			out.Claims = appendUniqueClaims(out.Claims, "search execution budget exhausted: time")
			break
		}
		unknownHead := false
		for i := range cursors {
			if cursors[i].blocked {
				unknownHead = true
				break
			}
		}
		if unknownHead {
			break
		}
		best := bestWorkspaceHead(cursors, req)
		if best < 0 {
			break
		}
		cursor := &cursors[best]
		hit := *cursor.head
		if !stateMembers[cursor.spec.Repository] {
			started := time.Now()
			var err error
			hit, err = hydrateSearchHit(ctx, logical, hit)
			out.Stats.HydrateDuration += time.Since(started)
			if index.SearchBudgetExhausted(ctx) {
				out.Completeness = retrieval.CompletenessPartial
				out.Stats.MarkPartial("budget")
				out.Claims = appendUniqueClaims(out.Claims, "search execution budget exhausted: time")
				break
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if err != nil {
				return nil, err
			}
		}
		hit.Knowledge.Repository = cursor.spec.Repository
		hit.Knowledge.KnowledgeRef.Repository = cursor.spec.Repository
		if hit.Knowledge.KnowledgeRef.Object == "" {
			hit.Knowledge.KnowledgeRef.Object = hit.Knowledge.Address.ObjectID
		}
		hit, err = deliverSearchHit(cx.Home, cx.Flags, hit)
		if err != nil {
			return nil, err
		}
		out.Hits = append(out.Hits, hit)
		cursor.consumeHead()
		if len(out.Hits) < pageLimit {
			if err := fillWorkspaceHead(ctx, cursor, fetch); err != nil {
				return nil, err
			}
		}
	}
	for i := range cursors {
		cursor := &cursors[i]
		out.Stats.Add(cursor.stats)
		out.Claims = appendUniqueClaims(out.Claims, cursor.claims...)
		if cursor.partial {
			out.Completeness = retrieval.CompletenessPartial
		}
	}
	if workspaceSearchHasMore(cursors) {
		members := workspaceMemberContinuations(cursors)
		if len(out.Hits) == 0 && !workspaceMembersAdvanced(initialMembers, members) {
			return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "Workspace search cannot make paging progress within the execution budget; increase the execution budget or narrow the Workspace")
		}
		out.Continuation = retrieval.EncodeContinuation(retrieval.ContinuationState{
			Scope: "workspace", Query: queryDigest, SearchView: viewDigest, Members: members,
		})
	}
	return out, nil
}

const workspaceSearchBatchSize = 16
const workspaceSearchConcurrency = 4

type workspaceSearchCursor struct {
	initialLimit int
	spec         retrieval.AccessSpec
	position     string
	offset       int
	exhausted    bool
	head         *retrieval.KnowledgeHit
	buffer       []retrieval.KnowledgeHit
	nextPosition string
	sortType     string
	blocked      bool
	partial      bool
	stats        retrieval.SearchExecutionStats
	claims       []string
}

// Initial probes reserve the replayed offset plus one unread head, bounded
// by the batch size. Fresh members need one candidate; refills keep the batch.
func (c *workspaceSearchCursor) batchLimit() int {
	if c.initialLimit > 0 {
		return c.initialLimit
	}
	return workspaceSearchBatchSize
}

func workspaceMemberContinuations(cursors []workspaceSearchCursor) []retrieval.MemberContinuation {
	members := make([]retrieval.MemberContinuation, len(cursors))
	for i, cursor := range cursors {
		members[i] = retrieval.MemberContinuation{Repository: cursor.spec.Repository, Position: cursor.position, Offset: cursor.offset, Exhausted: cursor.exhausted}
	}
	return members
}

// An empty page is resumable only if something persisted in the token moved:
// a source position, completed member, or reduced buffered-page replay debt.
// Merely wrapping the starting position in a repository token is not progress.
func workspaceMembersAdvanced(before, after []retrieval.MemberContinuation) bool {
	if len(before) != len(after) {
		return false
	}
	for i, previous := range before {
		next := after[i]
		if previous.Repository != next.Repository || previous.Exhausted {
			continue
		}
		if next.Exhausted || next.Offset < previous.Offset || workspaceProviderPosition(previous.Position) != workspaceProviderPosition(next.Position) {
			return true
		}
	}
	return false
}

func workspaceProviderPosition(position string) string {
	if state, err := retrieval.DecodeContinuation(position); err == nil && state.Scope == "repository" {
		return state.Position
	}
	return position
}

type workspacePageFetcher func(context.Context, *workspaceSearchCursor) (retrieval.SearchResult, error)

func (c *workspaceSearchCursor) consumeHead() {
	c.head = nil
	c.offset++
	if c.offset >= len(c.buffer) {
		c.position = c.nextPosition
		c.exhausted = c.nextPosition == ""
		c.offset = 0
		c.buffer = nil
	}
}

func fillWorkspaceHead(ctx context.Context, c *workspaceSearchCursor, fetch workspacePageFetcher) error {
	for !c.exhausted && c.head == nil && !c.blocked {
		if c.buffer != nil {
			c.head = &c.buffer[c.offset]
			return nil
		}
		if ctx.Err() != nil {
			if !index.SearchBudgetExhausted(ctx) {
				return ctx.Err()
			}
			c.partial, c.blocked = true, true
			c.stats.MarkPartial("budget")
			c.claims = appendUniqueClaims(c.claims, "search execution budget exhausted: time")
			return nil
		}
		member, err := fetch(ctx, c)
		if err != nil {
			return err
		}
		c.stats.Add(member.Stats)
		c.claims = appendUniqueClaims(c.claims, member.Claims...)
		c.partial = c.partial || member.Completeness == retrieval.CompletenessPartial
		if len(member.Hits) > c.batchLimit() {
			return kernel.Fail(kernel.ErrPreconditionFailed, "member search exceeded its batch limit")
		}
		for _, hit := range member.Hits {
			if value, ok := providerOrderValue(hit); ok && c.sortType != "" {
				if _, valid := retrieval.NormalizeScalarValue(c.sortType, value); !valid {
					return kernel.Fail(kernel.ErrPreconditionFailed, "member returned invalid typed SORT value")
				}
			}
		}
		if len(member.Hits) <= c.offset {
			skipped := len(member.Hits)
			if member.Continuation == "" {
				if c.offset > skipped {
					return kernel.Fail(kernel.ErrPreconditionFailed, "member continuation offset exceeds available hits")
				}
				c.offset = 0
				c.exhausted = true
				return nil
			}
			if member.Continuation == c.position && member.Completeness != retrieval.CompletenessPartial {
				return kernel.Fail(kernel.ErrPreconditionFailed, "member search returned a non-advancing continuation")
			}
			c.offset -= skipped
			c.position = member.Continuation
			if member.Completeness == retrieval.CompletenessPartial {
				c.blocked = true
				return nil
			}
			continue
		}
		c.buffer = member.Hits
		c.nextPosition = member.Continuation
		c.head = &c.buffer[c.offset]
	}
	return nil
}

func primeWorkspaceHeads(ctx context.Context, cursors []workspaceSearchCursor, fetch workspacePageFetcher) error {
	for i := range cursors {
		cursors[i].initialLimit = min(cursors[i].offset, workspaceSearchBatchSize-1) + 1
	}
	defer func() {
		for i := range cursors {
			cursors[i].initialLimit = 0
		}
	}()
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int, len(cursors))
	for i := range cursors {
		jobs <- i
	}
	close(jobs)
	errors := make([]error, len(cursors))
	var workers sync.WaitGroup
	for n := 0; n < workspaceSearchConcurrency && n < len(cursors); n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				errors[i] = fillWorkspaceHead(workCtx, &cursors[i], fetch)
				if errors[i] != nil {
					cancel()
				}
			}
		}()
	}
	workers.Wait()
	// Prefer the triggering failure to cancellation propagated to other members.
	for _, err := range errors {
		if err != nil && err != context.Canceled {
			return err
		}
	}
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}

func workspaceScalarType(fieldType string) string {
	switch strings.ToLower(strings.TrimSpace(fieldType)) {
	case "int", "integer", "long":
		return "long"
	case "number", "float", "double":
		return "number"
	case "bool", "boolean":
		return "boolean"
	case "datetime", "timestamp":
		return "timestamp"
	case "":
		return "string"
	default:
		return strings.ToLower(strings.TrimSpace(fieldType))
	}
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
		if best < 0 || workspaceHitLess(*cursors[i].head, *cursors[best].head, req, cursors[i].sortType) {
			best = i
		}
	}
	return best
}

func workspaceHitLess(left, right retrieval.KnowledgeHit, req retrieval.SearchRequest, fieldTypes ...string) bool {
	fieldType := "string"
	if len(fieldTypes) > 0 {
		fieldType = fieldTypes[0]
	}
	order, sorted := workspaceSortOrder(req)
	if sorted {
		leftValue, leftOK := providerOrderValue(left)
		rightValue, rightOK := providerOrderValue(right)
		if leftOK != rightOK {
			return leftOK // missing values are last for both asc and desc
		}
		if leftOK {
			if cmp, err := retrieval.CompareScalarValues(fieldType, leftValue, rightValue); err == nil && cmp != 0 {
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
	clause, ok := retrieval.SearchSortClause(req)
	if !ok {
		return "", false
	}
	order := strings.ToLower(strings.TrimSpace(clause.Order))
	if order == "" {
		order = "asc"
	}
	return order, true
}

func workspaceHasMatch(req retrieval.SearchRequest) bool {
	return retrieval.SearchHasOp(req, retrieval.OpMatch)
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
