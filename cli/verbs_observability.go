package cli

import (
	"fmt"
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/observability"
)

func observabilityVerbs() map[string]command {
	return map[string]command{
		"access-log":      {stage: stageHome, run: verbAccessLog},
		"trace":           {stage: stageHome, run: verbTrace},
		"hitmap":          {stage: stageHome, run: verbHitmap},
		"record-feedback": {stage: stageHome, run: verbRecordFeedback},
	}
}

func observabilityQuery(cx *invocation) (observability.AccessQuery, error) {
	limit, err := limitFrom(cx.Flags, defaultAuditLimit)
	if err != nil {
		return observability.AccessQuery{}, err
	}
	if limit == 0 {
		limit = unboundedLimit
	}
	return observability.AccessQuery{
		Principal:  cx.flag("filter-principal"),
		OnBehalfOf: cx.flag("filter-on-behalf-of"),
		Action:     cx.flag("action"),
		TraceID:    cx.flag("trace-id"),
		Repository: kernel.RepositoryID(cx.flag("repo")),
		Object:     knowledge.ObjectID(cx.flag("object")),
		Limit:      limit,
	}, nil
}

func requireAuditAccess(cx *invocation) error {
	return authorize(cx.Home, "audit", cx.Flags, cx.Observation.authorization)
}

func verbAccessLog(cx *invocation) (any, error) {
	if err := requireAuditAccess(cx); err != nil {
		return nil, err
	}
	query, err := observabilityQuery(cx)
	if err != nil {
		return nil, err
	}
	events, err := observability.NewFileStore(cx.Home).Access(query)
	if err != nil {
		return nil, err
	}
	return map[string]any{"source": "access", "entries": events}, nil
}

func verbTrace(cx *invocation) (any, error) {
	if err := requireAuditAccess(cx); err != nil {
		return nil, err
	}
	traceID, err := cx.require("trace-id")
	if err != nil {
		return nil, err
	}
	return observability.NewFileStore(cx.Home).Trace(traceID)
}

func verbHitmap(cx *invocation) (any, error) {
	if err := requireAuditAccess(cx); err != nil {
		return nil, err
	}
	query, err := observabilityQuery(cx)
	if err != nil {
		return nil, err
	}
	limit := query.Limit
	query.Limit = 0
	hits, err := observability.NewFileStore(cx.Home).Hitmap(query)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return map[string]any{"source": "access", "hits": hits}, nil
}

func verbRecordFeedback(cx *invocation) (any, error) {
	traceID, err := cx.require("trace-id")
	if err != nil {
		return nil, err
	}
	workspaceID, err := cx.workspaceID()
	if err != nil {
		return nil, err
	}
	if err := authorize(cx.Home, "read-workspace", cx.Flags, cx.Observation.authorization); err != nil {
		return nil, err
	}
	outcome, err := cx.require("outcome")
	if err != nil {
		return nil, err
	}
	switch outcome {
	case "accepted", "rejected", "corrected", "helpful", "unhelpful":
	default:
		return nil, fmt.Errorf("--outcome must be accepted, rejected, corrected, helpful, or unhelpful")
	}
	message := strings.TrimSpace(cx.flag("message"))
	if len(message) > 4096 {
		return nil, fmt.Errorf("--message is too long")
	}
	identity, err := identityContextFrom(cx.Flags)
	if err != nil {
		return nil, err
	}
	trace, err := traceContextFrom(cx.Flags)
	if err != nil {
		return nil, err
	}
	trace.TraceID = traceID
	store := observability.NewFileStore(cx.Home)
	access, err := store.Access(observability.AccessQuery{TraceID: traceID, Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(access) == 0 {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "trace %s has no knowledge access", traceID)
	}
	event := observability.FeedbackEvent{
		Identity: identity, Trace: trace, Workspace: workspaceID, Outcome: outcome, Message: message,
	}
	if err := recordFeedbackWithTelemetry(cx.Observation, store, event); err != nil {
		return nil, err
	}
	return map[string]any{"traceId": traceID, "outcome": outcome, "recorded": true}, nil
}
