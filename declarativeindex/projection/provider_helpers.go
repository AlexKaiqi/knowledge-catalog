package projection

import (
	"context"
	retrieval "kc/discovery"
	ports "kc/discovery/projection"
	objectmodel "kc/object"
)

func loadMetaContext(ctx context.Context, engine ports.Engine) (ports.Meta, error) {
	if err := ctx.Err(); err != nil {
		return ports.Meta{}, err
	}
	if loader, ok := engine.(ports.ContextMetaLoader); ok {
		return loader.LoadMetaContext(ctx)
	}
	meta, err := engine.LoadMeta()
	if ctx.Err() != nil {
		return ports.Meta{}, ctx.Err()
	}
	return meta, err
}

func compileValueComputed(ctx context.Context, engine ports.Engine, repo objectmodel.Repository, value objectmodel.ObjectValue, spec retrieval.AccessSpec) (ports.CompiledDoc, bool, error) {
	if objectmodel.IsSchemaObject(value.Address.ObjectID) {
		return ports.CompiledDoc{}, false, nil
	}
	computations, _ := engine.(retrieval.MemberComputations)
	doc, err := compileProjectionDocumentComputed(ctx, repo, value, nil, spec, computations)
	return doc, err == nil, err
}

func compileObservedValue(ctx context.Context, engine ports.Engine, repo objectmodel.Repository, value objectmodel.ObjectValue, observations []objectmodel.UnitObservation, spec retrieval.AccessSpec) (ports.CompiledDoc, error) {
	computations, _ := engine.(retrieval.MemberComputations)
	return compileProjectionDocumentComputed(ctx, repo, value, observations, spec, computations)
}
