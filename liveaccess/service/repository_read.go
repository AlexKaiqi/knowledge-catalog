package service

import (
	"context"

	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
)

// ReadRequest names content on one Repository and already fixed commit. It is
// a neutral read contract: no task executor or transport implementation enters
// the value path.
type ReadRequest struct {
	Commit    kernel.CommitID
	Object    objectmodel.ObjectID
	Reference *objectmodel.Reference
	Address   *objectmodel.Address
	Selector  *objectmodel.AspectSelector
	Hydrator  objectmodel.Hydrator
	// Descriptors resolves descriptorRef in the caller's request scope; nil
	// limits it to the read Repository commit.
	Descriptors liveaccess.DescriptorScope
}

// ReadRepository reads complete selected units before adopting external values
// and projecting a dynamic leaf. A leaf absent from the fixed object value
// does not prevent a declared unit's source lookup.
func ReadRepository(ctx context.Context, repo objectmodel.Repository, selection ReadRequest, state liveaccess.StateLookup, request objectmodel.RequestContext) (objectmodel.ObservedValue, error) {
	service, err := repositoryService(repo, selection.Commit, state, request, false)
	if err != nil {
		return objectmodel.ObservedValue{}, err
	}
	service.WithDescriptors(selection.Descriptors)
	if err := validateRepositoryRead(repo.ID(), selection); err != nil {
		return objectmodel.ObservedValue{}, err
	}
	service.declarations.SetHydrator(selection.Hydrator)
	var values []objectmodel.ObservedValue
	switch {
	case selection.Reference != nil:
		values, err = service.ReadReference(ctx, *selection.Reference)
	case selection.Address != nil:
		values, err = service.ReadAddress(ctx, *selection.Address)
	default:
		values, err = service.Read(ctx, selection.Object, selection.Selector)
	}
	if err != nil {
		return objectmodel.ObservedValue{}, err
	}
	if len(values) != 1 {
		return objectmodel.ObservedValue{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "Repository read did not select one published value")
	}
	return values[0], nil
}

func validateRepositoryRead(repository kernel.RepositoryID, request ReadRequest) error {
	if request.Reference != nil {
		ref := *request.Reference
		if ref.Repository != repository || request.Object != "" && request.Object != ref.Object || request.Address != nil || request.Selector != nil {
			return kernel.Fail(kernel.ErrUsageInvalid, "reference conflicts with separate object selection")
		}
		_, err := objectmodel.FormatReference(ref)
		return err
	}
	if request.Address != nil {
		if request.Object != "" && request.Object != request.Address.ObjectID {
			return kernel.Fail(kernel.ErrUsageInvalid, "read address does not match its object")
		}
		return objectmodel.AssertWritable(*request.Address)
	}
	if request.Object == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "read requires an object")
	}
	return nil
}
