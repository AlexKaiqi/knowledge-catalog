package cli

// The request-context vocabulary, the action derivation, and the whole
// observation/evidence family moved to kc/invocation in CAP-ORG-01 B5b.3: they
// serve the shared dispatcher and the typed service alike. These aliases keep
// the CLI call sites (registry verbs, run loop, product output) unchanged.

import (
	"context"
	"kc/internal/journal"
	"kc/internal/telemetry"
	inv "kc/invocation"
	knowledgeserving "kc/knowledge/serving"
	"kc/observability"
	serv "kc/server"
	"kc/snapshot"
	"time"
)

func requestIDFrom(flags map[string]FlagValue) (string, error) { return inv.RequestIDFrom(flags) }

func identityContextFrom(flags map[string]FlagValue) (observability.IdentityContext, error) {
	return inv.IdentityContextFrom(flags)
}

func traceContextFrom(flags map[string]FlagValue) (observability.TraceContext, error) {
	return inv.TraceContextFrom(flags)
}

func actionOf(command string, flags map[string]FlagValue) string { return inv.ActionOf(command, flags) }

func telemetrySince(started time.Time) time.Duration { return inv.TelemetrySince(started) }

func telemetryResultFor(command string, result any, err error) (string, string) {
	return inv.TelemetryResultFor(command, result, err)
}

func telemetryFace(command string) string { return inv.TelemetryFace(command) }

func telemetryProvider(flags map[string]FlagValue) string { return inv.TelemetryProvider(flags) }

func recordDomainTelemetry(ctx context.Context, runtime *telemetry.Runtime, command string, flags map[string]FlagValue, observation *operationTelemetry, result any, callErr error, elapsed time.Duration) {
	inv.RecordDomainTelemetry(ctx, runtime, command, flags, observation, result, callErr, elapsed)
}

func evidenceChain(ctx context.Context, runtime *telemetry.Runtime, home, command string, flags map[string]FlagValue, result any, callErr error) (any, error) {
	return inv.EvidenceChain(ctx, runtime, home, command, flags, result, callErr)
}

func shapeInvocationResult(result any, err error) RunResult {
	return inv.ShapeInvocationResult(result, err)
}

type telemetryStart = inv.TelemetryStart

var _ = knowledgeserving.StateLookup(nil)

func usesAddress(flags map[string]FlagValue) bool { return inv.UsesAddress(flags) }

func recordAudit(home, command string, flags map[string]FlagValue, result any, err error) (int64, error) {
	return inv.RecordAudit(home, command, flags, result, err)
}

func rejectFlagsOutside(flags map[string]FlagValue, allowed map[string]struct{}, surface string) error {
	return inv.RejectFlagsOutside(flags, allowed, surface)
}

func rejectUnknownFlags(flags map[string]FlagValue) error { return inv.RejectUnknownFlags(flags) }

func rejectServeOnlyFlags(flags map[string]FlagValue) error { return inv.RejectServeOnlyFlags(flags) }

func rejectServeFlags(flags map[string]FlagValue) error { return inv.RejectServeFlags(flags) }

func executeApplicationOperation(ctx context.Context, name, action string, cmd command, flags map[string]FlagValue, state knowledgeserving.StateLookup, opened *Home, home string, observation *operationTelemetry) (any, error) {
	return inv.ExecuteApplicationOperation(ctx, name, action, cmd, flags, state, opened, home, observation)
}

type ClientCatalogState = inv.ClientCatalogState

const SemanticFileViewV1 = inv.SemanticFileViewV1

func requireRemoteFlag(flags map[string]FlagValue, name string) (string, error) {
	return inv.RequireRemoteFlag(flags, name)
}

type HTTPIdentity = inv.HTTPIdentity

type ManagedRepositorySummary = serv.ManagedRepositorySummary

func readTrail(home, layer, cmd string, limit int) ([]journal.Event, error) {
	return inv.ReadTrail(home, layer, cmd, limit)
}

func observeSnapshotStore(store snapshot.Store, runtime *telemetry.Runtime, fallback string) snapshot.Store {
	return inv.ObserveSnapshotStore(store, runtime, fallback)
}

func snapshotStoreKind(store any, fallback string) string {
	return inv.SnapshotStoreKind(store, fallback)
}

func observeStandingProjection(runtime *telemetry.Runtime, ws *Home, flags map[string]FlagValue) {
	inv.ObserveStandingProjection(runtime, ws, flags)
}
