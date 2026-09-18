// Package serving exposes the consumer Knowledge value surface. Repository
// Reader stays pinned and declaration-only; this package composes it with an
// injected wall-out State runtime so callers receive logical Aspect values.
package serving

import (
	"context"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	"kc/observability"
)

// StateLookup is the narrow port supplied by a Materialization Runtime. The
// resource-access origin is declared on Domain Schema; the implementation
// still owns credentials, rate limits, caching, and source-specific protocol.
type StateLookup interface {
	LookupState(context.Context, StateLookupRequest) (StateObservation, error)
}

type StateLookupRequest struct {
	Binding   reader.ResolvedBinding        `json:"binding"`
	SchemaRef string                        `json:"schemaRef,omitempty"`
	Origin    string                        `json:"origin,omitempty"`
	Identity  observability.IdentityContext `json:"identity"`
	Trace     observability.TraceContext    `json:"trace,omitempty"`
	RequestID string                        `json:"requestId,omitempty"`
}

type StateObservation struct {
	Value any                        `json:"value"`
	Basis knowledge.ObservationBasis `json:"basis"`
}

// ReadResult retains the immutable declaration coordinates while Value holds
// the logical, hydrated result. Observations makes it impossible to mistake a
// dynamic value for bytes frozen by Commit.
type ReadResult struct {
	reader.FederatedValue
	Observations []knowledge.UnitObservation `json:"observations,omitempty"`
}

type Service struct {
	declarations *reader.Serving
	state        StateLookup
	request      RequestContext
}

type RequestContext struct {
	Identity  observability.IdentityContext `json:"identity"`
	Trace     observability.TraceContext    `json:"trace,omitempty"`
	RequestID string                        `json:"requestId,omitempty"`
}

func Open(declarations *reader.Serving, state StateLookup, identity observability.IdentityContext) *Service {
	return OpenRequest(declarations, state, RequestContext{Identity: identity})
}

// OpenRequest adds the correlation metadata that an out-of-process runtime
// needs. The identity has already crossed the Knowledge Server's trusted
// authentication boundary; this package does not authenticate it again.
func OpenRequest(declarations *reader.Serving, state StateLookup, request RequestContext) *Service {
	return &Service{declarations: declarations, state: state, request: request}
}

// HydrateRepositoryValue applies the same logical READ semantics to one raw
// KnowledgeValue at a fixed Repository commit. Projection control uses this
// entry point so exact READ and dynamic indexing cannot grow separate Binding
// implementations.
func HydrateRepositoryValue(ctx context.Context, repo knowledge.Repository, value knowledge.KnowledgeValue, state StateLookup, request RequestContext) (ReadResult, error) {
	if repo == nil || value.Commit == "" {
		return ReadResult{}, kernel.Fail(kernel.ErrUsageInvalid, "hydrate Repository value requires repository and commit")
	}
	repos := map[kernel.RepositoryID]kernel.CommitID{repo.ID(): value.Commit}
	pin := reader.KnowledgeSetPin{
		SetID:        "projection",
		Repositories: repos,
		Items:        reader.WholeRepositoryItems(repos),
	}
	declarations := reader.Open(func(id kernel.RepositoryID) (knowledge.Repository, error) {
		if id != repo.ID() {
			return nil, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "repository %s is not available", id)
		}
		return repo, nil
	}, pin)
	raw := reader.FederatedValue{
		KnowledgeRef: value.KnowledgeRef, Repository: repo.ID(), Commit: value.Commit,
		ObjectID: value.Address.ObjectID, Address: value.Address, Value: value.Value,
		Provenance: value.Provenance, Units: value.Units, Declarations: value.Declarations,
	}
	return OpenRequest(declarations, state, request).Hydrate(ctx, raw, nil)
}

