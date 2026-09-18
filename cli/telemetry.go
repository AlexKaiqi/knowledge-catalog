package cli

import (
	"context"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"

	"kc/internal/telemetry"
	"kc/kernel"
	"kc/knowledge"
	knowledgeserving "kc/knowledge/serving"
	"kc/retrieval"
)

type telemetryStart struct {
	span trace.Span
	at   time.Time
}

type projectionBacklogObserver func(lagging int, oldestPendingAt time.Time)
type evidenceTelemetryObserver func(kind, outcome string, elapsed time.Duration, bytes int64)

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
	case kernel.ErrKnowledgeRefUnresolved, kernel.ErrVersionUnresolved, kernel.ErrKnowledgeSetInvalid:
		outcome = "unresolved"
	case kernel.ErrNonFastForward, kernel.ErrIdempotencyConflict, kernel.ErrObjectIDConflict,
		kernel.ErrEventIDConflict, kernel.ErrCandidateMoved, kernel.ErrSchemaIncompatible:
		outcome = "conflict"
	case kernel.ErrUsageInvalid, kernel.ErrPreconditionFailed, kernel.ErrWriteTargetRequired,
		kernel.ErrSurfaceMismatch, kernel.ErrScopeDenied, kernel.ErrSchemaUnsupported,
		kernel.ErrSchemaRevisionUnresolved, kernel.ErrSchemaInstanceInvalid, kernel.ErrTargetRepositoryDenied:
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
	if command != "knowledge-search" {
		return outcome, errorType
	}
	if row, ok := jsonValue(accessOutput(result)).(map[string]any); ok && stringValue(row["completeness"]) == "partial" {
		return "partial", ""
	}
	return outcome, errorType
}

func telemetryFace(command string) string {
	switch {
	case strings.HasPrefix(command, "writer-") || command == "diff":
		return "writer"
	case command == "knowledge-search" || strings.HasPrefix(command, "operations-projection-") || command == "operations-access-spec-describe":
		return "projection"
	case strings.HasPrefix(command, "knowledge-") || command == "rerank" || command == "search-rerank":
		return "knowledge"
	case strings.HasPrefix(command, "workspace-") || strings.HasPrefix(command, "catalog-") ||
		strings.HasPrefix(command, "dataset-") || command == "pin" || command == "pin-check" ||
		command == "pin-source" || command == "local-repository-attach" || command == "local-catalog-attach" ||
		command == "named-repository-create" || command == "create" || command == "attach" ||
		command == "detach" || command == "show":
		return "catalog"
	case strings.HasPrefix(command, "file-"):
		return "vfs"
	case strings.HasPrefix(command, "governance-") || strings.HasPrefix(command, "admin-") ||
		strings.HasPrefix(command, "grant-") || strings.HasPrefix(command, "admission-"):
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
	case "pin", "pin-source", "pin-check":
		members := -1
		if row, ok := jsonValue(visible).(map[string]any); ok {
			if repositories, ok := row["repositories"].(map[string]any); ok {
				members = len(repositories)
			}
		}
		runtime.RecordWorkspaceResolve(ctx, outcome, elapsed, members)
	case "knowledge-read":
		objects, units := knowledgeReadFanout(visible)
		runtime.RecordKnowledgeReadFanout(ctx, objects, units)
	case "knowledge-search":
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
	case "writer-put", "writer-remove", "writer-commit", "governance-proposal-create":
		replayed := false
		if row, ok := jsonValue(visible).(map[string]any); ok {
			replayed = strings.EqualFold(stringValue(row["disposition"]), "REPLAYED")
		}
		surface := "COMMIT"
		if command == "governance-proposal-create" {
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
	case "operations-projection-sync", "operations-projection-notice":
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

func knowledgeReadFanout(visible any) (objects, units int) {
	objects, units = -1, -1
	switch typed := visible.(type) {
	case []knowledgeserving.ReadResult:
		seen := map[string]struct{}{}
		units = 0
		for _, item := range typed {
			seen[string(item.Repository)+"\x00"+string(item.ObjectID)] = struct{}{}
			if n := len(item.Units); n > 0 {
				units += n
			} else {
				units++
			}
		}
		objects = len(seen)
	case knowledgeserving.ReadResult:
		objects = 1
		if n := len(typed.Units); n > 0 {
			units = n
		} else {
			units = 1
		}
	case knowledge.KnowledgeValue:
		objects = 1
		if n := len(typed.Units); n > 0 {
			units = n
		} else {
			units = 1
		}
	case []knowledge.KnowledgeValue:
		seen := map[string]struct{}{}
		units = 0
		for _, item := range typed {
			seen[string(item.Repository)+"\x00"+string(item.KnowledgeRef.Object)] = struct{}{}
			if n := len(item.Units); n > 0 {
				units += n
			} else {
				units++
			}
		}
		objects = len(seen)
	}
	return objects, units
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
