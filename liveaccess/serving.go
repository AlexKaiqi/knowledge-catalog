// Package serving exposes the consumer Object value surface. Repository
// Reader stays pinned and declaration-only; this package composes it with an
// injected wall-out State runtime so callers receive logical Aspect values.
package liveaccess

import (
	"context"
	"kc/kernel"

	objectmodel "kc/object"
)

// StateLookup is the narrow port supplied by a Materialization Runtime. The
// resource-access origin is declared on Domain Schema; the implementation
// still owns credentials, rate limits, caching, and source-specific protocol.
type StateLookup interface {
	LookupState(context.Context, StateLookupRequest) (StateObservation, error)
}

type StateLookupRequest struct {
	Binding   ResolvedBinding        `json:"binding"`
	SchemaRef string                 `json:"schemaRef,omitempty"`
	Origin    string                 `json:"origin,omitempty"`
	Identity  kernel.IdentityContext `json:"identity"`
	Trace     kernel.TraceContext    `json:"trace,omitempty"`
	RequestID string                 `json:"requestId,omitempty"`
}

type StateObservation struct {
	Value any                          `json:"value"`
	Basis objectmodel.ObservationBasis `json:"basis"`
}
