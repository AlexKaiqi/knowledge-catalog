package control_test

import (
	"context"
	fixtures "kc/declarativeindex/internal/testfixture"
	"testing"

	query "kc/declarativeindex/projection"
	index "kc/discovery/projection"
	"kc/kernel"
)

func TestDatasetServingPreparesBuiltInRelationsWithoutCustomSchema(t *testing.T) {
	repo, commit := fixtures.RelationFixture(t)
	engine := &fixtures.StateEngine{}
	idx := query.NewIndexEngine("", func(string, kernel.RepositoryID) (index.Engine, error) { return engine, nil })
	t.Cleanup(func() { _ = idx.Close() })
	controller := newTestController(t, idx, repo)
	if err := controller.PrepareServingBasis(context.Background(), repo, commit); err != nil {
		t.Fatal(err)
	}
	if engine.Meta.Basis != commit || engine.Meta.State != index.ProjectionStateReady {
		t.Fatalf("published built-in relations have no retained projection: %#v", engine.Meta)
	}
}
