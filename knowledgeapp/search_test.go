package knowledgeapp

import (
	"context"
	"strings"
	"testing"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
	"kc/retrieval"
)

type searchRepository struct{ knowledge.Repository }

func (*searchRepository) ID() kernel.RepositoryID { return "kr://app/search" }

type searchLookup struct{ repo knowledge.Repository }

func (l searchLookup) Require(id kernel.RepositoryID, _ kernel.ErrorCode) (knowledge.Repository, error) {
	if id != l.repo.ID() {
		return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "missing repository")
	}
	return l.repo, nil
}

type searchProjection struct {
	stateRequired bool
	stateReady    bool
	snapshotCalls int
	stateCalls    int
}

func (p *searchProjection) RequiresState(knowledge.Repository, kernel.CommitID, retrieval.SearchRequest) (bool, error) {
	return p.stateRequired, nil
}
func (p *searchProjection) StateView(kernel.RepositoryID, kernel.CommitID) (string, bool) {
	return "revision", p.stateReady
}
func (p *searchProjection) SearchAtContext(context.Context, knowledge.Repository, kernel.CommitID, retrieval.SearchRequest) (retrieval.SearchResult, error) {
	p.snapshotCalls++
	return retrieval.SearchResult{}, nil
}
func (p *searchProjection) SearchStateAtRevisionContext(context.Context, knowledge.Repository, kernel.CommitID, string, retrieval.SearchRequest) (retrieval.SearchResult, error) {
	p.stateCalls++
	return retrieval.SearchResult{}, nil
}

func TestSearchExecutorSelectsOneTypedProjectionPath(t *testing.T) {
	repo := &searchRepository{}
	projection := &searchProjection{}
	executor := SearchExecutor{Repositories: searchLookup{repo: repo}, Projection: projection}
	if _, err := executor.Execute(context.Background(), SearchRequest{
		Repository: repo.ID(), Commit: "fixed", Query: retrieval.SearchRequest{},
	}); err != nil {
		t.Fatal(err)
	}
	if projection.snapshotCalls != 1 || projection.stateCalls != 0 {
		t.Fatalf("snapshot search calls = %d/%d", projection.snapshotCalls, projection.stateCalls)
	}

	projection.stateRequired, projection.stateReady = true, false
	_, err := executor.Execute(context.Background(), SearchRequest{
		Repository: repo.ID(), Commit: "fixed", Query: retrieval.SearchRequest{},
	})
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || projection.stateCalls != 0 {
		t.Fatalf("unprepared state search = %v, calls=%d", err, projection.stateCalls)
	}
	projection.stateReady = true
	if _, err := executor.Execute(context.Background(), SearchRequest{
		Repository: repo.ID(), Commit: "fixed", Query: retrieval.SearchRequest{},
	}); err != nil {
		t.Fatal(err)
	}
	if projection.stateCalls != 1 {
		t.Fatalf("state search calls = %d", projection.stateCalls)
	}
}

func TestSearchExecutorFailsClosedOnSemanticRecall(t *testing.T) {
	repo := &searchRepository{}
	projection := &searchProjection{}
	executor := SearchExecutor{Repositories: searchLookup{repo: repo}, Projection: projection}
	query := retrieval.SearchOf(retrieval.SearchMATCH("gmv"))
	query.Recall = retrieval.RecallSemantic
	_, err := executor.Execute(context.Background(), SearchRequest{Repository: repo.ID(), Commit: "c1", Query: query})
	if code := kernel.CodeOf(err); code != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("semantic recall must fail closed with CAPABILITY_UNSATISFIED, got %v", err)
	}
	if projection.snapshotCalls != 0 || projection.stateCalls != 0 {
		t.Fatalf("ineligible recall must not reach any lane: %d snapshot / %d state calls", projection.snapshotCalls, projection.stateCalls)
	}
}

func TestDatasetSearchExecutorSemanticAuthorizesFirstThenFailsClosedWithoutProvider(t *testing.T) {
	authorizeCalls := 0
	read := func(context.Context) (*reader.Serving, *knowledgeserving.Service, error) {
		t.Fatal("ineligible recall resolved data")
		return nil, nil, nil
	}
	deliver := func(_ context.Context, v retrieval.KnowledgeHit) (retrieval.KnowledgeHit, error) {
		t.Fatal("ineligible recall delivered data")
		return v, nil
	}
	authorize := func(context.Context) error {
		authorizeCalls++
		return nil
	}
	executor := DatasetSearchExecutor{Authorize: authorize, Resolve: read, Deliver: deliver,
		Repositories: searchLookup{repo: &searchRepository{}}, Projection: &searchProjection{}}
	query := retrieval.SearchOf(retrieval.SearchMATCH("gmv"))
	query.Recall = retrieval.RecallSemantic
	_, err := executor.Execute(context.Background(), query)
	if code := kernel.CodeOf(err); code != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("semantic recall without a provider must fail closed, got %v", err)
	}
	if authorizeCalls != 1 {
		t.Fatalf("authorization must run exactly once before recall work, got %d", authorizeCalls)
	}
}

