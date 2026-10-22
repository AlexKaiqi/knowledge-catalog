package opensearchimpl

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	"kc/kernel"
)

func TestLiveOpenSearchVectorComputationReuseAcrossRebuildAndColdReopenKeepsCurrentSources(t *testing.T) {
	e, resources := publicationRecoveryEngine(t)
	model := &countedEmbedding{model: "test@1"}
	e.embedder, e.vectorModel, e.vectorDimension = model, "test@1", 2
	e.computationCacheDirectory, e.embeddingIdentity = t.TempDir(), "counted-config-v1"
	e.embeddingCacheIdentity = e.embeddingIdentity
	docs := publicationRecoveryDocs()
	docs[0].SourceRepository, docs[0].SourceCommit = "source/first", "first-commit"
	docs[0].Semantic = []retrieval.SemanticField{{Member: "member/one", Model: "test@1", Text: "unchanged"}}
	if err := e.Rebuild(docs, ports.Meta{Basis: "first"}); err != nil {
		t.Fatal(err)
	}
	if model.count() != 1 {
		t.Fatalf("first build model inputs=%d", model.count())
	}
	peer := reopenResourceEngine(t, e, resources)
	peer.embedder, peer.vectorModel, peer.vectorDimension = model, "test@1", 2
	peer.computationCacheDirectory, peer.embeddingIdentity = e.computationCacheDirectory, e.embeddingIdentity
	peer.embeddingCacheIdentity = e.embeddingCacheIdentity
	docs[0].SourceRepository, docs[0].SourceCommit = "source/current", "current-commit"
	docs[0].ObjectDigest = kernel.Digest("changed-local-state")
	docs[0].Semantic = append(docs[0].Semantic, retrieval.SemanticField{Member: "member/two", Model: "test@1", Text: "unchanged"})
	if err := peer.Rebuild(docs, ports.Meta{Basis: "current"}); err != nil {
		t.Fatal(err)
	}
	if model.count() != 1 {
		t.Fatalf("unchanged member inputs called model after reopen: %d", model.count())
	}
	root, _, err := peer.loadControl()
	if err != nil {
		t.Fatal(err)
	}
	status, body, err := peer.do(http.MethodGet, "/"+root.ActiveIndex+"/_doc/"+url.PathEscape(documentID(string(docs[0].ObjectID))), nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("read rebuilt corpus: %d %s %v", status, body, err)
	}
	var result struct {
		Source osDoc `json:"_source"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result.Source.SourceRepository != "source/current" || result.Source.SourceCommit != "current-commit" || result.Source.ObjectDigest != "changed-local-state" || len(result.Source.SemanticVectors) != 2 {
		t.Fatalf("cached values carried old provenance: %+v", result.Source)
	}
	for i, member := range []string{"member/one", "member/two"} {
		if result.Source.SemanticVectors[i].Member != member || !reflect.DeepEqual(result.Source.SemanticVectors[i].Vector, []float32{9, 1}) {
			t.Fatalf("member vector changed: %+v", result.Source.SemanticVectors)
		}
	}
	docs[0].Semantic[1].Text = "changed"
	if err := peer.Rebuild(docs, ports.Meta{Basis: "changed"}); err != nil {
		t.Fatal(err)
	}
	if model.count() != 2 {
		t.Fatalf("single changed member recomputed unchanged vector: inputs=%d", model.count())
	}
	docs[0].ObjectDigest = "unindexed-change"
	docs[0].SourceCommit = "incremental-commit"
	if err := peer.Apply(docs, nil, ports.Meta{Basis: "incremental"}); err != nil {
		t.Fatal(err)
	}
	if model.count() != 2 {
		t.Fatalf("incremental unindexed change repeated model calls: %d", model.count())
	}
	cache := computationCache{directory: peer.computationCacheDirectory, authority: peer.computationCacheAuthority, identity: peer.embeddingCacheIdentity, dimensions: 2}
	expireComputationEntries(t, cache)
	if _, _, err := cache.claim(nil); err != nil {
		t.Fatal(err)
	}
	root, _, err = peer.loadControl()
	if err != nil {
		t.Fatal(err)
	}
	status, body, err = peer.do(http.MethodGet, "/"+root.ActiveIndex+"/_doc/"+url.PathEscape(documentID(string(docs[0].ObjectID))), nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("cache cleanup removed published corpus: %d %v", status, err)
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result.Source.SourceCommit != "incremental-commit" || len(result.Source.SemanticVectors) != 2 || model.count() != 2 {
		t.Fatalf("published corpus depends on evicted computation: %+v inputs=%d", result.Source, model.count())
	}

}
