package knowledgeapp

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

// DatasetSearchExecutor owns authorization, fixed-basis resolution, bounded
// federated retrieval, same-scope hydration, and delivery.
type DatasetSearchExecutor struct {
	Authorize    func(context.Context) error
	Resolve      func(context.Context) (*reader.Serving, *knowledgeserving.Service, error)
	Repositories RepositoryLookup
	Projection   SearchProjection
	Deliver      func(context.Context, retrieval.KnowledgeHit) (retrieval.KnowledgeHit, error)
	// Embedder serves the request-time query vector for semantic recall
	// (RETRIEVAL.md §8.1). Nil keeps dataset semantic recall fail-closed.
	Embedder retrieval.Embedder
}

// datasetSearchRun carries one bounded federated search execution: the
// resolved serving basis and access contract, plus the merge state that the
// paging loop advances.
type datasetSearchRun struct {
	exec   DatasetSearchExecutor
	req    retrieval.SearchRequest
	ctx    context.Context
	cancel context.CancelFunc

	serving *reader.Serving
	logical *knowledgeserving.Service
	pin     reader.KnowledgeSetPin
	plan    retrieval.AccessPlan
	out     retrieval.SearchResult

	stateMembers   map[kernel.RepositoryID]bool
	cursors        []workspaceSearchCursor
	queryDigest    kernel.Digest
	viewDigest     kernel.Digest
	pageLimit      int
	initialMembers []retrieval.MemberContinuation
}

func (e DatasetSearchExecutor) Execute(ctx context.Context, req retrieval.SearchRequest) (retrieval.SearchResult, error) {
	if e.Authorize == nil || e.Resolve == nil || e.Repositories == nil || e.Projection == nil || e.Deliver == nil {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset search services are incomplete")
	}
	if req.Recall == retrieval.RecallSemantic {
		return e.executeSemantic(ctx, req)
	}
	if err := e.Authorize(ctx); err != nil {
		return retrieval.SearchResult{}, err
	}
	run, err := e.beginSearchRun(ctx, req)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	defer run.cancel()
	if err := run.checkAccessContract(); err != nil {
		return retrieval.SearchResult{}, err
	}
	if err := run.resolveStateMembers(); err != nil {
		return retrieval.SearchResult{}, err
	}
	if err := run.initCursors(); err != nil {
		return retrieval.SearchResult{}, err
	}
	if err := run.restoreContinuation(); err != nil {
		return retrieval.SearchResult{}, err
	}
	if err := run.mergePage(); err != nil {
		return retrieval.SearchResult{}, err
	}
	return run.finish()
}

// beginSearchRun resolves the serving pair, applies the execution budget and
// the frozen dataset candidate scope, and plans member access on the pin.
func (e DatasetSearchExecutor) beginSearchRun(ctx context.Context, req retrieval.SearchRequest) (*datasetSearchRun, error) {
	serving, logical, err := e.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	if serving == nil || logical == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset serving is unavailable")
	}
	budgetCtx, cancel := index.WithSearchBudget(ctx, index.SearchBudget{})
	pin := serving.Pin()
	budgetCtx = index.WithCandidateScope(budgetCtx, kernel.CanonicalDigest(pin.Items), func(id kernel.RepositoryID, at kernel.CommitID, object knowledge.ObjectID) (bool, error) {
		if pin.Repositories[id] != at {
			return false, kernel.Fail(kernel.ErrPreconditionFailed, "candidate is outside dataset basis")
		}
		return serving.Contains(id, object)
	})
	if len(pin.Repositories) == 0 {
		cancel()
		return nil, kernel.Fail(kernel.ErrForbidden, "dataset has no members")
	}
	plan, err := retrieval.PlanAccess(func(id kernel.RepositoryID) (knowledge.Repository, error) {
		return e.Repositories.Require(id, kernel.ErrKnowledgeRefUnresolved)
	}, pin)
	if err != nil {
		cancel()
		return nil, err
	}
	return &datasetSearchRun{
		exec: e, req: req, ctx: budgetCtx, cancel: cancel,
		serving: serving, logical: logical, pin: pin, plan: plan,
		out: retrieval.SearchResult{
			SearchView:   retrieval.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{}},
			Completeness: retrieval.CompletenessComplete, Hits: []retrieval.KnowledgeHit{},
		},
	}, nil
}

// checkAccessContract verifies every member's SEARCH capability and records
// the snapshot each member is searched at.
func (r *datasetSearchRun) checkAccessContract() error {
	for _, spec := range r.plan.Specs {
		if err := retrieval.CheckSearch(r.req, spec); err != nil {
			if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
				return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "workspace member %s cannot satisfy SEARCH: %v; schema/* must declare the required text/filter/sort access within the dataset", spec.Repository, err)
			}
			return err
		}
		r.out.SearchView.Snapshots[spec.Repository] = spec.Commit
	}
	return nil
}