type embeddingProvider struct {
	calls    int
	vectors  [][]float32
	err      error
	lastText string
}

func (p *embeddingProvider) Embed(_ context.Context, request retrieval.EmbeddingRequest) (retrieval.EmbeddingResult, error) {
	p.calls++
	if len(request.Texts) == 1 {
		p.lastText = request.Texts[0]
	}
	if p.err != nil {
		return retrieval.EmbeddingResult{}, p.err
	}
	return retrieval.EmbeddingResult{Vectors: p.vectors, Provider: "test", Model: "embed-1", Dimensions: len(p.vectors[0])}, nil
}

type semanticProjection struct {
	searchProjection
	vector        []float32
	windowQueries int
	overclaimed   bool
}

func (p *semanticProjection) SemanticWindowAtContext(_ context.Context, _ knowledge.Repository, _ kernel.CommitID, _ retrieval.SearchRequest, queryVector []float32) (retrieval.SearchResult, error) {
	p.windowQueries++
	p.vector = append([]float32(nil), queryVector...)
	result := retrieval.SearchResult{Hits: []retrieval.KnowledgeHit{{}}}
	if p.overclaimed {
		result.Completeness = retrieval.CompletenessComplete
	}
	return result, nil
}

func TestSearchExecutorRunsSemanticWindowAndForcesApproximate(t *testing.T) {
	repo := &searchRepository{}
	projection := &semanticProjection{searchProjection: searchProjection{}}
	embedder := &embeddingProvider{vectors: [][]float32{{0.1, 0.2, 0.3}}}
	executor := SearchExecutor{Repositories: searchLookup{repo: repo}, Projection: projection, Embedder: embedder}
	query := retrieval.SearchOf(retrieval.SearchMATCH("gmv trend"))
	query.Recall = retrieval.RecallSemantic
	result, err := executor.Execute(context.Background(), SearchRequest{Repository: repo.ID(), Commit: "c1", Query: query})
	if err != nil {
		t.Fatal(err)
	}
	if embedder.calls != 1 || embedder.lastText != "gmv trend" {
		t.Fatalf("embedding calls = %d, text = %q", embedder.calls, embedder.lastText)
	}
	if projection.windowQueries != 1 || len(projection.vector) != 3 {
		t.Fatalf("window queries = %d, vector = %v", projection.windowQueries, projection.vector)
	}
	if result.Completeness != retrieval.CompletenessPartial {
		t.Fatalf("semantic result must be partial, got %s", result.Completeness)
	}
	joined := strings.Join(result.Claims, "\n")
	if !strings.Contains(joined, "approximate k-NN ranking window") || !strings.Contains(joined, "test/embed-1") {
		t.Fatalf("semantic result claims must disclose window and model, got %q", joined)
	}
}

func TestSearchExecutorForcesPartialOverOverclaimedWindow(t *testing.T) {
	repo := &searchRepository{}
	projection := &semanticProjection{searchProjection: searchProjection{}, overclaimed: true}
	embedder := &embeddingProvider{vectors: [][]float32{{0.4}}}
	executor := SearchExecutor{Repositories: searchLookup{repo: repo}, Projection: projection, Embedder: embedder}
	query := retrieval.SearchOf(retrieval.SearchMATCH("gmv"))
	query.Recall = retrieval.RecallSemantic
	result, err := executor.Execute(context.Background(), SearchRequest{Repository: repo.ID(), Commit: "c1", Query: query})
	if err != nil {
		t.Fatal(err)
	}
	if result.Completeness != retrieval.CompletenessPartial {
		t.Fatalf("overclaimed semantic window must be forced to partial, got %s", result.Completeness)
	}
}

