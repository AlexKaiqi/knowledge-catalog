package service

import (
	"context"
	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
	"kc/object/reader"
)

// objectmodel.ObservedValue retains the immutable declaration coordinates while Value holds
// the logical, hydrated result. Observations makes it impossible to mistake a
// dynamic value for bytes frozen by Commit.

type Service struct {
	declarations   *reader.Serving
	descriptors    liveaccess.DescriptorScope
	state          liveaccess.StateLookup
	request        objectmodel.RequestContext
	strict         bool
	interpretation objectmodel.Repository
}

func Open(declarations *reader.Serving, state liveaccess.StateLookup, identity kernel.IdentityContext) *Service {
	return OpenRequest(declarations, state, objectmodel.RequestContext{Identity: identity})
}

// OpenRequest adds the correlation metadata that an out-of-process runtime
// needs. The identity has already crossed the Object Server's trusted
// authentication boundary; this package does not authenticate it again.
// ResourceDescriptors resolve inside the same fixed declaration scope unless
// WithDescriptors names the caller's request scope.
func OpenRequest(declarations *reader.Serving, state liveaccess.StateLookup, request objectmodel.RequestContext) *Service {
	return &Service{declarations: declarations, descriptors: liveaccess.ServingDescriptors(declarations), state: state, request: request}
}

// WithDescriptors replaces the scope that resolves descriptorRef. A nil scope
// keeps the declaration scope.
func (s *Service) WithDescriptors(descriptors liveaccess.DescriptorScope) *Service {
	if descriptors != nil {
		s.descriptors = descriptors
	}
	return s
}

// HydrateRepositoryValue applies the same logical READ semantics to one raw
// ObjectValue at a fixed Repository commit. Projection control uses this
// entry point so exact READ and dynamic indexing cannot grow separate Binding
// implementations. A nil descriptors scope resolves descriptorRef only inside
// the same Repository commit.
func HydrateRepositoryValue(ctx context.Context, repo objectmodel.Repository, value objectmodel.ObjectValue, state liveaccess.StateLookup, request objectmodel.RequestContext, descriptors liveaccess.DescriptorScope) (objectmodel.ObservedValue, error) {
	return hydrateRepository(ctx, repo, value, state, request, nil, true, descriptors)
}

// HydrateRepositoryRead composes an already read, fixed Repository result with
// external units. Address and Pointer selections preserve the original shape;
// one external failure is explained only at that unit's selected position.
func HydrateRepositoryRead(ctx context.Context, repo objectmodel.Repository, value objectmodel.ObjectValue, state liveaccess.StateLookup, request objectmodel.RequestContext, selector *objectmodel.AspectSelector) (objectmodel.ObservedValue, error) {
	return hydrateRepository(ctx, repo, value, state, request, selector, false, nil)
}

func hydrateRepository(ctx context.Context, repo objectmodel.Repository, value objectmodel.ObjectValue, state liveaccess.StateLookup, request objectmodel.RequestContext, selector *objectmodel.AspectSelector, strict bool, descriptors liveaccess.DescriptorScope) (objectmodel.ObservedValue, error) {
	service, err := repositoryService(repo, value.Commit, state, request, strict)
	if err != nil {
		return objectmodel.ObservedValue{}, err
	}
	service.WithDescriptors(descriptors)
	raw := objectmodel.FederatedValue{
		ObjectRef: value.ObjectRef, Repository: repo.ID(), Commit: value.Commit,
		ObjectID: value.Address.ObjectID, Address: value.Address, Value: value.Value,
		Provenance: value.Provenance, Units: value.Units, Declarations: value.Declarations,
		Reference: value.Reference, Locator: value.Locator, Digest: value.Digest,
	}
	return service.Hydrate(ctx, raw, selector)
}

func repositoryService(repo objectmodel.Repository, commit kernel.CommitID, state liveaccess.StateLookup, request objectmodel.RequestContext, strict bool) (*Service, error) {
	if repo == nil || commit == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "Repository read requires repository and fixed commit")
	}
	repos := map[kernel.RepositoryID]kernel.CommitID{repo.ID(): commit}
	pin := reader.DatasetPin{
		SetID:        "projection",
		Repositories: repos,
		Items:        reader.WholeRepositoryItems(repos),
	}
	declarations := reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		if id != repo.ID() {
			return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "repository %s is not available", id)
		}
		return repo, nil
	}, pin)
	service := OpenRequest(declarations, state, request)
	service.strict = strict
	service.interpretation = repo
	return service, nil
}

