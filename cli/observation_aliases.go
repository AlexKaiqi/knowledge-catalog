package cli

// The request-context vocabulary, the action derivation, and the whole
// observation/evidence family moved to kc/invocation in CAP-ORG-01 B5b.3: they
// serve the shared dispatcher and the typed service alike. These aliases keep
// the CLI call sites (registry verbs, run loop, product output) unchanged.

import (
	"context"
	"time"

	inv "kc/invocation"

	"kc/catalog"
	"kc/internal/journal"
	"kc/internal/telemetry"
	knowledge "kc/knowledge"
	knowledgeserving "kc/knowledge/serving"
	"kc/observability"
	"kc/retrieval"
	"kc/snapshot"
)

func requestIDFrom(flags map[string]FlagValue) (string, error) { return inv.RequestIDFrom(flags) }

func identityContextFrom(flags map[string]FlagValue) (observability.IdentityContext, error) {
	return inv.IdentityContextFrom(flags)
}

func traceContextFrom(flags map[string]FlagValue) (observability.TraceContext, error) {
	return inv.TraceContextFrom(flags)
}

func actionOf(command string, flags map[string]FlagValue) string { return inv.ActionOf(command, flags) }

func bindHomeTelemetry(runtime *telemetry.Runtime, ws *Home, flags map[string]FlagValue) {
	inv.BindHomeTelemetry(runtime, ws, flags)
}

func telemetryNow() time.Time { return inv.TelemetryNow() }

func telemetrySince(started time.Time) time.Duration { return inv.TelemetrySince(started) }

func telemetryOutcome(err error) string { return inv.TelemetryOutcome(err) }

func telemetryResult(err error) (string, string) { return inv.TelemetryResult(err) }

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

func observeProjectionExecution(cx *invocation) func() { return inv.ObserveProjectionExecution(cx) }

func observeStandingProjection(runtime *telemetry.Runtime, ws *Home, flags map[string]FlagValue) {
	inv.ObserveStandingProjection(runtime, ws, flags)
}

type telemetryStart = inv.TelemetryStart

var _ = knowledgeserving.StateLookup(nil)

func limitFrom(flags map[string]FlagValue, fallback int) (int, error) {
	return inv.LimitFrom(flags, fallback)
}

func pageLimit(flags map[string]FlagValue, fallback, max int) (int, error) {
	return inv.PageLimit(flags, fallback, max)
}

func usesAddress(flags map[string]FlagValue) bool { return inv.UsesAddress(flags) }

func addressFrom(flags map[string]FlagValue) (knowledge.Address, error) {
	return inv.AddressFrom(flags)
}

func searchRequestFromFlags(flags map[string]FlagValue) (retrieval.SearchRequest, error) {
	return inv.SearchRequestFromFlags(flags)
}

func traverseHops(flags map[string]FlagValue) (int, error) { return inv.TraverseHops(flags) }

func errorResult(err error) RunResult { return inv.ErrorResult(err) }

const resolvedPinFlag = inv.ResolvedPinFlag

func recordKnowledgeAccess(home, command string, flags map[string]FlagValue, result any, callErr error) (string, int64, error) {
	return inv.RecordKnowledgeAccess(home, command, flags, result, callErr)
}

func recordAudit(home, command string, flags map[string]FlagValue, result any, err error) (int64, error) {
	return inv.RecordAudit(home, command, flags, result, err)
}

const resolvedCommitFlag = inv.ResolvedCommitFlag

func knowledgeAccesses(result any) []observability.KnowledgeAccess {
	return inv.KnowledgeAccesses(result)
}

func withRetrievalObservationSource(result any, source any) any {
	return inv.WithRetrievalObservationSource(result, source)
}

type observedAccessResult = inv.ObservedAccessResult

const defaultHome = inv.DefaultHome

func readTrail(home, layer, cmd string, limit int) ([]journal.Event, error) {
	return inv.ReadTrail(home, layer, cmd, limit)
}

func catalogLogEntries(commits []catalog.CatalogCommit, cmd string, limit int) []map[string]any {
	return inv.CatalogLogEntries(commits, cmd, limit)
}

func jsonValue(value any) any { return inv.JSONValue(value) }

func stringValue(value any) string { return inv.StringValue(value) }

func fileAccesses(result any) []observability.FileAccess { return inv.FileAccesses(result) }

func snapshotAccesses(result any) []observability.SnapshotAccess { return inv.SnapshotAccesses(result) }

func retrievalEventFrom(command string, flags map[string]FlagValue, result any, accessEvidenceID string, callErr error) (observability.RetrievalEvent, error) {
	return inv.RetrievalEventFrom(command, flags, result, accessEvidenceID, callErr)
}

func observeSnapshotStore(store snapshot.Store, runtime *telemetry.Runtime, fallback string) snapshot.Store {
	return inv.ObserveSnapshotStore(store, runtime, fallback)
}

func snapshotStoreKind(store any, fallback string) string {
	return inv.SnapshotStoreKind(store, fallback)
}

func flagNames(names string) map[string]struct{} { return inv.FlagNames(names) }

func rejectFlagsOutside(flags map[string]FlagValue, allowed map[string]struct{}, surface string) error {
	return inv.RejectFlagsOutside(flags, allowed, surface)
}

func rejectUnknownFlags(flags map[string]FlagValue) error { return inv.RejectUnknownFlags(flags) }

func rejectServeOnlyFlags(flags map[string]FlagValue) error { return inv.RejectServeOnlyFlags(flags) }

func rejectServeFlags(flags map[string]FlagValue) error { return inv.RejectServeFlags(flags) }

func typedInvocationReadOnly(action string) bool { return inv.TypedInvocationReadOnly(action) }

func executeApplicationOperation(ctx context.Context, name, action string, cmd command, flags map[string]FlagValue, state knowledgeserving.StateLookup, opened *Home, home string, observation *operationTelemetry) (any, error) {
	return inv.ExecuteApplicationOperation(ctx, name, action, cmd, flags, state, opened, home, observation)
}

func invokeApplicationWithTelemetryAtHome(ctx context.Context, runtime *telemetry.Runtime, name, action string, op command, flags map[string]FlagValue, state knowledgeserving.StateLookup, opened *Home) RunResult {
	return inv.InvokeApplicationWithTelemetryAtHome(ctx, runtime, name, action, op, flags, state, opened)
}