func TestSearchExecutorFailsClosedWithoutEmbeddingProvider(t *testing.T) {
	repo := &searchRepository{}
	projection := &semanticProjection{searchProjection: searchProjection{}}
	executor := SearchExecutor{Repositories: searchLookup{repo: repo}, Projection: projection}
	query := retrieval.SearchOf(retrieval.SearchMATCH("gmv"))
	query.Recall = retrieval.RecallSemantic
	_, err := executor.Execute(context.Background(), SearchRequest{Repository: repo.ID(), Commit: "c1", Query: query})
	if code := kernel.CodeOf(err); code != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("semantic recall without a provider must fail closed, got %v", err)
	}
	if projection.windowQueries != 0 {
		t.Fatalf("ineligible recall must not reach the vector window")
	}
}

func TestSearchExecutorFailsClosedWhenProjectionHasNoVectorWindow(t *testing.T) {
	repo := &searchRepository{}
	projection := &searchProjection{}
	embedder := &embeddingProvider{vectors: [][]float32{{0.4}}}
	executor := SearchExecutor{Repositories: searchLookup{repo: repo}, Projection: projection, Embedder: embedder}
	query := retrieval.SearchOf(retrieval.SearchMATCH("gmv"))
	query.Recall = retrieval.RecallSemantic
	_, err := executor.Execute(context.Background(), SearchRequest{Repository: repo.ID(), Commit: "c1", Query: query})
	if code := kernel.CodeOf(err); code != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("semantic recall without a vector projection must fail closed, got %v", err)
	}
	if projection.snapshotCalls != 0 {
		t.Fatalf("ineligible semantic recall must not fall back to the lexical lane")
	}
}

func TestSearchExecutorFailsClosedOnSemanticStateView(t *testing.T) {
	repo := &searchRepository{}
	projection := &semanticProjection{searchProjection: searchProjection{stateRequired: true, stateReady: true}}
	embedder := &embeddingProvider{vectors: [][]float32{{0.4}}}
	executor := SearchExecutor{Repositories: searchLookup{repo: repo}, Projection: projection, Embedder: embedder}
	query := retrieval.SearchOf(retrieval.SearchMATCH("gmv"))
	query.Recall = retrieval.RecallSemantic
	_, err := executor.Execute(context.Background(), SearchRequest{Repository: repo.ID(), Commit: "c1", Query: query})
	if code := kernel.CodeOf(err); code != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("semantic recall on the state view must fail closed, got %v", err)
	}
	if projection.stateCalls != 0 {
		t.Fatalf("semantic recall must not run the lexical state lane")
	}
}

type headCommitRepository struct {
	searchRepository
	id   kernel.RepositoryID
	head kernel.CommitID
}

func (r *headCommitRepository) ID() kernel.RepositoryID               { return r.id }
func (r *headCommitRepository) HasCommit(commit kernel.CommitID) bool { return commit == r.head }
func (r *headCommitRepository) SchemaObjectIDs(kernel.CommitID) ([]knowledge.ObjectID, error) {
	return nil, nil
}

type datasetSemanticProjection struct {
	searchProjection
	windows map[string][]retrieval.KnowledgeHit // "repo\x1fcommit" -> hits
	calls   []string
}

func (p *datasetSemanticProjection) SemanticWindowAtContext(_ context.Context, repo knowledge.Repository, commit kernel.CommitID, _ retrieval.SearchRequest, queryVector []float32) (retrieval.SearchResult, error) {
	if len(queryVector) == 0 {
		return retrieval.SearchResult{}, kernel.Fail(kernel.ErrUsageInvalid, "test expects a query vector")
	}
	p.calls = append(p.calls, string(repo.ID()))
	return retrieval.SearchResult{Hits: append([]retrieval.KnowledgeHit(nil), p.windows[string(repo.ID())+"\x1f"+string(commit)]...)}, nil
}

func semanticHit(repo kernel.RepositoryID, commit kernel.CommitID, object string) retrieval.KnowledgeHit {
	return retrieval.KnowledgeHit{Knowledge: knowledge.KnowledgeValue{
		Repository: repo, Commit: commit,
		KnowledgeRef: knowledge.KnowledgeRef{Repository: repo, Object: knowledge.ObjectID(object)},
		Address:      knowledge.Address{ObjectID: knowledge.ObjectID(object)},
	}}
}

