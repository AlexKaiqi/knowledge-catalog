package knowledgeapp

import (
	"context"
	"kc/kernel"
	"kc/snapshot/dataset"
)

type DatasetPublication struct {
	Dataset  string
	Revision int
	Sources  []dataset.KnowledgeSetSource
}

type DatasetRegistry interface {
	PrepareKnowledgeSet(string, int, []dataset.KnowledgeSetSource) (dataset.KnowledgeSet, error)
	PublishKnowledgeSet(dataset.KnowledgeSet) (dataset.KnowledgeSet, error)
}

// DatasetPublisher orders authorization, frozen candidate, capability readiness,
// and the single authoritative serving switch. Readiness cannot mutate Dataset service.
type DatasetPublisher struct {
	Registry  DatasetRegistry
	Authorize func(context.Context, DatasetPublication) error
	Prepare   func(context.Context, dataset.KnowledgeSet) error
}

func (e DatasetPublisher) Execute(ctx context.Context, request DatasetPublication) (dataset.KnowledgeSet, error) {
	if e.Registry == nil || e.Authorize == nil || e.Prepare == nil {
		return dataset.KnowledgeSet{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "dataset publication services are incomplete")
	}
	if err := e.Authorize(ctx, request); err != nil {
		return dataset.KnowledgeSet{}, err
	}
	def, err := e.Registry.PrepareKnowledgeSet(request.Dataset, request.Revision, request.Sources)
	if err != nil {
		return dataset.KnowledgeSet{}, err
	}
	if err := e.Prepare(ctx, def); err != nil {
		return dataset.KnowledgeSet{}, err
	}
	if err := ctx.Err(); err != nil {
		return dataset.KnowledgeSet{}, err
	}
	return e.Registry.PublishKnowledgeSet(def)
}
