package index_test

import (
	"context"
	"testing"

	"kc/index"
	"kc/kernel"
)

func TestDatasetServingPreparesBuiltInRelationsWithoutCustomSchema(t *testing.T) {
	repo, commit := relationFixture(t)
	engine := &staleCandidateEngine{}
	idx := index.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	controller := newTestController(t, idx, repo)
	if err := controller.PrepareServingBasis(context.Background(), repo, commit); err != nil {
		t.Fatal(err)
	}
	if engine.meta.Basis != commit || engine.meta.State != index.ProjectionStateReady {
		t.Fatalf("published built-in relations have no retained projection: %#v", engine.meta)
	}
}
