package service

import (
	"context"
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
)

// ShapeReadExecutor reads a GraphQL shape as independent maintenance-level
// reads. Each selection is planned from coordinates and read on its own, so
// unselected units are never read or hydrated; fields are trimmed from each
// freshly read unit before the result is assembled.
type ShapeReadExecutor struct {
	// Read performs one ordinary reference read (URN plus Pointer) in the
	// caller's already authorized scope.
	Read func(context.Context, objectmodel.Reference) (objectmodel.ObservedValue, error)
	// Schema resolves the Schema a read unit adopted, inside the same scope.
	// A nil definition means the Schema is not interpretable here (open).
	Schema func(context.Context, kernel.RepositoryID, kernel.CommitID, string) (*objectmodel.SchemaDefinition, error)
}

type ShapeReadRequest struct {
	Reference objectmodel.Reference
	Shape     string
}

func (e ShapeReadExecutor) Execute(ctx context.Context, request ShapeReadRequest) (objectmodel.ShapedValue, error) {
	if e.Read == nil || e.Schema == nil {
		return objectmodel.ShapedValue{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "shaped read services are incomplete")
	}
	if request.Reference.Locator != "" {
		return objectmodel.ShapedValue{}, kernel.Fail(kernel.ErrUsageInvalid, "reference already selects content; do not add a shape")
	}
	raw, err := objectmodel.FormatReference(request.Reference)
	if err != nil {
		return objectmodel.ShapedValue{}, err
	}
	shape, err := objectmodel.ParseShape(request.Shape)
	if err != nil {
		return objectmodel.ShapedValue{}, err
	}
	out := objectmodel.ShapedValue{Reference: raw, Shape: request.Shape, ObjectID: request.Reference.Object, Data: map[string]any{}}
	acc := shapeAccumulator{out: &out, sources: map[kernel.RepositoryID]int{}, withheld: map[kernel.RepositoryID]bool{}}
	for _, selection := range shape.Selections() {
		if err := ctx.Err(); err != nil {
			return objectmodel.ShapedValue{}, err
		}
		if err := e.readSelection(ctx, request.Reference, selection, &acc); err != nil {
			return objectmodel.ShapedValue{}, err
		}
	}
	return out, nil
}

func (e ShapeReadExecutor) readSelection(ctx context.Context, root objectmodel.Reference, selection objectmodel.ShapeSelection, acc *shapeAccumulator) error {
	ref := root
	ref.Locator = selection.Locator()
	path := selection.Path()
	observed, err := e.Read(ctx, ref)
	if err != nil {
		if !shapeLocatedFailure(kernel.CodeOf(err)) {
			return err
		}
		if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
			acc.out.LocalIncomplete = true
		}
		acc.fail(path, kernel.CodeOf(err), err.Error())
		return nil
	}
	members, err := selectedMembers(selection, observed.Declarations)
	if err != nil {
		return err
	}
	if err := e.checkSchemas(ctx, selection, observed); err != nil {
		return err
	}
	acc.adopt(selection.Aspect, observed)
	if len(observed.Failures) > 0 {
		acc.fail(path, observed.Failures[0].Code, observed.Failures[0].Message)
		return nil
	}
	if !members || selection.Keyed {
		value, errs := objectmodel.ProjectShapeFields(observed.Value, selection.Fields, path)
		objectmodel.SetShapePath(acc.out.Data, path, value)
		acc.out.Errors = append(acc.out.Errors, errs...)
		return nil
	}
	collection, _ := observed.Value.(map[string]any)
	projected := make(map[string]any, len(collection))
	for key, member := range collection {
		value, errs := objectmodel.ProjectShapeFields(member, selection.Fields, append(path, key))
		projected[key] = value
		acc.out.Errors = append(acc.out.Errors, errs...)
	}
	objectmodel.SetShapePath(acc.out.Data, path, projected)
	return nil
}

// selectedMembers reports whether the read units are Members, and rejects
// `keys` on an Aspect that is maintained as one record.
func selectedMembers(selection objectmodel.ShapeSelection, declarations []objectmodel.UnitDeclaration) (bool, error) {
	members := false
	for _, declaration := range declarations {
		if declaration.Address.Kind == objectmodel.KindMember {
			members = true
		} else if selection.Keyed {
			return false, kernel.Fail(kernel.ErrUsageInvalid, "aspect %s has no Members; remove keys from the shape", selection.Aspect)
		}
	}
	return members, nil
}

