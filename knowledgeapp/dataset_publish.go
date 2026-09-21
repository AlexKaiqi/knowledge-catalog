package knowledgeapp

import (
	"context"
	"kc/catalog"
	"kc/kernel"
)

type DatasetPublication struct {
	Dataset  string
	Revision int
	Sources  []catalog.KnowledgeSetSource
}

type DatasetRegistry interface {
	PrepareKnowledgeSet(string, int, []catalog.KnowledgeSetSource) (catalog.KnowledgeSet, error)
	PublishKnowledgeSet(catalog.KnowledgeSet) (catalog.KnowledgeSet, error)
}

// DatasetPublisher orders authorization, frozen candidate, capability readiness,
// and the single authoritative serving switch. Readiness cannot mutate Catalog.
type DatasetPublisher struct {
	Registry  DatasetRegistry
	Authorize func(context.Context, DatasetPublication) error
	Prepare   func(context.Context, catalog.KnowledgeSet) error
}

func (e DatasetPublisher) Execute(ctx context.Context, request DatasetPublication) (catalog.KnowledgeSet, error) {
	if e.Registry == nil || e.Authorize == nil || e.Prepare == nil {
		return catalog.KnowledgeSet{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset publication services are incomplete")
	}
	if err := e.Authorize(ctx, request); err != nil {
		return catalog.KnowledgeSet{}, err
	}
	def, err := e.Registry.PrepareKnowledgeSet(request.Dataset, request.Revision, request.Sources)
	if err != nil {
		return catalog.KnowledgeSet{}, err
	}
	if err := e.Prepare(ctx, def); err != nil {
		return catalog.KnowledgeSet{}, err
	}
	if err := ctx.Err(); err != nil {
		return catalog.KnowledgeSet{}, err
	}
	return e.Registry.PublishKnowledgeSet(def)
}
