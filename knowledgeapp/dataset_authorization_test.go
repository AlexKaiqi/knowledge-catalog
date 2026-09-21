package knowledgeapp

import (
	"context"
	"testing"

	"kc/kernel"
	"kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
	"kc/retrieval"
)

func TestDatasetExecutorsAuthorizeBeforeResolution(t *testing.T) {
	deny := func(context.Context) error { return kernel.Fail(kernel.ErrForbidden, "denied") }
	read := DatasetReadExecutor{Authorize: deny,
		Resolve: func(context.Context) (*knowledgeserving.Service, error) {
			t.Fatal("denied read resolved data")
			return nil, nil
		},
		Deliver: func(_ context.Context, v []knowledgeserving.ReadResult) ([]knowledgeserving.ReadResult, error) {
			t.Fatal("denied read delivered data")
			return v, nil
		},
	}
	if _, err := read.Execute(context.Background(), DatasetReadRequest{Object: "one"}); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatal(err)
	}
	search := DatasetSearchExecutor{Authorize: deny, Repositories: searchLookup{repo: &searchRepository{}}, Projection: &searchProjection{},
		Resolve: func(context.Context) (*reader.Serving, *knowledgeserving.Service, error) {
			t.Fatal("denied search resolved data")
			return nil, nil, nil
		},
		Deliver: func(_ context.Context, v retrieval.KnowledgeHit) (retrieval.KnowledgeHit, error) {
			t.Fatal("denied search delivered data")
			return v, nil
		},
	}
	if _, err := search.Execute(context.Background(), retrieval.SearchRequest{}); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatal(err)
	}
}