// A direct Repository already supplies its permitted read boundary. Its fixed
// declarations may adopt an older Schema commit; the primary read remains on
// the command commit. Dataset services keep their original scoped member.
func (s *Service) interpretationRepository(id kernel.RepositoryID) (objectmodel.Repository, error) {
	if s.interpretation != nil && s.interpretation.ID() == id {
		return s.interpretation, nil
	}
	return s.declarations.Member(id)
}

func (s *Service) Read(ctx context.Context, objectID objectmodel.ObjectID, selector *objectmodel.AspectSelector) ([]objectmodel.ObservedValue, error) {
	values, err := s.declarations.Read(objectID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]objectmodel.ObservedValue, 0, len(values))
	for _, value := range values {
		result, err := s.Hydrate(ctx, value, selector)
		if err != nil {
			return nil, err
		}
		out = append(out, result)
	}
	return out, nil
}

// Hydrate upgrades one raw, pinned FederatedValue into the logical consumer
// value. SEARCH uses this after candidate discovery so exact READ and search
// hits cannot disagree about State Binding content.
func (s *Service) Hydrate(ctx context.Context, value objectmodel.FederatedValue, selector *objectmodel.AspectSelector) (objectmodel.ObservedValue, error) {
	if s.declarations.Pin().Repositories[value.Repository] != value.Commit {
		return objectmodel.ObservedValue{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "value is outside dataset release")
	}
	allowed, scopeErr := s.declarations.Contains(value.Repository, value.ObjectID)
	if scopeErr != nil {
		return objectmodel.ObservedValue{}, scopeErr
	}
	if !allowed {
		return objectmodel.ObservedValue{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "value is outside dataset scope")
	}
	result := objectmodel.ObservedValue{FederatedValue: value, Observations: []objectmodel.UnitObservation{}}
	if _, err := s.interpretationRepository(value.Repository); err != nil {
		return objectmodel.ObservedValue{}, err
	}
	for _, declaration := range value.Declarations {
		if !selected(declaration.Address, selector) {
			continue
		}
		if err := s.adoptUnit(ctx, &result, declaration); err != nil {
			return objectmodel.ObservedValue{}, err
		}
	}
	if objectmodel.IsEntityBlob(value.Address) && value.Locator == "" {
		result.Value = objectmodel.SelectAspects(result.Value, result.Units, selector)
	}
	if len(result.Observations) > 0 {
		result.Digest = ""
	}
	return result, nil
}

func (s *Service) ReadAddress(ctx context.Context, address objectmodel.Address) ([]objectmodel.ObservedValue, error) {
	values, err := s.declarations.ReadAddress(address)
	if err != nil {
		return nil, err
	}
	out := make([]objectmodel.ObservedValue, 0, len(values))
	for _, value := range values {
		result := objectmodel.ObservedValue{FederatedValue: value, Observations: []objectmodel.UnitObservation{}}
		if len(value.Declarations) == 1 {
			if err := s.adoptUnit(ctx, &result, value.Declarations[0]); err != nil {
				return nil, err
			}
		}
		out = append(out, result)
	}
	return out, nil
}

type hydratedUnit struct {
	Value   any
	Version objectmodel.UnitObservation
}

