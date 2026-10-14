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
	state          liveaccess.StateLookup
	request        objectmodel.RequestContext
	strict         bool
	interpretation objectmodel.Repository
}

func Open(declarations *reader.Serving, state liveaccess.StateLookup, identity kernel.IdentityContext) *Service {
	return OpenRequest(declarations, state, objectmodel.RequestContext{Identity: identity})
}

// OpenRequest adds the correlation metadata that an out-of-process runtime
// needs. The identity has already crossed the Knowledge Server's trusted
// authentication boundary; this package does not authenticate it again.
func OpenRequest(declarations *reader.Serving, state liveaccess.StateLookup, request objectmodel.RequestContext) *Service {
	return &Service{declarations: declarations, state: state, request: request}
}

// HydrateRepositoryValue applies the same logical READ semantics to one raw
// KnowledgeValue at a fixed Repository commit. Projection control uses this
// entry point so exact READ and dynamic indexing cannot grow separate Binding
// implementations.
func HydrateRepositoryValue(ctx context.Context, repo objectmodel.Repository, value objectmodel.KnowledgeValue, state liveaccess.StateLookup, request objectmodel.RequestContext) (objectmodel.ObservedValue, error) {
	return hydrateRepository(ctx, repo, value, state, request, nil, true)
}

// HydrateRepositoryRead composes an already read, fixed Repository result with
// external units. Address and Pointer selections preserve the original shape;
// one external failure is explained only at that unit's selected position.
func HydrateRepositoryRead(ctx context.Context, repo objectmodel.Repository, value objectmodel.KnowledgeValue, state liveaccess.StateLookup, request objectmodel.RequestContext, selector *objectmodel.AspectSelector) (objectmodel.ObservedValue, error) {
	return hydrateRepository(ctx, repo, value, state, request, selector, false)
}

func hydrateRepository(ctx context.Context, repo objectmodel.Repository, value objectmodel.KnowledgeValue, state liveaccess.StateLookup, request objectmodel.RequestContext, selector *objectmodel.AspectSelector, strict bool) (objectmodel.ObservedValue, error) {
	service, err := repositoryService(repo, value.Commit, state, request, strict)
	if err != nil {
		return objectmodel.ObservedValue{}, err
	}
	raw := objectmodel.FederatedValue{
		KnowledgeRef: value.KnowledgeRef, Repository: repo.ID(), Commit: value.Commit,
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
	pin := reader.KnowledgeSetPin{
		SetID:        "projection",
		Repositories: repos,
		Items:        reader.WholeRepositoryItems(repos),
	}
	declarations := reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		if id != repo.ID() {
			return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "repository %s is not available", id)
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
		return objectmodel.ObservedValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "value is outside dataset release")
	}
	allowed, scopeErr := s.declarations.Contains(value.Repository, value.ObjectID)
	if scopeErr != nil {
		return objectmodel.ObservedValue{}, scopeErr
	}
	if !allowed {
		return objectmodel.ObservedValue{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "value is outside dataset scope")
	}
	result := objectmodel.ObservedValue{FederatedValue: value, Observations: []objectmodel.UnitObservation{}}
	repo, err := s.interpretationRepository(value.Repository)
	if err != nil {
		return objectmodel.ObservedValue{}, err
	}
	var extra []objectmodel.UnitDeclaration
	if objectmodel.IsEntityBlob(value.Address) && value.Locator == "" {
		extra, err = liveaccess.BoundAspectDeclarations(repo, value.Commit, value.ObjectID, value.Declarations)
		if err != nil {
			return objectmodel.ObservedValue{}, err
		}
	}
	declarations := append(append([]objectmodel.UnitDeclaration{}, value.Declarations...), extra...)
	result.Declarations = declarations
	for _, extraDecl := range extra {
		result.Units = append(result.Units, extraDecl.Address)
	}
	for _, declaration := range declarations {
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
	if len(values) == 0 {
		synthesized, err := s.boundAddressValue(address)
		if err != nil {
			return nil, err
		}
		values = synthesized
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

func (s *Service) boundAddressValue(address objectmodel.Address) ([]objectmodel.FederatedValue, error) {
	var bindings []liveaccess.ResolvedBinding
	var err error
	if s.interpretation != nil {
		var binding liveaccess.ResolvedBinding
		binding, err = liveaccess.ResolveRepoBinding(s.interpretation, s.declarations.Pin().Repositories[s.interpretation.ID()], address)
		if err == nil {
			bindings = []liveaccess.ResolvedBinding{binding}
		}
	} else {
		bindings, err = liveaccess.ResolveBinding(s.declarations, address)
	}
	if err != nil {
		return nil, err
	}
	out := make([]objectmodel.FederatedValue, 0, len(bindings))
	for _, binding := range bindings {
		source := &objectmodel.ValueSource{Kind: objectmodel.ValueSourceBinding, Binding: &objectmodel.BindingDeclaration{
			Mode: binding.Mode, Runtime: binding.Runtime, Protocol: binding.Protocol,
			Operations: binding.Operations, DescriptorRef: binding.DescriptorRef,
		}}
		out = append(out, objectmodel.FederatedValue{
			KnowledgeRef: objectmodel.KnowledgeRef{Repository: binding.Repository, Object: address.ObjectID},
			Repository:   binding.Repository,
			Commit:       binding.DeclarationCommit,
			ObjectID:     address.ObjectID,
			Address:      address,
			Declarations: []objectmodel.UnitDeclaration{{
				Address: address, SchemaRef: binding.SchemaRef, ValueSource: source,
				DeclarationDigest: binding.DeclarationDigest,
			}},
		})
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
	if source.Kind != objectmodel.ValueSourceBinding || source.Binding == nil {
		return hydratedUnit{}, false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "address %s has an invalid value_source", objectmodel.AddressKey(declaration.Address))
	}
	if source.Binding.Mode == objectmodel.BindingStream {
		return hydratedUnit{}, false, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"ordinary READ cannot hydrate stream Binding at %s; use an explicit window/query surface", objectmodel.AddressKey(declaration.Address))
	}
	if source.Binding.Mode != objectmodel.BindingState {
		return hydratedUnit{}, false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "address %s is not a State Binding", objectmodel.AddressKey(declaration.Address))
	}
	if s.state == nil {
		return hydratedUnit{}, false, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"State Binding at %s requires a Materialization Runtime", objectmodel.AddressKey(declaration.Address))
	}
	binding, err := s.resolveHydrationBinding(repositoryID, declaration)
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
	if err := s.validateObservedStructure(binding, observation.Value); err != nil {
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
	repo, err := s.interpretationRepository(repository)
	if err != nil {
		return liveaccess.ResolvedBinding{}, err
	}
	binding, err := liveaccess.ResolveRepoDeclarationBinding(repo, s.declarations.Pin().Repositories[repository], declaration)
	if declaration.SchemaRef != "" {
		binding.SchemaRef = declaration.SchemaRef
	}
	return binding, err
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