// resolveStateMembers determines which members must be answered from the
// prepared State projection and records their revisions.
func (r *datasetSearchRun) resolveStateMembers() error {
	r.stateMembers = map[kernel.RepositoryID]bool{}
	for _, member := range r.plan.Specs {
		repo, err := r.serving.Member(member.Repository)
		if err != nil {
			return err
		}
		required, err := r.exec.Projection.RequiresState(repo, member.Commit, r.req)
		if err != nil {
			return err
		}
		if !required {
			continue
		}
		r.stateMembers[member.Repository] = true
		revision, ok := r.exec.Projection.StateView(member.Repository, member.Commit)
		if !ok {
			return kernel.Fail(kernel.ErrCapabilityUnsatisfied,
				"State projection for %s is not prepared", member.Repository)
		}
		if r.out.SearchView.ProjectionRevisions == nil {
			r.out.SearchView.ProjectionRevisions = map[kernel.RepositoryID]string{}
		}
		r.out.SearchView.ProjectionRevisions[member.Repository] = revision
	}
	return nil
}

// initCursors opens one paging cursor per member and rejects SORT fields of
// incompatible logical types across members.
func (r *datasetSearchRun) initCursors() error {
	r.queryDigest = retrieval.SearchQueryDigest(r.req)
	r.viewDigest = kernel.CanonicalDigest([]any{retrieval.SearchViewDigest(r.out.SearchView), r.pin.Items})
	r.cursors = make([]workspaceSearchCursor, len(r.plan.Specs))
	for i, spec := range r.plan.Specs {
		r.cursors[i] = workspaceSearchCursor{spec: spec}
		if clause, sorted := retrieval.SearchSortClause(r.req); sorted {
			resolved, err := retrieval.ResolveSearchClause(clause, spec)
			if err != nil {
				return err
			}
			field, err := spec.ResolveField(*resolved.Field)
			if err != nil {
				return err
			}
			r.cursors[i].sortType = workspaceScalarType(field.Type)
			if i > 0 && r.cursors[i].sortType != r.cursors[0].sortType {
				return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Workspace SORT fields have incompatible logical types")
			}
		}
	}
	r.pageLimit = r.req.Limit
	if r.pageLimit == 0 {
		r.pageLimit = retrieval.DefaultSearchLimit
	}
	return nil
}

// restoreContinuation replays a workspace continuation token onto the member
// cursors, rejecting tokens that do not match this SearchView; the incoming
// token is cleared so member fetches start from the replayed positions.
func (r *datasetSearchRun) restoreContinuation() error {
	if r.req.Continuation != "" {
		state, decodeErr := retrieval.DecodeContinuation(r.req.Continuation)
		if decodeErr != nil || state.Scope != "workspace" || state.Query != r.queryDigest || state.SearchView != r.viewDigest || len(state.Members) != len(r.cursors) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "continuation does not match this SearchView")
		}
		for i, saved := range state.Members {
			if saved.Repository != r.cursors[i].spec.Repository {
				return kernel.Fail(kernel.ErrPreconditionFailed, "continuation does not match this SearchView")
			}
			if saved.Offset < 0 {
				return kernel.Fail(kernel.ErrPreconditionFailed, "invalid member continuation offset")
			}
			r.cursors[i].offset = saved.Offset
			r.cursors[i].position = saved.Position
			r.cursors[i].exhausted = saved.Exhausted
		}
	}
	r.initialMembers = workspaceMemberContinuations(r.cursors)
	r.req.Continuation = ""
	return nil
}

// fetchMemberPage pulls one bounded page from a member, answering from the
// State projection at its prepared revision when the member requires it.
func (r *datasetSearchRun) fetchMemberPage(ctx context.Context, cursor *workspaceSearchCursor) (retrieval.SearchResult, error) {
	repo, err := r.exec.Repositories.Require(cursor.spec.Repository, kernel.ErrUsageInvalid)
	if err != nil {
		return retrieval.SearchResult{}, err
	}
	memberReq := r.req
	memberReq.Limit = cursor.batchLimit()
	memberReq.Continuation = cursor.position
	var member retrieval.SearchResult
	if r.stateMembers[cursor.spec.Repository] {
		member, err = r.exec.Projection.SearchStateAtRevisionContext(ctx, repo, cursor.spec.Commit, r.out.SearchView.ProjectionRevisions[cursor.spec.Repository], memberReq)
	} else {
		member, err = r.exec.Projection.SearchAtContext(ctx, repo, cursor.spec.Commit, memberReq)
	}
	if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
		return member, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "workspace member %s cannot satisfy SEARCH: %v; schema/* must declare the required text/filter/sort access", cursor.spec.Repository, err)
	}
	return member, err
}