func (s *Service) hydrate(ctx context.Context, repositoryID kernel.RepositoryID, declaration objectmodel.UnitDeclaration) (hydratedUnit, bool, error) {
	source := declaration.ValueSource
	if source == nil || source.Kind == "" || source.Kind == objectmodel.ValueSourceSnapshot {
		return hydratedUnit{}, false, nil
	}
	if !source.IsBinding() {
		return hydratedUnit{}, false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "address %s has an invalid valueSource", objectmodel.AddressKey(declaration.Address))
	}
	binding, err := s.resolveHydrationBinding(repositoryID, declaration)
	if err != nil {
		return hydratedUnit{}, false, err
	}
	if binding.Mode == objectmodel.BindingStream {
		return hydratedUnit{}, false, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"ordinary READ cannot hydrate stream Binding at %s; use an explicit window/query surface", objectmodel.AddressKey(declaration.Address))
	}
	if binding.Mode != objectmodel.BindingState {
		return hydratedUnit{}, false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "address %s is not a State Binding", objectmodel.AddressKey(declaration.Address))
	}
	if s.state == nil {
		return hydratedUnit{}, false, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"State Binding at %s requires a Materialization Runtime", objectmodel.AddressKey(declaration.Address))
	}
	schema, err := s.observedSchema(binding)
	if err != nil {
		return hydratedUnit{}, false, err
	}
	observation, err := s.state.LookupState(ctx, liveaccess.StateLookupRequest{
		Binding: binding, SchemaRef: declaration.SchemaRef, Origin: binding.Origin,
		Identity: s.request.Identity, Trace: s.request.Trace, RequestID: s.request.RequestID,
	})
	if err != nil {
		if kernel.AsIngress(err) != nil {
			return hydratedUnit{}, false, err
		}
		return hydratedUnit{}, false, kernel.Fail(kernel.ErrTemporaryUnavailable,
			"State lookup failed for %s: %v", objectmodel.AddressKey(declaration.Address), err)
	}
	if err := liveaccess.CheckCredentialDisclosure(ctx, observation); err != nil {
		return hydratedUnit{}, false, err
	}
	if err := objectmodel.ValidateObservationBasis(observation.Basis); err != nil {
		return hydratedUnit{}, false, err
	}
	if err := validateObservedStructure(binding.Address, schema, observation.Value); err != nil {
		return hydratedUnit{}, false, err
	}
	return hydratedUnit{
		Value: observation.Value,
		Version: objectmodel.UnitObservation{
			Address: declaration.Address, DeclarationCommit: binding.DeclarationCommit,
			DeclarationDigest: binding.DeclarationDigest, DescriptorDigest: binding.DescriptorDigest,
			Basis: observation.Basis,
		},
	}, true, nil
}

func (s *Service) resolveHydrationBinding(repository kernel.RepositoryID, declaration objectmodel.UnitDeclaration) (liveaccess.ResolvedBinding, error) {
	if _, err := s.interpretationRepository(repository); err != nil {
		return liveaccess.ResolvedBinding{}, err
	}
	return liveaccess.ResolveDeclaration(s.descriptors, repository, s.declarations.Pin().Repositories[repository], declaration)
}

func selected(address objectmodel.Address, selector *objectmodel.AspectSelector) bool {
	if selector == nil || address.AspectName == "" {
		return true
	}
	if len(selector.Include) > 0 {
		included := false
		for _, name := range selector.Include {
			if name == address.AspectName {
				included = true
				break
			}
		}
		if !included {
			return false
		}
	}
	for _, name := range selector.Exclude {
		if name == address.AspectName {
			return false
		}
	}
	return true
}

func replaceUnit(root any, address objectmodel.Address, value any) (any, error) {
	if objectmodel.IsEntityBlob(address) {
		return value, nil
	}
	object, ok := root.(map[string]any)
	if !ok {
		return nil, kernel.Fail(kernel.ErrObjectIDConflict, "bound unit %s cannot be assembled into a non-object", objectmodel.AddressKey(address))
	}
	out := make(map[string]any, len(object))
	for key, item := range object {
		out[key] = item
	}
	if address.MemberKey == "" {
		out[address.AspectName] = value
		return out, nil
	}
	rawBucket, ok := object[address.AspectName]
	if !ok {
		return nil, kernel.Fail(kernel.ErrObjectIDConflict, "bound member %s has no assembled aspect", objectmodel.AddressKey(address))
	}
	bucket, ok := rawBucket.(map[string]any)
	if !ok {
		return nil, kernel.Fail(kernel.ErrObjectIDConflict, "bound member %s is not assembled as a member map", objectmodel.AddressKey(address))
	}
	if _, ok := bucket[address.MemberKey]; !ok {
		return nil, kernel.Fail(kernel.ErrObjectIDConflict, "bound member %s is missing from its assembled value", objectmodel.AddressKey(address))
	}
	updated := make(map[string]any, len(bucket))
	for key, item := range bucket {
		updated[key] = item
	}
	updated[address.MemberKey] = value
	out[address.AspectName] = updated
	return out, nil
}