func (s *Service) Read(ctx context.Context, objectID knowledge.ObjectID, selector *knowledge.AspectSelector) ([]ReadResult, error) {
	values, err := s.declarations.Read(objectID, nil)
	if err != nil {
		return nil, err
	}
	out := make([]ReadResult, 0, len(values))
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
func (s *Service) Hydrate(ctx context.Context, value reader.FederatedValue, selector *knowledge.AspectSelector) (ReadResult, error) {
	result := ReadResult{FederatedValue: value, Observations: []knowledge.UnitObservation{}}
	repo, err := s.declarations.Member(value.Repository)
	if err != nil {
		return ReadResult{}, err
	}
	extra, err := reader.BoundAspectDeclarations(repo, value.Commit, value.ObjectID, value.Declarations)
	if err != nil {
		return ReadResult{}, err
	}
	declarations := append(append([]knowledge.UnitDeclaration{}, value.Declarations...), extra...)
	result.Declarations = declarations
	for _, extraDecl := range extra {
		result.Units = append(result.Units, extraDecl.Address)
	}
	for _, declaration := range declarations {
		if !selected(declaration.Address, selector) {
			continue
		}
		observation, ok, err := s.hydrate(ctx, value.Repository, declaration)
		if err != nil {
			return ReadResult{}, err
		}
		if !ok {
			continue
		}
		result.Value, err = replaceUnit(result.Value, declaration.Address, observation.Value)
		if err != nil {
			return ReadResult{}, err
		}
		result.Observations = append(result.Observations, observation.Version)
	}
	result.Value = knowledge.SelectAspects(result.Value, result.Units, selector)
	return result, nil
}

func (s *Service) ReadAddress(ctx context.Context, address knowledge.Address) ([]ReadResult, error) {
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
	out := make([]ReadResult, 0, len(values))
	for _, value := range values {
		result := ReadResult{FederatedValue: value, Observations: []knowledge.UnitObservation{}}
		if len(value.Declarations) == 1 {
			observation, ok, err := s.hydrate(ctx, value.Repository, value.Declarations[0])
			if err != nil {
				return nil, err
			}
			if ok {
				result.Value = observation.Value
				result.Observations = append(result.Observations, observation.Version)
			}
		}
		out = append(out, result)
	}
	return out, nil
}

func (s *Service) boundAddressValue(address knowledge.Address) ([]reader.FederatedValue, error) {
	bindings, err := s.declarations.ResolveBinding(address)
	if err != nil {
		return nil, err
	}
	out := make([]reader.FederatedValue, 0, len(bindings))
	for _, binding := range bindings {
		source := &knowledge.ValueSource{Kind: knowledge.ValueSourceBinding, Binding: &knowledge.BindingDeclaration{
			Mode: binding.Mode, Runtime: binding.Runtime, Protocol: binding.Protocol,
			Operations: binding.Operations, DescriptorRef: binding.DescriptorRef,
		}}
		out = append(out, reader.FederatedValue{
			KnowledgeRef: knowledge.KnowledgeRef{Repository: binding.Repository, Object: address.ObjectID},
			Repository:   binding.Repository,
			Commit:       binding.DeclarationCommit,
			ObjectID:     address.ObjectID,
			Address:      address,
			Declarations: []knowledge.UnitDeclaration{{
				Address: address, SchemaRef: binding.SchemaRef, ValueSource: source,
				DeclarationDigest: binding.DeclarationDigest,
			}},
		})
	}
	return out, nil
}

type hydratedUnit struct {
	Value   any
	Version knowledge.UnitObservation
}

func (s *Service) hydrate(ctx context.Context, repositoryID kernel.RepositoryID, declaration knowledge.UnitDeclaration) (hydratedUnit, bool, error) {
	source := declaration.ValueSource
	if source == nil || source.Kind == "" || source.Kind == knowledge.ValueSourceSnapshot {
		return hydratedUnit{}, false, nil
	}
	if source.Kind != knowledge.ValueSourceBinding || source.Binding == nil {
		return hydratedUnit{}, false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "address %s has an invalid value_source", knowledge.AddressKey(declaration.Address))
	}
	if source.Binding.Mode == knowledge.BindingStream {
		return hydratedUnit{}, false, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"ordinary READ cannot hydrate stream Binding at %s; use an explicit window/query surface", knowledge.AddressKey(declaration.Address))
	}
	if source.Binding.Mode != knowledge.BindingState {
		return hydratedUnit{}, false, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "address %s is not a State Binding", knowledge.AddressKey(declaration.Address))
	}
	if s.state == nil {
		return hydratedUnit{}, false, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"State Binding at %s requires a Materialization Runtime", knowledge.AddressKey(declaration.Address))
	}
	binding, err := s.declarations.ResolveBindingAt(repositoryID, declaration.Address)
	if err != nil {
		return hydratedUnit{}, false, err
	}
	observation, err := s.state.LookupState(ctx, StateLookupRequest{
		Binding: binding, SchemaRef: declaration.SchemaRef, Origin: binding.Origin,
		Identity: s.request.Identity, Trace: s.request.Trace, RequestID: s.request.RequestID,
	})
	if err != nil {
		if kernel.AsIngress(err) != nil {
			return hydratedUnit{}, false, err
		}
		return hydratedUnit{}, false, kernel.Fail(kernel.ErrTemporaryUnavailable,
			"State lookup failed for %s: %v", knowledge.AddressKey(declaration.Address), err)
	}
	if err := knowledge.ValidateObservationBasis(observation.Basis); err != nil {
		return hydratedUnit{}, false, err
	}
	return hydratedUnit{
		Value: observation.Value,
		Version: knowledge.UnitObservation{
			Address: declaration.Address, DeclarationCommit: binding.DeclarationCommit,
			DeclarationDigest: binding.DeclarationDigest, DescriptorDigest: binding.DescriptorDigest,
			Basis: observation.Basis,
		},
	}, true, nil
}

func selected(address knowledge.Address, selector *knowledge.AspectSelector) bool {
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

func replaceUnit(root any, address knowledge.Address, value any) (any, error) {
	if knowledge.IsEntityBlob(address) {
		return value, nil
	}
	object, ok := root.(map[string]any)
	if !ok {
		return nil, kernel.Fail(kernel.ErrObjectIDConflict, "bound unit %s cannot be assembled into a non-object", knowledge.AddressKey(address))
	}
	if address.MemberKey == "" {
		object[address.AspectName] = value
		return object, nil
	}
	rawBucket, ok := object[address.AspectName]
	if !ok {
		return nil, kernel.Fail(kernel.ErrObjectIDConflict, "bound member %s has no assembled aspect", knowledge.AddressKey(address))
	}
	bucket, ok := rawBucket.(map[string]any)
	if !ok {
		return nil, kernel.Fail(kernel.ErrObjectIDConflict, "bound member %s is not assembled as a member map", knowledge.AddressKey(address))
	}
	if _, ok := bucket[address.MemberKey]; !ok {
		return nil, kernel.Fail(kernel.ErrObjectIDConflict, "bound member %s is missing from its assembled value", knowledge.AddressKey(address))
	}
	bucket[address.MemberKey] = value
	return object, nil
}
