package index

import (
	"regexp"
	"sync"

	"kc/kernel"
	"kc/reader"
	"kc/repository"
)

const (
	IndexModeIncremental = "incremental"
	IndexModeRebuild     = "rebuild"
	IndexModeReady       = "ready"

	// Two first-class change kinds. Other causes explain why we could not increment.
	IndexCauseContent  = "content" // knowledge values changed; AccessHints unchanged
	IndexCauseSchema   = "schema"  // schema/* AccessHints (index config) changed
	IndexCauseReady    = "ready"
	IndexCauseCold     = "cold"     // no projection yet
	IndexCauseDiverged = "diverged" // stored basis is not the from-commit
)

// Search evaluates atomic SEARCH clauses (reader.SearchRequest) against one repo projection.

// IndexSync reports how a projection caught up to a commit.
// Cause is why: content (object upsert) vs schema (recompile from AccessHints).
type IndexSync struct {
	Mode           string              `json:"mode"`
	Cause          string              `json:"cause"`
	Repository     kernel.RepositoryID `json:"repository"`
	BasisCommit    kernel.CommitID     `json:"basisCommit"`
	AccessDigest   kernel.Digest       `json:"accessDigest"`
	PhysicalDigest kernel.Digest       `json:"physicalDigest"`
	ObjectCount    int                 `json:"objectCount"`
	Updated        int                 `json:"updated"`
	Removed        int                 `json:"removed"`
}

// IndexDescriptor is DESCRIBE_INDEX for one working projection (not a published snapshot).
// Fields / Schemas / Lanes are the compiled AccessHints at BasisCommit.
type IndexDescriptor struct {
	BasisRepository  kernel.RepositoryID  `json:"basisRepository"`
	BasisCommit      kernel.CommitID      `json:"basisCommit"`
	ObjectCount      int                  `json:"objectCount"`
	HeadCommit       kernel.CommitID      `json:"headCommit"`
	LagBehindHead    bool                 `json:"lagBehindHead"`
	AccessDigest     kernel.Digest        `json:"accessDigest,omitempty"`
	PhysicalDigest   kernel.Digest        `json:"physicalDigest,omitempty"`
	ProviderRevision string               `json:"providerRevision,omitempty"`
	Mode             string               `json:"mode,omitempty"`
	Cause            string               `json:"cause,omitempty"`
	Schemas          []kernel.ObjectID    `json:"schemas,omitempty"`
	Lanes            []string             `json:"lanes,omitempty"`
	Fields           []reader.AccessField `json:"fields,omitempty"`
}

// engineKey is one physical projection. commit=="" is the live working engine
// (AfterSnapshot / Ensure / maintainer Search). A non-empty commit is a frozen
// pin used by consume SearchAt; it must not rewind live.
type engineKey struct {
	repo   kernel.RepositoryID
	commit kernel.CommitID
}

// Index sits above one Repository (K-19): derived, discardable, never Canonical.
// Independent of Writer / Reader / Catalog. Subscribe via catalog.Hook (Sink).
// Live key is repository id (commit empty). Pin key is (repository, basisCommit).
// Same physical projection per member basis, not per Workspace. EngineOpener still receives (dir, id);
// pin engines open as id+"@"+commit so sqlite/ES get a different file/index.
type Index struct {
	dir  string
	open EngineOpener
	mu   sync.Mutex
	engs map[engineKey]Engine
}

func NewIndex(dir string) *Index {
	return NewIndexEngine(dir, nil)
}

// NewIndexEngine builds one working projection.
//
// Args:
//
//	dir: local projection directory. Elasticsearch ignores this path.
//	opener: physical engine factory (local.OpenSQLite or scale.OpenElasticsearch). nil is rejected.
//
// Returns:
//
//	an Index ready to subscribe via Catalog.Hook.
func NewIndexEngine(dir string, opener EngineOpener) *Index {
	if opener == nil {
		opener = func(string, kernel.RepositoryID) (Engine, error) {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "index engine opener required (local.OpenSQLite or scale.OpenElasticsearch)")
		}
	}
	return &Index{dir: dir, open: opener, engs: map[engineKey]Engine{}}
}

// AfterSnapshot applies a member snapshot to the working projection.
// Catalog never imports this package; CLI / tests wrap Catalog.Hook.
func (idx *Index) AfterSnapshot(repo repository.Repository, from, to kernel.CommitID, objectIDs []kernel.ObjectID) error {
	if idx == nil || repo == nil {
		return nil
	}
	if objectIDs == nil {
		_, err := idx.Ensure(repo, to)
		return err
	}
	_, err := idx.Apply(repo, from, to, objectIDs)
	return err
}