// checkSchemas rejects the whole request when a closed adopted Schema does not
// declare a selected top-level field. Uninterpretable Schemas stay open.
func (e ShapeReadExecutor) checkSchemas(ctx context.Context, selection objectmodel.ShapeSelection, observed objectmodel.ObservedValue) error {
	if selection.Fields == nil {
		return nil
	}
	checked := map[string]bool{}
	for _, declaration := range observed.Declarations {
		if declaration.SchemaRef == "" || checked[declaration.SchemaRef] {
			continue
		}
		checked[declaration.SchemaRef] = true
		schema, err := e.Schema(ctx, observed.Repository, observed.Commit, declaration.SchemaRef)
		if err != nil {
			return err
		}
		if schema == nil {
			continue
		}
		if mismatches := objectmodel.ShapeSchemaMismatches(selection.Fields, *schema); len(mismatches) > 0 {
			return kernel.Fail(kernel.ErrUsageInvalid, "shape does not match schema %s on aspect %s: %s", schema.ObjectID, selection.Aspect, strings.Join(mismatches, "; "))
		}
	}
	return nil
}

// Missing, withheld, conflicting and out-of-scope content is reported at its
// path; every other failure rejects the whole shaped read.
func shapeLocatedFailure(code kernel.ErrorCode) bool {
	switch code {
	case kernel.ErrObjectRefUnresolved, kernel.ErrForbidden, kernel.ErrObjectIDConflict:
		return true
	}
	return false
}

// AdoptedSchema reads the Schema a unit adopted through the request scope's
// Repository lookup. A Schema outside the scope is uninterpretable here and
// returns nil instead of being read from a Repository the scope excludes.
func AdoptedSchema(lookup func(kernel.RepositoryID) (objectmodel.Repository, error), repository kernel.RepositoryID, commit kernel.CommitID, schemaRef string) (*objectmodel.SchemaDefinition, error) {
	ref, ok := objectmodel.ParseSchemaRef(schemaRef)
	if !ok {
		return nil, nil
	}
	if ref.Repository != "" {
		repository = ref.Repository
	}
	if ref.Commit != "" {
		commit = ref.Commit
	}
	repo, err := lookup(repository)
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	value, err := repo.Read(ref.Object, commit)
	if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	schema, err := objectmodel.ParseSchemaDefinition(ref.Object, value.Value)
	if err != nil {
		return nil, err
	}
	return &schema, nil
}

type shapeAccumulator struct {
	out      *objectmodel.ShapedValue
	sources  map[kernel.RepositoryID]int
	withheld map[kernel.RepositoryID]bool
}

func (a *shapeAccumulator) fail(path []string, code kernel.ErrorCode, message string) {
	objectmodel.SetShapePath(a.out.Data, path, nil)
	a.out.Errors = append(a.out.Errors, objectmodel.ShapeError{Path: append([]string(nil), path...), Code: code, Message: message})
}

func (a *shapeAccumulator) adopt(aspect string, observed objectmodel.ObservedValue) {
	a.out.Declarations = append(a.out.Declarations, observed.Declarations...)
	a.out.Observations = append(a.out.Observations, observed.Observations...)
	a.out.Failures = append(a.out.Failures, observed.Failures...)
	sources := observed.Sources
	if len(sources) == 0 && observed.Repository != "" {
		sources = []objectmodel.SourceBasis{{Repository: observed.Repository, Commit: observed.Commit}}
	}
	for _, source := range sources {
		index, seen := a.sources[source.Repository]
		if !seen {
			index = len(a.out.Sources)
			a.sources[source.Repository] = index
			a.out.Sources = append(a.out.Sources, objectmodel.SourceBasis{Repository: source.Repository, Commit: source.Commit})
		}
		a.out.Sources[index].Aspects = appendAspect(a.out.Sources[index].Aspects, aspect)
	}
	for _, repository := range observed.Withheld {
		if !a.withheld[repository] {
			a.withheld[repository] = true
			a.out.Withheld = append(a.out.Withheld, repository)
		}
	}
}

func appendAspect(aspects []string, aspect string) []string {
	for _, existing := range aspects {
		if existing == aspect {
			return aspects
		}
	}
	return append(aspects, aspect)
}