func TestDatasetSearchExecutorMergesSemanticMemberWindowsByRankInterleave(t *testing.T) {
	headedA := &headCommitRepository{searchRepository: searchRepository{}, id: "kr://a/entities", head: "ca"}
	headedB := &headCommitRepository{searchRepository: searchRepository{}, id: "kr://b/graph", head: "cb"}
	repos := relationTestLookup{"kr://a/entities": headedA, "kr://b/graph": headedB}
	pin := reader.KnowledgeSetPin{Repositories: map[kernel.RepositoryID]kernel.CommitID{
		"kr://a/entities": "ca", "kr://b/graph": "cb",
	}, Items: reader.WholeRepositoryItems(map[kernel.RepositoryID]kernel.CommitID{
		"kr://a/entities": "ca", "kr://b/graph": "cb",
	})}
	lookup := func(id kernel.RepositoryID) (knowledge.Repository, error) {
		return repos.Require(id, kernel.ErrKnowledgeRefUnresolved)
	}
	projection := &datasetSemanticProjection{windows: map[string][]retrieval.KnowledgeHit{
		"kr://a/entities\x1fca": {semanticHit("kr://a/entities", "ca", "metric/gmv"), semanticHit("kr://a/entities", "ca", "metric/aov")},
		"kr://b/graph\x1fcb":    {semanticHit("kr://b/graph", "cb", "rel/defines/gmv")},
	}}
	delivered := 0
	executor := DatasetSearchExecutor{
		Authorize: func(context.Context) error { return nil },
		Resolve: func(context.Context) (*reader.Serving, *knowledgeserving.Service, error) {
			return reader.Open(lookup, pin), nil, nil
		},
		Repositories: repos, Projection: projection, Embedder: &embeddingProvider{vectors: [][]float32{{0.5, 0.5}}},
		Deliver: func(_ context.Context, hit retrieval.KnowledgeHit) (retrieval.KnowledgeHit, error) {
			delivered++
			return hit, nil
		},
	}
	query := retrieval.SearchOf(retrieval.SearchMATCH("gmv trend"))
	query.Recall = retrieval.RecallSemantic
	query.Limit = 3
	result, err := executor.Execute(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 3 || delivered != 3 {
		t.Fatalf("hits=%d delivered=%d", len(result.Hits), delivered)
	}
	wantOrder := []string{"metric/gmv", "rel/defines/gmv", "metric/aov"}
	for i, object := range wantOrder {
		if string(result.Hits[i].Knowledge.Address.ObjectID) != object {
			t.Fatalf("rank %d = %s, want %s (order: %v)", i+1, result.Hits[i].Knowledge.Address.ObjectID, object, result.Hits)
		}
	}
	if result.Completeness != retrieval.CompletenessPartial {
		t.Fatalf("merged semantic result must be partial, got %s", result.Completeness)
	}
	claims := strings.Join(result.Claims, "\n")
	if !strings.Contains(claims, "2 member windows") || !strings.Contains(claims, "test/embed-1") {
		t.Fatalf("claims must disclose member windows and model: %q", claims)
	}
	if len(projection.calls) != 2 || projection.calls[0] != "kr://a/entities" || projection.calls[1] != "kr://b/graph" {
		t.Fatalf("member window order must be deterministic: %v", projection.calls)
	}
}

func TestDatasetSearchExecutorSemanticFailsClosedOnStateMember(t *testing.T) {
	repos := relationTestLookup{"kr://a/entities": &headCommitRepository{searchRepository: searchRepository{}, id: "kr://a/entities", head: "ca"}}
	pin := reader.KnowledgeSetPin{Repositories: map[kernel.RepositoryID]kernel.CommitID{"kr://a/entities": "ca"},
		Items: reader.WholeRepositoryItems(map[kernel.RepositoryID]kernel.CommitID{"kr://a/entities": "ca"})}
	lookup := func(id kernel.RepositoryID) (knowledge.Repository, error) {
		return repos.Require(id, kernel.ErrKnowledgeRefUnresolved)
	}
	projection := &datasetSemanticProjection{searchProjection: searchProjection{stateRequired: true, stateReady: true}}
	executor := DatasetSearchExecutor{
		Authorize: func(context.Context) error { return nil },
		Resolve: func(context.Context) (*reader.Serving, *knowledgeserving.Service, error) {
			return reader.Open(lookup, pin), nil, nil
		},
		Repositories: repos, Projection: projection, Embedder: &embeddingProvider{vectors: [][]float32{{0.5}}},
		Deliver: func(_ context.Context, hit retrieval.KnowledgeHit) (retrieval.KnowledgeHit, error) { return hit, nil },
	}
	query := retrieval.SearchOf(retrieval.SearchMATCH("gmv"))
	query.Recall = retrieval.RecallSemantic
	_, err := executor.Execute(context.Background(), query)
	if code := kernel.CodeOf(err); code != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("State member must fail the semantic window closed, got %v", err)
	}
	if len(projection.calls) != 0 {
		t.Fatalf("State member must not open a vector window")
	}
}