func (idx *Index) Close() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	var first error
	for key, eng := range idx.engs {
		if err := eng.Close(); err != nil && first == nil {
			first = err
		}
		delete(idx.engs, key)
	}
	return first
}

func (idx *Index) Ensure(repo repository.Repository, commit kernel.CommitID) (IndexSync, error) {
	spec, err := specAtCommit(repo, commit)
	if err != nil {
		return IndexSync{}, err
	}
	eng, err := idx.engine(repo.ID())
	if err != nil {
		return IndexSync{}, err
	}
	meta, err := eng.LoadMeta()
	if err != nil {
		return IndexSync{}, err
	}
	if projectionMatches(eng, meta, commit, spec.AccessDigest) {
		return readySync(repo.ID(), commit, spec.AccessDigest, eng)
	}
	if meta.Basis == "" {
		return idx.rebuild(eng, repo, commit, spec, IndexCauseCold)
	}
	if meta.AccessDigest != spec.AccessDigest || !physicalMatches(eng, meta) {
		return idx.rebuild(eng, repo, commit, spec, IndexCauseSchema)
	}
	ids, err := repository.ChangedObjectIDs(repo, meta.Basis, commit)
	if err != nil {
		return idx.rebuild(eng, repo, commit, spec, IndexCauseDiverged)
	}
	sync, err := idx.apply(eng, repo, meta.Basis, commit, spec, ids, IndexCauseContent)
	if err != nil {
		return idx.rebuild(eng, repo, commit, spec, IndexCauseDiverged)
	}
	return sync, nil
}

// EnsureAt builds a projection at commit without moving the live engine.
// If live already sits on that commit, it is reused. Otherwise a pin engine
// is rebuilt at commit. Never Ensure(live, oldCommit).
func (idx *Index) EnsureAt(repo repository.Repository, commit kernel.CommitID) (IndexSync, error) {
	if commit == "" {
		return IndexSync{}, kernel.Fail(kernel.ErrUsageInvalid, "EnsureAt requires a commit")
	}
	spec, err := specAtCommit(repo, commit)
	if err != nil {
		return IndexSync{}, err
	}
	live, err := idx.engine(repo.ID())
	if err != nil {
		return IndexSync{}, err
	}
	liveMeta, err := live.LoadMeta()
	if err != nil {
		return IndexSync{}, err
	}
	if projectionMatches(live, liveMeta, commit, spec.AccessDigest) {
		return readySync(repo.ID(), commit, spec.AccessDigest, live)
	}
	eng, err := idx.engineAt(repo.ID(), commit)
	if err != nil {
		return IndexSync{}, err
	}
	meta, err := eng.LoadMeta()
	if err != nil {
		return IndexSync{}, err
	}
	if projectionMatches(eng, meta, commit, spec.AccessDigest) {
		return readySync(repo.ID(), commit, spec.AccessDigest, eng)
	}
	cause := IndexCauseCold
	if meta.Basis != "" {
		cause = IndexCauseContent
	}
	return idx.rebuild(eng, repo, commit, spec, cause)
}

func (idx *Index) Apply(repo repository.Repository, from, to kernel.CommitID, objectIDs []kernel.ObjectID) (IndexSync, error) {
	spec, err := specAtCommit(repo, to)
	if err != nil {
		return IndexSync{}, err
	}
	eng, err := idx.engine(repo.ID())
	if err != nil {
		return IndexSync{}, err
	}
	meta, err := eng.LoadMeta()
	if err != nil {
		return IndexSync{}, err
	}
	cause, needRebuild := classify(eng, meta, from, spec.AccessDigest)
	if needRebuild {
		return idx.rebuild(eng, repo, to, spec, cause)
	}
	if meta.Basis == to {
		return readySync(repo.ID(), to, spec.AccessDigest, eng)
	}
	return idx.apply(eng, repo, from, to, spec, objectIDs, IndexCauseContent)
}

func classify(eng Engine, meta Meta, from kernel.CommitID, digest kernel.Digest) (cause string, rebuild bool) {
	if meta.Basis == "" {
		return IndexCauseCold, true
	}
	if meta.AccessDigest != digest || !physicalMatches(eng, meta) {
		return IndexCauseSchema, true
	}
	if from != "" && meta.Basis != from {
		return IndexCauseDiverged, true
	}
	return IndexCauseContent, false
}

