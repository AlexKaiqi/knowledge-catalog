package cli

import (
	"context"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"

	"kc/internal/telemetry"
	"kc/kernel"
)

type telemetryStart struct {
	span trace.Span
	at   time.Time
}

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
		code = kernel.ErrUsageInvalid
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
	if err != nil || command != "search" {
		return outcome, errorType
	}
	if row, ok := jsonValue(accessOutput(result)).(map[string]any); ok && stringValue(row["completeness"]) == "partial" {
		return "partial", ""
	}
	return outcome, errorType
}

func telemetryFace(command string) string {
	switch command {
	case "put", "remove", "commit", "ingest", "receipt":
		return "writer"
	case "search", "describe-index", "index-sync", "describe-access":
		return "projection"
	case "read", "list", "relations", "provenance", "log", "diff", "resolve-binding", "describe-schema":
		return "knowledge"
	case "resolve", "inspect", "checkout", "define-workspace", "retire-workspace", "repo-add", "archive-repo", "catalog-add", "archive-catalog":
		return "catalog"
	case "vfs-read", "vfs-list":
		return "vfs"
	case "merge", "validate", "record-validation":
		return "control"
	default:
		return "other"
	}
}

func recordDomainTelemetry(ctx context.Context, runtime *telemetry.Runtime, command string, flags map[string]FlagValue, result any, callErr error, elapsed time.Duration) {
	outcome, errorType := telemetryResultFor(command, result, callErr)
	if cmd, ok := commands[command]; ok && cmd.stage == stageGoverned {
		decision := "allow"
		if outcome == "denied" {
			decision = "deny"
		}
		runtime.RecordAuthorization(ctx, command, decision)
	}
	visible := accessOutput(result)
	switch command {
	case "search":
		root := jsonValue(visible)
		completeness, partialReason, hydrated := "unknown", "none", 0
		if row, ok := root.(map[string]any); ok {
			completeness = boundedTelemetryValue(stringValue(row["completeness"]), "unknown", "complete", "partial")
			if completeness == "partial" {
				partialReason = "other"
			}
			if hits, ok := row["hits"].([]any); ok {
				hydrated = len(hits)
			}
		}
		provider := telemetryProvider(flags)
		runtime.RecordSearch(ctx, provider, completeness, partialReason, outcome, elapsed, hydrated)
	case "put", "remove", "commit":
		replayed := false
		if row, ok := jsonValue(visible).(map[string]any); ok {
			replayed = strings.EqualFold(stringValue(row["disposition"]), "REPLAYED")
		}
		runtime.RecordWriter(ctx, "COMMIT", outcome, errorType, replayed)
	case "index-sync":
		mode, cause := "unknown", "unknown"
		if row, ok := jsonValue(visible).(map[string]any); ok {
			mode = boundedTelemetryValue(stringValue(row["mode"]), "unknown", "ready", "incremental", "rebuild")
			cause = boundedTelemetryValue(stringValue(row["cause"]), "unknown", "ready", "content", "schema", "cold", "diverged")
		}
		runtime.RecordProjection(ctx, telemetryProvider(flags), mode, cause, outcome, elapsed)
	}
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
	return boundedTelemetryValue(stores.Index, "other", "sqlite", "opensearch", "starrocks")
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
