package service

import (
	"context"

	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
)

// ReadReference keeps one explicitly named source and its accepted declaration
// commit. Dynamic values use the same hydration and observation validation as
// ordinary READ; selection occurs after the chosen complete unit is hydrated.
func (s *Service) ReadReference(ctx context.Context, ref objectmodel.Reference) ([]objectmodel.ObservedValue, error) {
	rawReference, err := objectmodel.FormatReference(ref)
	if err != nil {
		return nil, err
	}
	if ref.Locator == "" {
		values, err := s.declarations.ReadReference(ref)
		if err != nil {
			return nil, err
		}
		result, err := s.Hydrate(ctx, values[0], nil)
		if err != nil {
			return nil, err
		}
		return []objectmodel.ObservedValue{result}, nil
	}
	value, err := s.selectedReferenceValue(ref)
	if err != nil {
		return nil, err
	}
	result := objectmodel.ObservedValue{FederatedValue: value, Observations: []objectmodel.UnitObservation{}}
	// selectedReferenceValue still contains assembled complete units. Keep that
	// assembly position until Pointer projection, while retaining the selected
	// maintenance coordinate for the public result.
	selectedAddress := result.Address
	result.Address = objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: ref.Object}
	for _, declaration := range value.Declarations {
		if err := s.adoptUnit(ctx, &result, declaration); err != nil {
			return nil, err
		}
	}
	result.Address = selectedAddress
	result.Value, err = selectObservedReference(result, ref.Locator)
	if err != nil {
		return nil, err
	}
	result.Reference, result.Locator = rawReference, ref.Locator
	if len(result.Observations) > 0 {
		result.Digest = ""
	}
	return []objectmodel.ObservedValue{result}, nil
}

func selectObservedReference(value objectmodel.ObservedValue, locator string) (any, error) {
	tokens, err := objectmodel.PointerTokens(locator)
	if err != nil {
		return nil, err
	}
	for _, failure := range value.Failures {
		if tokenPrefix(unitPointerTokens(failure.Address), tokens) {
			return ValueFailure{Error: kernel.IngressError{Code: failure.Code, Message: failure.Message}}, nil
		}
	}
	return objectmodel.SelectPointer(value.Value, locator)
}

func (s *Service) selectedReferenceValue(ref objectmodel.Reference) (objectmodel.FederatedValue, error) {
	repo, err := s.declarations.Member(ref.Repository)
	if err != nil {
		return objectmodel.FederatedValue{}, err
	}
	commit := s.declarations.Pin().Repositories[ref.Repository]
	port, ok := repo.(objectmodel.ReferenceUnitsReader)
	if !ok {
		return objectmodel.FederatedValue{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "repository cannot provide selected complete reference units")
	}
	units, err := port.ReadReferenceUnits(ref, commit)
	var bound []objectmodel.UnitDeclaration
	if err != nil {
		if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
			return objectmodel.FederatedValue{}, err
		}
		// A missing leaf is not a maintenance coordinate. Only declarations
		// actually supplied by the permitted Domain Schema may synthesize one.
		interpretation, scopeErr := s.interpretationRepository(ref.Repository)
		if scopeErr != nil {
			return objectmodel.FederatedValue{}, scopeErr
		}
		extra, scopeErr := liveaccess.BoundAspectDeclarations(interpretation, commit, ref.Object, nil)
		if scopeErr != nil {
			return objectmodel.FederatedValue{}, scopeErr
		}
		tokens, _ := objectmodel.PointerTokens(ref.Locator)
		for _, declaration := range extra {
			if len(tokens) > 0 && declaration.Address.AspectName == tokens[0] {
				bound = append(bound, declaration)
				units = append(units, objectmodel.Unit{ObjectID: ref.Object, Address: declaration.Address, SchemaRef: declaration.SchemaRef, ValueSource: declaration.ValueSource})
			}
		}
		if len(bound) == 0 {
			return objectmodel.FederatedValue{}, err
		}
	}
	assembled, err := objectmodel.AssembleObjectValue(ref.Repository, ref.Object, commit, units)
	if err != nil {
		return objectmodel.FederatedValue{}, err
	}
	if len(bound) > 0 {
		assembled.Declarations = bound
	}
	value := objectmodel.FederatedValue{ObjectRef: assembled.ObjectRef, Repository: assembled.Repository, Commit: assembled.Commit, ObjectID: ref.Object, Address: assembled.Address, Value: assembled.Value, Provenance: assembled.Provenance, Units: assembled.Units, Declarations: assembled.Declarations}
	if len(units) == 1 && referenceWithinUnit(ref, units[0].Address) {
		value.Address, value.Digest = units[0].Address, units[0].Digest
	}
	return value, nil
}

func referenceWithinUnit(ref objectmodel.Reference, address objectmodel.Address) bool {
	if objectmodel.IsEntityBlob(address) {
		return true
	}
	tokens, _ := objectmodel.PointerTokens(ref.Locator)
	if len(tokens) == 0 || tokens[0] != address.AspectName {
		return false
	}
	return address.MemberKey == "" || (len(tokens) > 1 && tokens[1] == address.MemberKey)
}