func (idx *Index) Rebuild(repo repository.Repository, commit kernel.CommitID) (IndexSync, error) {
	spec, err := specAtCommit(repo, commit)
	if err != nil {
		return IndexSync{}, err
	}
	eng, err := idx.engine(repo.ID())
	if err != nil {
		return IndexSync{}, err
	}
	cause := IndexCauseCold
	if meta, err := eng.LoadMeta(); err == nil && meta.Basis != "" {
		if meta.AccessDigest != spec.AccessDigest || !physicalMatches(eng, meta) {
			cause = IndexCauseSchema
		} else {
			cause = IndexCauseContent
		}
	}
	return idx.rebuild(eng, repo, commit, spec, cause)
}

func (idx *Index) Search(repo repository.Repository, req reader.SearchRequest) (reader.SearchResult, error) {
	eng, err := idx.engine(repo.ID())
	if err != nil {
		return reader.SearchResult{}, err
	}
	meta, err := eng.LoadMeta()
	if err != nil {
		return reader.SearchResult{}, err
	}
	if meta.Basis == "" {
		return reader.SearchResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "projection for %s is empty; write or index-sync first", repo.ID())
	}
	return idx.searchEngine(repo, eng, meta.Basis, req)
}

// SearchAt evaluates SEARCH at a frozen commit. Live AfterSnapshot / maintainer
// Search keep their own engine. Hydrate Canonical at the requested commit.
func (idx *Index) SearchAt(repo repository.Repository, commit kernel.CommitID, req reader.SearchRequest) (reader.SearchResult, error) {
	if commit == "" {
		return idx.Search(repo, req)
	}
	if _, err := idx.EnsureAt(repo, commit); err != nil {
		return reader.SearchResult{}, err
	}
	eng, err := idx.engineForCommit(repo.ID(), commit)
	if err != nil {
		return reader.SearchResult{}, err
	}
	return idx.searchEngine(repo, eng, commit, req)
}

func (idx *Index) searchEngine(repo repository.Repository, eng Engine, commit kernel.CommitID, req reader.SearchRequest) (reader.SearchResult, error) {
	result := reader.SearchResult{
		View:         reader.SearchView{Snapshots: map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit}},
		Completeness: reader.CompletenessComplete,
		Hits:         []reader.KnowledgeHit{},
	}
	spec, err := specAtCommit(repo, commit)
	if err != nil {
		return reader.SearchResult{}, err
	}
	var identity ProviderIdentity
	if provider, ok := eng.(ProviderIdentity); ok {
		identity = provider
	}
	plan, err := PlanRetrieval(eng, identity, req, spec)
	if err != nil {
		return reader.SearchResult{}, err
	}
	resolved := plan.Search
	needsResidual := false
	for _, fragment := range plan.Fragments {
		capability := fragment.Capability
		if capability.Guarantee == GuaranteeUnsupported {
			return reader.SearchResult{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "%s", capability.Reason)
		}
		if capability.Guarantee == GuaranteeSuperset {
			needsResidual = true
		}
		if capability.Guarantee == GuaranteeApproximate || capability.Coverage < 1 {
			result.Completeness = reader.CompletenessPartial
			result.Claims = append(result.Claims, "provider guarantee="+string(capability.Guarantee))
		}
	}
	viewDigest := reader.SearchViewDigest(result.View)
	queryDigest := reader.SearchQueryDigest(resolved)
	projectionDigest := kernel.CanonicalDigest(plan.Projection)
	continuation := ""
	if resolved.Continuation != "" {
		state, err := reader.DecodeContinuation(resolved.Continuation)
		if err != nil || state.Scope != "repository" || state.Query != queryDigest || state.View != viewDigest || state.Projection != projectionDigest {
			return reader.SearchResult{}, kernel.Fail(kernel.ErrPreconditionFailed, "continuation does not match this search view")
		}
		continuation = state.Position
	}
	resolved.Continuation = ""
	nextContinuation := ""
	for {
		pageReq := resolved
		if resolved.Limit > 0 {
			pageReq.Limit = resolved.Limit - len(result.Hits)
			if pageReq.Limit <= 0 {
				break
			}
		}
		page, err := eng.Retrieve(RetrieveRequest{Search: pageReq, Spec: spec, Continuation: continuation})
		if err != nil {
			return reader.SearchResult{}, err
		}
		for _, candidate := range page.Candidates {
			if candidate.Repository != "" && candidate.Repository != repo.ID() {
				result.Completeness = reader.CompletenessPartial
				result.Claims = append(result.Claims, "candidate repository mismatch")
				continue
			}
			candidate.Repository = repo.ID()
			if candidate.Basis != commit {
				result.Completeness = reader.CompletenessPartial
				result.Claims = append(result.Claims, "candidate basis mismatch")
				continue
			}
			value, err := repo.Read(candidate.ObjectID, commit)
			if err != nil {
				if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
					result.Completeness = reader.CompletenessPartial
					result.Claims = append(result.Claims, "candidate removed before hydrate: "+string(candidate.ObjectID))
					continue
				}
				return reader.SearchResult{}, err
			}
			if needsResidual {
				matched, err := matchesResidual(repo, value, resolved, spec)
				if err != nil {
					return reader.SearchResult{}, err
				}
				if !matched {
					continue
				}
			}
			result.Hits = append(result.Hits, reader.KnowledgeHit{Knowledge: value, Version: reader.VersionOf(value), Evidence: candidate.Evidence})
			if resolved.Limit > 0 && len(result.Hits) >= resolved.Limit {
				break
			}
		}
		nextContinuation = page.Continuation
		if page.Exhausted || page.Continuation == "" || (resolved.Limit > 0 && len(result.Hits) >= resolved.Limit) {
			break
		}
		continuation = page.Continuation
	}
	if resolved.Limit > 0 && len(result.Hits) >= resolved.Limit && nextContinuation != "" {
		result.Continuation = reader.EncodeContinuation(reader.ContinuationState{
			Scope: "repository", Query: queryDigest, View: viewDigest,
			Projection: projectionDigest, Position: nextContinuation,
		})
	}
	return result, nil
}

