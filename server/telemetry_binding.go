package server

import (
	"context"
	inv "kc/invocation"
	"strings"
	"time"

	"kc/internal/telemetry"
	"kc/kernel"
	knowledgeserving "kc/knowledge/serving"
)

type observedStateLookup struct {
	base    knowledgeserving.StateLookup
	runtime *telemetry.Runtime
}

func observeStateLookup(base knowledgeserving.StateLookup, runtime *telemetry.Runtime) knowledgeserving.StateLookup {
	if base == nil || runtime == nil {
		return base
	}
	if _, wrapped := base.(*observedStateLookup); wrapped {
		return base
	}
	return &observedStateLookup{base: base, runtime: runtime}
}

func (s *observedStateLookup) LookupState(ctx context.Context, request knowledgeserving.StateLookupRequest) (knowledgeserving.StateObservation, error) {
	mode := strings.ToLower(string(request.Binding.Mode))
	ctx, span, started := s.runtime.StartBindingLookup(ctx, mode)
	observation, err := s.base.LookupState(ctx, request)
	outcome, errorType := inv.TelemetryResult(err)
	age := time.Duration(-1)
	if err == nil {
		if observedAt, parseErr := time.Parse(time.RFC3339Nano, observation.Basis.ObservedAt); parseErr == nil {
			age = time.Since(observedAt)
			if age < 0 {
				age = 0
			}
		}
	}
	s.runtime.EndBindingLookup(ctx, span, started, mode, outcome, errorType, age)
	return observation, err
}

// AccessResource preserves HTTPStateLookup's optional descriptor-operation
// capability. Observability decorators must not change runtime capability
// discovery merely by wrapping the configured dependency.
func (s *observedStateLookup) AccessResource(ctx context.Context, request inv.ResourceOperationRequest) (any, error) {
	accessor, ok := s.base.(inv.ResourceOperationAccessor)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "resource-access/v1 runtime does not support descriptor operations")
	}
	return accessor.AccessResource(ctx, request)
}
