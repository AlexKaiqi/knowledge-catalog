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

func compileValue(repo objectmodel.Repository, value objectmodel.ObjectValue, spec retrieval.AccessSpec) (ports.CompiledDoc, bool, error) {
	if objectmodel.IsSchemaObject(value.Address.ObjectID) {
		return ports.CompiledDoc{}, false, nil
	}
	doc, err := compileProjectionDocument(repo, value, spec)
	if err != nil {
		return ports.CompiledDoc{}, false, err
	}
	return doc, true, nil
}
