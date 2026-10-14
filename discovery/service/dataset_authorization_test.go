package service

import (
	"context"
	retrieval "kc/discovery"
	"kc/kernel"

	"kc/object/reader"
	"testing"
)

func TestDatasetSearchAuthorizesBeforeResolution(t *testing.T) {
	deny := func(context.Context) error { return kernel.Fail(kernel.ErrForbidden, "denied") }
	search := DatasetSearchExecutor{Authorize: deny, Repositories: searchLookup{repo: &searchRepository{}}, Projection: &searchProjection{},
		Resolve: func(context.Context) (*reader.Serving, LogicalHydrator, error) {
			t.Fatal("denied search resolved data")
			return nil, nil, nil
		},
		Deliver: func(_ context.Context, v retrieval.ObjectHit) (retrieval.ObjectHit, error) {
			t.Fatal("denied search delivered data")
			return v, nil
		},
	}
	if _, err := search.Execute(context.Background(), retrieval.SearchRequest{}); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatal(err)
	}
}
