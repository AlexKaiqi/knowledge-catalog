package reader

import (
	"fmt"

	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
)

// BindingIdentity is one selected binding unit of a fixed Dataset revision and
// the result identity its frozen ResourceDescriptor declares. Reason is set,
// and Identity left empty, when the descriptor cannot be interpreted in this
// scope; the consuming capability decides what that excludes. Building it reads
// metadata-only binding coordinates and descriptors, never a source.
type BindingIdentity struct {
	Repository kernel.RepositoryID
	Address    objectmodel.Address
	Schema     objectmodel.ObjectID
	Descriptor objectmodel.ObjectID
	Identity   objectmodel.ResultIdentity
	Reason     string
}

// BindingIdentities lists the selected binding units whose Schema belongs to
// the Dataset namespace, in Repository and address order.
func (s *Serving) BindingIdentities() ([]BindingIdentity, error) {
	out := []BindingIdentity{}
	resolved := map[objectmodel.ObjectID]BindingIdentity{}
	err := s.eachRepository(func(repositoryID kernel.RepositoryID, commit kernel.CommitID, repo objectmodel.Repository) error {
		if !datasetHasRepository(s.pin.Items, repositoryID) {
			return nil
		}
		member, ok := repo.(*datasetRepository)
		if !ok {
			return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository %s is not a Dataset-scoped member", repositoryID)
		}
		coordinates, err := member.scopedBindingCoordinates(commit)
		if err != nil {
			return err
		}
		for _, coordinate := range coordinates {
			if coordinate.Descriptor == "" {
				return kernel.Fail(kernel.ErrCapabilityUnsatisfied,
					"binding unit coordinates predate descriptor references; explicitly rebuild locators")
			}
			known, seen := resolved[coordinate.Descriptor]
			if !seen {
				known, err = s.descriptorIdentity(coordinate.Descriptor)
				if err != nil {
					return err
				}
				resolved[coordinate.Descriptor] = known
			}
			out = append(out, BindingIdentity{Repository: repositoryID, Address: coordinate.Address, Schema: coordinate.Schema,
				Descriptor: coordinate.Descriptor, Identity: known.Identity, Reason: known.Reason})
		}
		return nil
	})
	return out, err
}

// scopedBindingCoordinates keeps selected binding units whose Schema is in the
// member's namespace, the same intersection BindingSchemaObjectIDs answers.
func (r *datasetRepository) scopedBindingCoordinates(at kernel.CommitID) ([]repofile.BindingCoordinate, error) {
	allowed, err := r.SchemaObjectIDs(at)
	if err != nil || len(allowed) == 0 {
		return nil, err
	}
	scoped := make(map[objectmodel.ObjectID]bool, len(allowed))
	for _, id := range allowed {
		scoped[id] = true
	}
	if _, ok := r.Repository.(bindingCoordinatesReader); !ok {
		// Without Reader-owned coordinates only a repository-wide proof that
		// no unit is bound may stand in; any binding fails closed below.
		if locator, native := r.Repository.(objectmodel.BindingLocator); native {
			ids, err := locator.BindingSchemaObjectIDs(at)
			if err != nil {
				return nil, err
			}
			if len(ids) == 0 {
				return nil, nil
			}
		}
	}
	coordinates, err := r.selectedBindingCoordinates(at)
	if err != nil {
		return nil, err
	}
	out := make([]repofile.BindingCoordinate, 0, len(coordinates))
	for _, coordinate := range coordinates {
		if scoped[coordinate.Schema] {
			out = append(out, coordinate)
		}
	}
	return out, nil
}

func (s *Serving) descriptorIdentity(ref objectmodel.ObjectID) (BindingIdentity, error) {
	value, err := s.ReadDescriptor(ref)
	switch kernel.CodeOf(err) {
	case "":
	case kernel.ErrObjectRefUnresolved, kernel.ErrObjectIDConflict:
		return BindingIdentity{Reason: err.Error()}, nil
	default:
		return BindingIdentity{}, err
	}
	descriptor, err := objectmodel.ParseResourceDescriptor(ref, value.Value)
	if err != nil {
		return BindingIdentity{Reason: fmt.Sprintf("descriptorRef %s is not a valid ResourceDescriptor: %v", ref, err)}, nil
	}
	return BindingIdentity{Identity: descriptor.Identity}, nil
}

// ReadDescriptor resolves one descriptorRef inside this fixed scope. The
// descriptor must be selected exactly once; one the scope does not select
// resolves at the dependency version frozen with the Dataset revision without
// entering the consumer-visible scope.
func (s *Serving) ReadDescriptor(ref objectmodel.ObjectID) (objectmodel.FederatedValue, error) {
	values, err := s.Read(ref)
	if err != nil && kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		return objectmodel.FederatedValue{}, err
	}
	if len(values) == 0 {
		if values, err = s.ReadDependency(ref); err != nil {
			return objectmodel.FederatedValue{}, err
		}
	}
	switch {
	case len(values) == 0:
		return objectmodel.FederatedValue{}, kernel.Fail(kernel.ErrObjectRefUnresolved,
			"descriptorRef %s is not readable in this request scope", ref)
	case len(values) > 1:
		return objectmodel.FederatedValue{}, kernel.Fail(kernel.ErrObjectIDConflict,
			"descriptorRef %s is held by several repositories in this request scope", ref)
	}
	return values[0], nil
}
