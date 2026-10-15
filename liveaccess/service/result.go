package service

import (
	"context"
	"strings"

	"kc/kernel"
	"kc/liveaccess"
	"kc/liveaccess/evidence"
	objectmodel "kc/object"
)

// ValueFailure is the value-position explanation for an unavailable external
// unit. ObservedValue.Failures distinguishes it from a business value with an
// ordinary field named error.
type ValueFailure struct {
	Error kernel.IngressError `json:"error"`
}

func (s *Service) adoptUnit(ctx context.Context, result *objectmodel.ObservedValue, declaration objectmodel.UnitDeclaration) error {
	prepared := declaration
	observation, dynamic, err := s.hydrate(ctx, result.Repository, prepared)
	if !dynamic && err == nil {
		return nil
	}
	if err != nil && (s.strict || ctx.Err() != nil) {
		return err
	}
	sample := evidence.Sample{Repository: result.Repository, Commit: result.Commit, Address: prepared.Address, Declaration: prepared}
	var replacement any
	if err != nil {
		failure := kernel.Normalize(liveaccess.SafeFailure(ctx, err))
		replacement = ValueFailure{Error: *failure}
		result.Failures = append(result.Failures, objectmodel.UnitReadFailure{Address: prepared.Address, Code: failure.Code, Message: failure.Message})
		sample.Outcome, sample.Failure = evidence.FailureOutcome, &evidence.Failure{Code: failure.Code, Message: failure.Message}
	} else {
		replacement = observation.Value
		result.Observations = append(result.Observations, observation.Version)
		sample.Declaration.DeclarationDigest = observation.Version.DeclarationDigest
		sample.Outcome, sample.Value, sample.Observation = evidence.ValueOutcome, observation.Value, &observation.Version
	}
	result.Value, err = replaceSelectedUnit(result.FederatedValue, prepared.Address, replacement, sample.Outcome == evidence.FailureOutcome)
	if err != nil {
		return err
	}
	declarations := append([]objectmodel.UnitDeclaration(nil), result.Declarations...)
	for i := range declarations {
		if declarations[i].Address == prepared.Address {
			declarations[i] = sample.Declaration
		}
	}
	result.Declarations = declarations
	result.Digest = ""
	return evidence.Capture(ctx, sample)
}

func unitPointerTokens(address objectmodel.Address) []string {
	if objectmodel.IsEntityBlob(address) {
		return nil
	}
	tokens := []string{address.AspectName}
	if address.MemberKey != "" {
		tokens = append(tokens, address.MemberKey)
	}
	return tokens
}

func tokenPrefix(prefix, tokens []string) bool {
	if len(prefix) > len(tokens) {
		return false
	}
	for i, token := range prefix {
		if tokens[i] != token {
			return false
		}
	}
	return true
}

func pointerOf(tokens []string) string {
	parts := make([]string, len(tokens))
	for i, token := range tokens {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
	}
	if len(parts) == 0 {
		return ""
	}
	return "/" + strings.Join(parts, "/")
}

func replaceSelectedUnit(raw objectmodel.FederatedValue, address objectmodel.Address, value any, failure bool) (any, error) {
	if raw.Locator == "" {
		if raw.Address == address && !objectmodel.IsEntityBlob(raw.Address) {
			return value, nil
		}
		return replaceUnit(raw.Value, address, value)
	}
	selected, err := objectmodel.PointerTokens(raw.Locator)
	if err != nil {
		return nil, err
	}
	unit := unitPointerTokens(address)
	if tokenPrefix(unit, selected) {
		if failure {
			return value, nil
		}
		return objectmodel.SelectPointer(value, pointerOf(selected[len(unit):]))
	}
	if !tokenPrefix(selected, unit) {
		return raw.Value, nil
	}
	relative := unit[len(selected):]
	if len(relative) != 1 {
		return nil, kernel.Fail(kernel.ErrObjectIDConflict, "selected unit has no supported relative position")
	}
	object, ok := raw.Value.(map[string]any)
	if !ok {
		return nil, kernel.Fail(kernel.ErrObjectIDConflict, "selected member collection is not an object")
	}
	out := make(map[string]any, len(object))
	for key, item := range object {
		out[key] = item
	}
	out[relative[0]] = value
	return out, nil
}
