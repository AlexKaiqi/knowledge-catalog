package invocation

import (
	"context"

	"kc/kernel"
	objectmodel "kc/object"
	objectservice "kc/object/service"
)

// readShape runs a GraphQL shape as one ordinary reference read per selected
// Aspect or Member in the already bound scope. Schemas are interpreted only
// from Repositories that scope admits.
func readShape(cx *Invocation, reference objectmodel.Reference, shape string) (any, error) {
	lookup := shapeSchemaLookup(cx)
	return (objectservice.ShapeReadExecutor{
		Read: func(_ context.Context, ref objectmodel.Reference) (objectmodel.ObservedValue, error) {
			return readOneReference(cx, ref)
		},
		Schema: func(_ context.Context, repository kernel.RepositoryID, commit kernel.CommitID, schemaRef string) (*objectmodel.SchemaDefinition, error) {
			return objectservice.AdoptedSchema(lookup, repository, commit, schemaRef)
		},
	}).Execute(cx.Context, objectservice.ShapeReadRequest{Reference: reference, Shape: shape})
}

func shapeSchemaLookup(cx *Invocation) func(kernel.RepositoryID) (objectmodel.Repository, error) {
	if !ServingWorkspace(cx.Flags) {
		return func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			return cx.WS.Reader.Require(id, kernel.ErrObjectRefUnresolved)
		}
	}
	return func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		serving, _, err := OpenServingAt(cx, cx.WS, cx.Flags)
		if err != nil {
			return nil, err
		}
		return serving.Member(id)
	}
}
