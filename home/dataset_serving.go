package home

import (
	"context"
	"kc/catalog"
	"kc/kernel"
	"kc/knowledge"
)

// PrepareDataset is application assembly, not a Catalog knowledge dependency.
func (ws *Home) PrepareDataset(ctx context.Context, def catalog.KnowledgeSet) error {
	if ws.Projection == nil || ws.Stores.Index == "none" {
		return nil
	}
	seen := map[kernel.RepositoryID]bool{}
	for _, source := range def.Sources {
		if seen[source.Repository] {
			continue
		}
		seen[source.Repository] = true
		repo, err := ws.Reader.Require(source.Repository, kernel.ErrCapabilityUnsatisfied)
		if err != nil {
			return err
		}
		if err := ws.Projection.PrepareServingBasis(ctx, repo, source.Commit); err != nil {
			return err
		}
	}
	return nil
}

// This independent recovery lane follows Catalog serving releases rather than
// mistaking Repository HEAD for the Dataset's accepted basis. Reconciliation
// runs on startup and periodically even when no source notification was sent.
type datasetProjectionConsumer struct {
	catalogs   []*catalog.Catalog
	controller interface {
		PrepareServingBasis(context.Context, knowledge.Repository, kernel.CommitID) error
	}
}

func (datasetProjectionConsumer) ID() string { return "dataset-serving/v1" }
func (d datasetProjectionConsumer) Reconcile(ctx context.Context, repo knowledge.Repository, _ kernel.CommitID) error {
	seen := map[kernel.CommitID]bool{}
	for _, cat := range d.catalogs {
		state := cat.DumpState()
		if state.Archived || !cat.HasRepository(repo.ID()) {
			continue
		}
		for _, def := range state.KnowledgeSets {
			if def.Retired {
				continue
			}
			for _, source := range def.Sources {
				if source.Repository != repo.ID() || seen[source.Commit] {
					continue
				}
				seen[source.Commit] = true
				if err := d.controller.PrepareServingBasis(ctx, repo, source.Commit); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
