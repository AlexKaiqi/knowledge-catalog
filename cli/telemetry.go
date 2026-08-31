package cli

import (
	"context"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"

	"kc/internal/telemetry"
	"kc/kernel"
	"kc/retrieval"
)

type telemetryStart struct {
	span trace.Span
	at   time.Time
}

type projectionBacklogObserver func(lagging int, oldestPendingAt time.Time)
type evidenceTelemetryObserver func(kind, outcome string, elapsed time.Duration)

func telemetryNow() time.Time { return time.Now() }

func telemetrySince(started time.Time) time.Duration {
	if started.IsZero() {
		return 0
	}
	return time.Since(started)
}

func telemetryOutcome(err error) string {
	if err == nil {
		return "ok"
	}
	return "error"
}

func telemetryResult(err error) (outcome, errorType string) {
	if err == nil {
		return "ok", "none"
	}
	code := kernel.CodeOf(err)
	if code == "" {
		// A bare internal/backend error has not been classified. Do not label
		// it as a caller mistake in telemetry merely because Normalize keeps a
		// compatibility fallback for the public error envelope.
		return "error", "other"
	}
	switch code {
	case kernel.ErrForbidden, kernel.ErrUnauthenticated:
		outcome = "denied"
	case kernel.ErrKnowledgeRefUnresolved, kernel.ErrVersionUnresolved, kernel.ErrWorkspaceInvalid:
		outcome = "unresolved"
	case kernel.ErrNonFastForward, kernel.ErrIdempotencyConflict, kernel.ErrObjectIDConflict,
		kernel.ErrEventIDConflict, kernel.ErrCandidateMoved:
		outcome = "conflict"
	case kernel.ErrUsageInvalid, kernel.ErrPreconditionFailed, kernel.ErrWriteTargetRequired,
		kernel.ErrSurfaceMismatch, kernel.ErrScopeDenied, kernel.ErrSchemaUnsupported,
		kernel.ErrSchemaRevisionUnresolved, kernel.ErrTargetRepositoryDenied:
		outcome = "invalid"
	default:
		outcome = "error"
	}
	return outcome, string(code)
}

func telemetryResultFor(command string, result any, err error) (outcome, errorType string) {
	outcome, errorType = telemetryResult(err)
	if err != nil {
		return outcome, errorType
	}
	if resultOutcome(result) == "partial" {
		return "partial", ""
	}
	if command != "search" {
		return outcome, errorType
	}
	if row, ok := jsonValue(accessOutput(result)).(map[string]any); ok && stringValue(row["completeness"]) == "partial" {
		return "partial", ""
	}
	return outcome, errorType
}

func telemetryFace(command string) string {
	switch command {
	case "put", "remove", "commit", "ingest", "writer-head", "receipt":
		return "writer"
	case "search", "describe-index", "index-sync", "describe-access":
		return "projection"
	case "read", "list", "relations", "provenance", "log", "diff", "resolve-binding", "describe-schema":
		return "knowledge"
	case "resolve", "inspect", "checkout", "define-workspace", "retire-workspace", "repo-add", "archive-repo", "catalog-add", "archive-catalog":
		return "catalog"
	case "file-mounts", "file-list", "file-read":
		return "vfs"
	case "merge", "validate", "record-validation":
		return "control"
	default:
		return "other"
	}
}

func recordDomainTelemetry(ctx context.Context, runtime *telemetry.Runtime, command string, flags map[string]FlagValue, observation *operationTelemetry, result any, callErr error, elapsed time.Duration) {
	observation = noOperationTelemetry(observation)
	outcome, errorType := telemetryResultFor(command, result, callErr)
	visible := accessOutput(result)
	switch command {
	case "resolve", "resolve-definition":
		members := -1
		if row, ok := jsonValue(visible).(map[string]any); ok {
			if repositories, ok := row["repositories"].(map[string]any); ok {
				members = len(repositories)
			}
		}
		runtime.RecordWorkspaceResolve(ctx, outcome, elapsed, members)
	case "search":
		root := jsonValue(visible)
		completeness, partialReason, candidates, hydrated, dropped, authorizationDropped := "unknown", "none", 0, 0, 0, 0
		phases := telemetry.SearchPhases{}
		if searchResult, ok := visible.(retrieval.SearchResult); ok {
			candidates = searchResult.Stats.Candidates
			hydrated = searchResult.Stats.Hydrated
			dropped = searchResult.Stats.Dropped
			authorizationDropped = searchResult.Stats.DroppedAuthorization
			partialReason = searchResult.Stats.PartialReason
			phases = telemetry.SearchPhases{
				Plan: searchResult.Stats.PlanDuration, Probe: searchResult.Stats.ProbeDuration, Hydrate: searchResult.Stats.HydrateDuration,
			}
		}
		if row, ok := root.(map[string]any); ok {
			completeness = boundedTelemetryValue(stringValue(row["completeness"]), "unknown", "complete", "partial")
			if completeness == "partial" && partialReason == "" {
				partialReason = "other"
			}
			if hits, ok := row["hits"].([]any); ok && hydrated == 0 {
				hydrated = len(hits)
			}
		}
		provider := telemetryProvider(flags)
		runtime.RecordSearch(ctx, provider, completeness, partialReason, outcome, elapsed, phases, candidates, hydrated, dropped, authorizationDropped)
	case "put", "remove", "commit", "propose":
		replayed := false
		if row, ok := jsonValue(visible).(map[string]any); ok {
			replayed = strings.EqualFold(stringValue(row["disposition"]), "REPLAYED")
		}
		surface := "COMMIT"
		if command == "propose" {
			surface = "PROPOSAL"
		}
		puts, removes := -1, -1
		if observation.writerCountsSet {
			puts, removes = observation.putCount, observation.removeCount
		}
		payloadBytes := -1
		if observation.writerCountsSet {
			payloadBytes = observation.writerPayloadBytes
		}
		runtime.RecordWriter(ctx, surface, outcome, errorType, replayed, puts, removes, payloadBytes, elapsed)
	case "index-sync":
		mode := "unknown"
		if row, ok := jsonValue(visible).(map[string]any); ok {
			mode = boundedTelemetryValue(stringValue(row["mode"]), "unknown", "ready", "incremental", "rebuild")
		}
		projectionElapsed := elapsed
		if observation.projectionElapsed > 0 {
			projectionElapsed = observation.projectionElapsed
		}
		documents, updated, removed := projectionVolume(visible)
		runtime.RecordProjection(ctx, telemetryProvider(flags), mode, outcome, projectionElapsed, documents, updated, removed)
	}
}

func projectionVolume(value any) (documents, updated, removed int) {
	documents, updated, removed = -1, -1, -1
	row, ok := jsonValue(value).(map[string]any)
	if !ok {
		return
	}
	if snapshot, nested := row["snapshot"].(map[string]any); nested {
		row = snapshot
	}
	if value, ok := row["objectCount"].(float64); ok {
		documents = int(value)
	}
	if value, ok := row["updated"].(float64); ok {
		updated = int(value)
	}
	if value, ok := row["removed"].(float64); ok {
		removed = int(value)
	}
	return
}

func telemetryProvider(flags map[string]FlagValue) string {
	home, err := resolveHome(flags)
	if err != nil {
		return "other"
	}
	stores, err := ReadStores(home)
	if err != nil {
		return "other"
	}
	return boundedTelemetryValue(stores.Index, "other", "none", "opensearch")
}

func boundedTelemetryValue(value, fallback string, allowed ...string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return "other"
}
