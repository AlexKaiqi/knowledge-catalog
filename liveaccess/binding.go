package liveaccess

import (
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

// ResolvedBinding is the complete stable access declaration resolved at one
// Repository commit: the unit's valueSource plus the fixed ResourceDescriptor
// version it references. Credentials and observation values stay out of it.
//
// DeclarationDigest is the declaration version: it combines the unit's
// declaration digest with the content digest of the adopted
// ResourceDescriptor, so a change to either is a different declaration.
type ResolvedBinding struct {
	Repository           kernel.RepositoryID                     `json:"repository"`
	DeclarationCommit    kernel.CommitID                         `json:"declarationCommit"`
	Address              objectmodel.Address                     `json:"address"`
	DeclarationDigest    kernel.Digest                           `json:"declarationDigest"`
	Mode                 objectmodel.BindingMode                 `json:"mode"`
	Runtime              string                                  `json:"runtime"`
	Protocol             string                                  `json:"protocol"`
	Identity             objectmodel.ResultIdentity              `json:"identity"`
	Operations           map[string]objectmodel.BindingOperation `json:"operations"`
	SchemaRef            string                                  `json:"schemaRef,omitempty"`
	Origin               string                                  `json:"origin"`
	DescriptorRef        objectmodel.ObjectID                    `json:"descriptorRef"`
	DescriptorRepository kernel.RepositoryID                     `json:"descriptorRepository"`
	DescriptorCommit     kernel.CommitID                         `json:"descriptorCommit"`
	DescriptorDigest     kernel.Digest                           `json:"descriptorDigest"`
}

// DescriptorAt is one ResourceDescriptor version visible in a request scope.
type DescriptorAt struct {
	Repository kernel.RepositoryID
	Commit     kernel.CommitID
	Value      any
}

// DescriptorScope resolves a descriptorRef inside the caller's request scope.
// A Dataset read resolves within its fixed pin; a direct Repository read
// resolves within the Repositories the caller may read.
type DescriptorScope func(objectmodel.ObjectID) (DescriptorAt, error)

// ServingDescriptors resolves descriptorRef inside one fixed Serving scope.
// The descriptor must be selected exactly once by that scope; a descriptor the
// scope does not select resolves at the dependency version frozen with the
// Dataset revision, without entering the consumer-visible scope.
func ServingDescriptors(scope *reader.Serving) DescriptorScope {
	return func(id objectmodel.ObjectID) (DescriptorAt, error) {
		value, err := scope.ReadDescriptor(id)
		if err != nil {
			return DescriptorAt{}, err
		}
		return DescriptorAt{Repository: value.Repository, Commit: value.Commit, Value: value.Value}, nil
	}
}

// RepositoryDescriptors resolves descriptorRef inside one Repository commit:
// the scope of a declaration read with no wider request scope.
func RepositoryDescriptors(repo objectmodel.Repository, commit kernel.CommitID) DescriptorScope {
	return func(id objectmodel.ObjectID) (DescriptorAt, error) {
		value, err := repo.Read(id, commit)
		if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
			return DescriptorAt{}, kernel.Fail(kernel.ErrObjectRefUnresolved,
				"descriptorRef %s is not readable in %s at %s", id, repo.ID(), commit)
		}
		if err != nil {
			return DescriptorAt{}, err
		}
		return DescriptorAt{Repository: repo.ID(), Commit: commit, Value: value.Value}, nil
	}
}

// ResolveRepoBinding reads one stored unit declaration at a fixed commit and
// resolves it through the single declaration path.
func ResolveRepoBinding(repo objectmodel.Repository, commit kernel.CommitID, address objectmodel.Address, descriptors DescriptorScope) (ResolvedBinding, error) {
	if err := validateBindingAddress(address); err != nil {
		return ResolvedBinding{}, err
	}
	resolution, err := repo.ResolveAddress(address, commit)
	if err != nil {
		return ResolvedBinding{}, err
	}
	if resolution.Status != objectmodel.StatusResolved {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrObjectRefUnresolved,
			"binding address %s is not resolved at %s", objectmodel.AddressKey(address), commit)
	}
	return ResolveDeclaration(descriptors, repo.ID(), commit, objectmodel.UnitDeclaration{
		Address: address, ValueSource: resolution.ValueSource,
		SchemaRef: resolution.SchemaRef, DeclarationDigest: resolution.DeclarationDigest,
	})
}

// ResolveDeclaration is the only LiveAccess declaration path: one unit
// valueSource plus the ResourceDescriptor its descriptorRef selects in the
// request scope.
func ResolveDeclaration(descriptors DescriptorScope, repository kernel.RepositoryID, commit kernel.CommitID, declaration objectmodel.UnitDeclaration) (ResolvedBinding, error) {
	address := declaration.Address
	if err := validateBindingAddress(address); err != nil {
		return ResolvedBinding{}, err
	}
	if err := objectmodel.AssertWritable(address); err != nil {
		return ResolvedBinding{}, err
	}
	source := declaration.ValueSource
	if !source.IsBinding() {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"address %s declares no binding valueSource", objectmodel.AddressKey(address))
	}
	if err := objectmodel.ValidateValueSource(source); err != nil {
		return ResolvedBinding{}, err
	}
	if descriptors == nil {
		return ResolvedBinding{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"address %s has no request scope for its ResourceDescriptor", objectmodel.AddressKey(address))
	}
	ref := source.Binding.DescriptorRef
	at, err := descriptors(ref)
	if err != nil {
		return ResolvedBinding{}, err
	}
	descriptor, err := objectmodel.ParseResourceDescriptor(ref, at.Value)
	if err != nil {
		return ResolvedBinding{}, err
	}
	unitDigest := declaration.DeclarationDigest
	if unitDigest == "" {
		unitDigest = objectmodel.DeclarationDigest(declaration.SchemaRef, source)
	}
	descriptorDigest := kernel.CanonicalDigest(at.Value)
	return ResolvedBinding{
		Repository:           repository,
		DeclarationCommit:    commit,
		Address:              address,
		DeclarationDigest:    objectmodel.BindingDeclarationDigest(unitDigest, descriptorDigest),
		Mode:                 descriptor.Mode,
		Runtime:              descriptor.Runtime,
		Protocol:             descriptor.Protocol,
		Identity:             descriptor.Identity,
		Operations:           copyOperations(descriptor.Access),
		SchemaRef:            declaration.SchemaRef,
		Origin:               descriptor.Origin,
		DescriptorRef:        ref,
		DescriptorRepository: at.Repository,
		DescriptorCommit:     at.Commit,
		DescriptorDigest:     descriptorDigest,
	}, nil
}

func validateBindingAddress(address objectmodel.Address) error {
	if strings.TrimSpace(address.AspectName) == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "resolve-binding requires --aspect")
	}
	if objectmodel.IsSchemaObject(address.ObjectID) {
		return kernel.Fail(kernel.ErrUsageInvalid, "LiveAccess targets an entity, not a schema object")
	}
	return nil
}

func copyOperations(source map[string]objectmodel.BindingOperation) map[string]objectmodel.BindingOperation {
	out := make(map[string]objectmodel.BindingOperation, len(source))
	for name, operation := range source {
		out[name] = operation
	}
	return out
}