func (idx *Index) Describe(repo repository.Repository) (IndexDescriptor, error) {
	return idx.describe(repo, "")
}

// DescribeAt reports the projection at commit (EnsureAt first). Live Describe
// is unchanged. inspect --workspace uses this so the descriptor matches the pin.
func (idx *Index) DescribeAt(repo repository.Repository, commit kernel.CommitID) (IndexDescriptor, error) {
	if commit == "" {
		return idx.Describe(repo)
	}
	if _, err := idx.EnsureAt(repo, commit); err != nil {
		return IndexDescriptor{}, err
	}
	return idx.describe(repo, commit)
}

func (idx *Index) describe(repo repository.Repository, commit kernel.CommitID) (IndexDescriptor, error) {
	var (
		eng Engine
		err error
	)
	if commit == "" {
		eng, err = idx.engine(repo.ID())
	} else {
		eng, err = idx.engineForCommit(repo.ID(), commit)
	}
	if err != nil {
		return IndexDescriptor{}, err
	}
	meta, err := eng.LoadMeta()
	if err != nil {
		return IndexDescriptor{}, err
	}
	count, err := eng.Count()
	if err != nil {
		return IndexDescriptor{}, err
	}
	head, err := repo.Head("")
	if err != nil {
		return IndexDescriptor{}, err
	}
	desc := IndexDescriptor{
		BasisRepository:  repo.ID(),
		BasisCommit:      meta.Basis,
		ObjectCount:      count,
		HeadCommit:       head,
		LagBehindHead:    meta.Basis != "" && head != meta.Basis,
		AccessDigest:     meta.AccessDigest,
		PhysicalDigest:   meta.PhysicalDigest,
		ProviderRevision: meta.ProviderRevision,
		Mode:             meta.Mode,
		Cause:            meta.Cause,
	}
	if meta.Basis != "" {
		spec, err := specAtCommit(repo, meta.Basis)
		if err != nil {
			return IndexDescriptor{}, err
		}
		desc.Schemas = spec.Schemas
		desc.Lanes = spec.QueryLanes()
		desc.Fields = spec.Fields
	}
	return desc, nil
}

func specAtCommit(repo repository.Repository, commit kernel.CommitID) (reader.AccessSpec, error) {
	report, err := reader.DescribeRepoSchema(repo, commit, "")
	if err != nil {
		return reader.AccessSpec{}, err
	}
	return reader.AccessSpecFromReport(report), nil
}

func (idx *Index) engine(id kernel.RepositoryID) (Engine, error) {
	return idx.engineAt(id, "")
}

func pinOpenID(id kernel.RepositoryID, commit kernel.CommitID) kernel.RepositoryID {
	if commit == "" {
		return id
	}
	return kernel.RepositoryID(string(id) + "@" + string(commit))
}

func (idx *Index) engineAt(id kernel.RepositoryID, commit kernel.CommitID) (Engine, error) {
	key := engineKey{repo: id, commit: commit}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if eng, ok := idx.engs[key]; ok {
		return eng, nil
	}
	eng, err := idx.open(idx.dir, pinOpenID(id, commit))
	if err != nil {
		return nil, err
	}
	idx.engs[key] = eng
	return eng, nil
}