// mergePage primes every member head and merges hits in workspace order,
// hydrating snapshot members and validating State members against the
// dataset view before delivery.
func (r *datasetSearchRun) mergePage() error {
	if err := primeWorkspaceHeads(r.ctx, r.cursors, r.fetchMemberPage); err != nil {
		return err
	}
	for len(r.out.Hits) < r.pageLimit {
		if r.ctx.Err() != nil {
			if !index.SearchBudgetExhausted(r.ctx) {
				return r.ctx.Err()
			}
			r.out.Completeness = retrieval.CompletenessPartial
			r.out.Stats.MarkPartial("budget")
			r.out.Claims = appendUniqueClaims(r.out.Claims, "search execution budget exhausted: time")
			break
		}
		if r.unknownHead() {
			break
		}
		best := bestWorkspaceHead(r.cursors, r.req)
		if best < 0 {
			break
		}
		cursor := &r.cursors[best]
		hit := *cursor.head
		hit.Knowledge.Repository = cursor.spec.Repository
		hit.Knowledge.KnowledgeRef.Repository = cursor.spec.Repository
		if hit.Knowledge.KnowledgeRef.Object == "" {
			hit.Knowledge.KnowledgeRef.Object = hit.Knowledge.Address.ObjectID
		}
		inView, err := r.serving.Contains(cursor.spec.Repository, hit.Knowledge.KnowledgeRef.Object)
		if err != nil {
			return err
		}
		if !inView {
			cursor.consumeHead()
			if len(r.out.Hits) < r.pageLimit {
				if err := fillWorkspaceHead(r.ctx, cursor, r.fetchMemberPage); err != nil {
					return err
				}
			}
			continue
		}
		stop, err := r.admitHit(cursor, &hit)
		if err != nil {
			return err
		}
		if stop {
			break
		}
	}
	for i := range r.cursors {
		r.out.Stats.Add(r.cursors[i].stats)
		r.out.Claims = appendUniqueClaims(r.out.Claims, r.cursors[i].claims...)
		if r.cursors[i].partial {
			r.out.Completeness = retrieval.CompletenessPartial
		}
	}
	return nil
}

// unknownHead reports whether any member's head is temporarily unknowable
// because a partial provider page blocked its refill.
func (r *datasetSearchRun) unknownHead() bool {
	for i := range r.cursors {
		if r.cursors[i].blocked {
			return true
		}
	}
	return false
}

// admitHit validates or hydrates one in-view head, delivers it into the
// page, and advances its cursor. It reports whether the page must stop
// early because the execution budget ran out during hydration.
func (r *datasetSearchRun) admitHit(cursor *workspaceSearchCursor, hit *retrieval.KnowledgeHit) (bool, error) {
	if r.stateMembers[cursor.spec.Repository] {
		if err := validateDatasetObservations(r.serving, *hit); err != nil {
			return false, err
		}
	} else {
		started := time.Now()
		hydrated, err := HydrateSearchHit(r.ctx, r.logical, *hit)
		r.out.Stats.HydrateDuration += time.Since(started)
		if index.SearchBudgetExhausted(r.ctx) {
			r.out.Completeness = retrieval.CompletenessPartial
			r.out.Stats.MarkPartial("budget")
			r.out.Claims = appendUniqueClaims(r.out.Claims, "search execution budget exhausted: time")
			return true, nil
		}
		if r.ctx.Err() != nil {
			return false, r.ctx.Err()
		}
		if err != nil {
			return false, err
		}
		*hit = hydrated
	}
	if hit.Knowledge.KnowledgeRef.Object == "" {
		hit.Knowledge.KnowledgeRef.Object = hit.Knowledge.Address.ObjectID
	}
	delivered, err := r.exec.Deliver(r.ctx, *hit)
	if err != nil {
		return false, err
	}
	*hit = delivered
	r.out.Hits = append(r.out.Hits, *hit)
	cursor.consumeHead()
	if len(r.out.Hits) < r.pageLimit {
		if err := fillWorkspaceHead(r.ctx, cursor, r.fetchMemberPage); err != nil {
			return false, err
		}
	}
	return false, nil
}

// finish aggregates cursor evidence and encodes the continuation token when
// members still have readable heads.
func (r *datasetSearchRun) finish() (retrieval.SearchResult, error) {
	if workspaceSearchHasMore(r.cursors) {
		members := workspaceMemberContinuations(r.cursors)
		if len(r.out.Hits) == 0 && !workspaceMembersAdvanced(r.initialMembers, members) {
			return retrieval.SearchResult{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "Workspace search cannot make paging progress within the execution budget; increase the execution budget or narrow the Workspace")
		}
		r.out.Continuation = retrieval.EncodeContinuation(retrieval.ContinuationState{
			Scope: "workspace", Query: r.queryDigest, SearchView: r.viewDigest, Members: members,
		})
	}
	return r.out, nil
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

func HydrateSearchHit(ctx context.Context, logical *knowledgeserving.Service, hit retrieval.KnowledgeHit) (retrieval.KnowledgeHit, error) {
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
