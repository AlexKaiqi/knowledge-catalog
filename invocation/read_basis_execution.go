package invocation

import (
	"strconv"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

func prepareObjectReadBasis(cx *Invocation, action string) error {
	reference := cx.Flag("basis")
	if reference == "" {
		return nil
	}
	if action != "object.read" || cx.Command != "object-read" && cx.Command != "object-comparison-side" {
		return kernel.Fail(kernel.ErrUsageInvalid, "read basis is only accepted by object reads")
	}
	for _, name := range []string{"repo", "dataset", "revision", "commit", "ref", "pin", "dataset-file", "source"} {
		if cx.Flag(name) != "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "read basis cannot be combined with --%s", name)
		}
	}
	if SuppliedDataset(cx.Flags) != nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "read basis cannot be combined with a Dataset definition")
	}
	if cx.WS.ReadBases == nil {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "durable read basis storage is unavailable")
	}
	basis, err := cx.WS.ReadBases.Resolve(cx.Context, reference)
	if err != nil {
		return err
	}
	requested := basis
	requested.Reference = cx.Flag("reference")
	if shape := cx.Flag("shape"); shape != "" {
		requested.Shape = shape
	}
	if cx.Command == "object-comparison-side" {
		requested.Shape = cx.Flag("shape")
	}
	before, err := basis.ScopeDigest()
	if err != nil {
		return err
	}
	after, err := requested.ScopeDigest()
	if err != nil || before != after {
		return kernel.Fail(kernel.ErrUsageInvalid, "read basis does not match the requested object selection")
	}
	cx.Flags["shape"] = basis.Shape
	cx.readBasis = &objectReadBasis{fixed: &basis}
	if basis.Temporary {
		if err := restoreTemporaryReadBasis(cx, basis); err != nil {
			return err
		}
	} else if basis.Dataset != "" {
		cx.Flags["dataset"], cx.Flags["revision"] = basis.Dataset, strconv.Itoa(basis.Revision)
	}
	if basis.Temporary || basis.Dataset != "" {
		bound := cx.Flags[referenceBindingFlag].(referenceBinding)
		bound.dataset = true
		cx.Flags[referenceBindingFlag] = bound
	}
	return nil
}

func validateObjectReadBasis(cx *Invocation) error {
	if cx.readBasis == nil || cx.readBasis.fixed == nil {
		return nil
	}
	basis := *cx.readBasis.fixed
	if basis.Temporary || basis.Dataset != "" {
		serving, _, err := OpenServingAt(cx, cx.WS, cx.Flags)
		if err != nil {
			return err
		}
		ref, err := objectmodel.ParseReference(basis.Reference)
		if err != nil {
			return err
		}
		current, err := serving.ReadBasis(ref, basis.Shape)
		if err != nil {
			return err
		}
		if kernel.CanonicalDigest(current) != kernel.CanonicalDigest(basis) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "read basis no longer matches its accepted Dataset publication")
		}
	}
	_, err := reader.OpenReadBasis(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return cx.WS.Reader.Require(id, kernel.ErrObjectRefUnresolved)
	}, basis)
	return err
}

func captureObjectReadBasis(cx *Invocation, ref objectmodel.Reference) (objectmodel.ReadBasis, error) {
	if cx.readBasis.fixed != nil {
		return *cx.readBasis.fixed, nil
	}
	if ServingWorkspace(cx.Flags) {
		serving, _, err := OpenServingAt(cx, cx.WS, cx.Flags)
		if err != nil {
			return objectmodel.ReadBasis{}, err
		}
		return serving.ReadBasis(ref, cx.Flag("shape"))
	}
	candidates, ok := cx.Flags[referenceCandidatesFlag].(map[kernel.RepositoryID]kernel.CommitID)
	if !ok {
		candidates = map[kernel.RepositoryID]kernel.CommitID{ref.Repository: kernel.CommitID(cx.Flag("commit"))}
	}
	dependencies, failures := cx.readBasis.dependencies()
	serving := reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		return cx.WS.Reader.Require(id, kernel.ErrObjectRefUnresolved)
	}, reader.DatasetPin{Repositories: candidates, Dependencies: dependencies, Items: reader.WholeRepositoryItems(candidates)})
	basis, err := serving.ReadBasis(ref, cx.Flag("shape"))
	basis.UnresolvedDependencies = failures
	return basis, err
}

func retainObjectReadBasis(cx *Invocation, ref objectmodel.Reference, result any) (any, error) {
	if cx.WS.ReadBases == nil {
		return result, nil
	}
	if cx.readBasis == nil {
		cx.readBasis = &objectReadBasis{}
	}
	declarations, complete, issue := localReadStateInputs(result)
	if cx.readBasis.fixed != nil && !complete {
		return nil, issue
	}
	basis, err := captureObjectReadBasis(cx, ref)
	if err != nil {
		return nil, err
	}
	reference := cx.Flag("basis")
	if reference == "" {
		if reference, err = cx.WS.ReadBases.Retain(cx.Context, basis); err != nil {
			return nil, err
		}
	}
	scope, err := basis.ScopeDigest()
	if err != nil {
		return nil, err
	}
	state := &objectmodel.ReadState{Algorithm: objectmodel.LocalStateAlgorithm, Scope: scope, Complete: complete}
	if complete {
		state.Digest = objectmodel.LocalStateDigest(declarations)
	}
	switch value := result.(type) {
	case objectmodel.ObjectValue:
		value.Basis, value.State = reference, state
		return value, nil
	case objectmodel.ObservedValue:
		value.Basis, value.State = reference, state
		return value, nil
	case objectmodel.ShapedValue:
		value.Basis, value.State = reference, state
		return value, nil
	}
	return nil, kernel.Fail(kernel.ErrPreconditionFailed, "object read returned an unsupported result envelope")
}

func localReadStateInputs(result any) ([]objectmodel.UnitDeclaration, bool, error) {
	denied := kernel.Fail(kernel.ErrForbidden, "fixed object selection is no longer fully readable")
	switch value := result.(type) {
	case objectmodel.ObjectValue:
		return value.Declarations, len(value.Withheld) == 0, denied
	case objectmodel.ObservedValue:
		return value.Declarations, len(value.Withheld) == 0, denied
	case objectmodel.ShapedValue:
		for _, issue := range value.Errors {
			if value.LocalIncomplete && issue.Code == kernel.ErrObjectIDConflict {
				denied = kernel.Fail(issue.Code, "%s", issue.Message)
				break
			}
		}
		return value.Declarations, len(value.Withheld) == 0 && !value.LocalIncomplete, denied
	}
	return nil, false, kernel.Fail(kernel.ErrPreconditionFailed, "object read returned an unsupported result envelope")
}