func (idx *Index) engineForCommit(id kernel.RepositoryID, commit kernel.CommitID) (Engine, error) {
	live, err := idx.engine(id)
	if err != nil {
		return nil, err
	}
	meta, err := live.LoadMeta()
	if err != nil {
		return nil, err
	}
	if meta.Basis == commit {
		return live, nil
	}
	return idx.engineAt(id, commit)
}

func readySync(id kernel.RepositoryID, commit kernel.CommitID, digest kernel.Digest, eng Engine) (IndexSync, error) {
	count, err := eng.Count()
	if err != nil {
		return IndexSync{}, err
	}
	return IndexSync{
		Mode: IndexModeReady, Cause: IndexCauseReady, Repository: id,
		BasisCommit: commit, AccessDigest: digest, PhysicalDigest: projectionPhysicalDigest(eng), ObjectCount: count,
	}, nil
}

func projectionMeta(eng Engine, basis kernel.CommitID, access kernel.Digest, mode, cause string) Meta {
	meta := Meta{Basis: basis, AccessDigest: access, Mode: mode, Cause: cause}
	if provider, ok := eng.(ProviderIdentity); ok {
		meta.ProviderRevision = provider.ProviderRevision()
		meta.PhysicalDigest = provider.PhysicalDigest()
	}
	return meta
}

func projectionPhysicalDigest(eng Engine) kernel.Digest {
	if provider, ok := eng.(ProviderIdentity); ok {
		return provider.PhysicalDigest()
	}
	return ""
}

func physicalMatches(eng Engine, meta Meta) bool {
	provider, ok := eng.(ProviderIdentity)
	if !ok {
		return true
	}
	return meta.PhysicalDigest == provider.PhysicalDigest() && meta.ProviderRevision == provider.ProviderRevision()
}

func projectionMatches(eng Engine, meta Meta, basis kernel.CommitID, access kernel.Digest) bool {
	return meta.Basis == basis && meta.AccessDigest == access && physicalMatches(eng, meta)
}

func (idx *Index) rebuild(eng Engine, repo repository.Repository, commit kernel.CommitID, spec reader.AccessSpec, cause string) (IndexSync, error) {
	listed, err := repo.List(commit)
	if err != nil {
		return IndexSync{}, err
	}
	var docs []CompiledDoc
	for _, value := range listed {
		if doc, ok := compileValue(repo, value, spec); ok {
			docs = append(docs, doc)
		}
	}
	meta := projectionMeta(eng, commit, spec.AccessDigest, IndexModeRebuild, cause)
	if err := eng.Rebuild(docs, meta); err != nil {
		return IndexSync{}, err
	}
	return IndexSync{
		Mode: IndexModeRebuild, Cause: cause, Repository: repo.ID(), BasisCommit: commit,
		AccessDigest: spec.AccessDigest, PhysicalDigest: meta.PhysicalDigest, ObjectCount: len(docs), Updated: len(docs),
	}, nil
}

func (idx *Index) apply(eng Engine, repo repository.Repository, from, to kernel.CommitID, spec reader.AccessSpec, objectIDs []kernel.ObjectID, cause string) (IndexSync, error) {
	var upserts []CompiledDoc
	var deletes []kernel.ObjectID
	seen := map[kernel.ObjectID]struct{}{}
	for _, id := range objectIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		value, err := repo.Read(id, to)
		if err != nil {
			if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
				deletes = append(deletes, id)
				continue
			}
			return IndexSync{}, err
		}
		if doc, ok := compileValue(repo, value, spec); ok {
			upserts = append(upserts, doc)
		} else {
			deletes = append(deletes, id)
		}
	}
	meta := projectionMeta(eng, to, spec.AccessDigest, IndexModeIncremental, cause)
	if err := eng.Apply(upserts, deletes, meta); err != nil {
		return IndexSync{}, err
	}
	count, err := eng.Count()
	if err != nil {
		return IndexSync{}, err
	}
	return IndexSync{
		Mode: IndexModeIncremental, Cause: cause, Repository: repo.ID(), BasisCommit: to,
		AccessDigest: spec.AccessDigest, PhysicalDigest: meta.PhysicalDigest, ObjectCount: count, Updated: len(upserts), Removed: len(deletes),
	}, nil
}

var unsafeIndexChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func SanitizeID(id string) string {
	s := unsafeIndexChars.ReplaceAllString(id, "_")
	if s == "" {
		return "repo"
	}
	return s
}
