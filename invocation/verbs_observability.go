package invocation

import (
	"context"
	"fmt"
	"strings"

	"kc/identity"
	"kc/invoke"
	"kc/kernel"
	liveservice "kc/liveaccess/service"
	objectmodel "kc/object"
	"kc/observability"
)

func ObservabilityQuery(cx *Invocation) (observability.AccessQuery, error) {
	limit, err := PageLimit(cx.Flags, DefaultAuditLimit, MaxAuditPageSize)
	if err != nil {
		return observability.AccessQuery{}, err
	}
	return observability.AccessQuery{
		EvidenceID:   cx.Flag("evidence-id"),
		Since:        cx.Flag("since"),
		Until:        cx.Flag("until"),
		Principal:    cx.Flag("filter-principal"),
		OnBehalfOf:   cx.Flag("filter-on-behalf-of"),
		Action:       cx.Flag("action"),
		TraceID:      ObservabilityFilterTraceID(cx),
		RequestID:    cx.Flag("filter-request-id"),
		Repository:   kernel.RepositoryID(cx.Flag("repo")),
		Object:       objectmodel.ObjectID(cx.Flag("object")),
		Limit:        limit,
		Continuation: cx.Flag("continuation"),
	}, nil
}

func ObservabilityFilterTraceID(cx *Invocation) string {
	if _, typedQuery := cx.Flags["_filter-trace-id"]; typedQuery {
		return strings.TrimSpace(invoke.FlagString(cx.Flags, "_filter-trace-id"))
	}
	// Local CLI calls still use --trace-id as the query operand. Typed HTTP
	// routes stamp _filter-trace-id before adding their own transport trace.
	return cx.Flag("trace-id")
}

func RequireAuditAccess(cx *Invocation) error {
	return Authorize(cx.Home, "audit", cx.Flags, cx.Observation.Authorization)
}

func VerbAccessLog(cx *Invocation) (any, error) {
	if err := RequireAuditAccess(cx); err != nil {
		return nil, err
	}
	query, err := ObservabilityQuery(cx)
	if err != nil {
		return nil, err
	}
	page, err := observability.NewFileStore(cx.Home).Access(context.Background(), query)
	if err != nil {
		return nil, err
	}
	store, err := liveservice.OpenResultStore(cx.Home)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"source": "access", "entries": liveservice.AuditEntries(cx.Context, store, page.Entries)}
	if page.Continuation != "" {
		out["continuation"] = page.Continuation
	}
	if page.CompleteThrough != "" {
		out["completeThrough"] = page.CompleteThrough
	}
	return out, nil
}

func VerbTrace(cx *Invocation) (any, error) {
	if err := RequireAuditAccess(cx); err != nil {
		return nil, err
	}
	traceID := ObservabilityFilterTraceID(cx)
	if traceID == "" {
		return nil, fmt.Errorf("missing --trace-id")
	}
	return observability.NewFileStore(cx.Home).Trace(traceID)
}

func VerbHitmap(cx *Invocation) (any, error) {
	if err := RequireAuditAccess(cx); err != nil {
		return nil, err
	}
	query, err := ObservabilityQuery(cx)
	if err != nil {
		return nil, err
	}
	hits, err := observability.NewFileStore(cx.Home).Hitmap(query)
	if err != nil {
		return nil, err
	}
	if query.Limit > 0 && len(hits) > query.Limit {
		hits = hits[:query.Limit]
	}
	return map[string]any{"source": "access", "hits": hits}, nil
}

func VerbRefineLog(cx *Invocation) (any, error) {
	if err := RequireAuditAccess(cx); err != nil {
		return nil, err
	}
	limit, err := PageLimit(cx.Flags, DefaultAuditLimit, MaxAuditPageSize)
	if err != nil {
		return nil, err
	}
	events, err := observability.NewFileStore(cx.Home).Refine(observability.RefineQuery{
		EvidenceID: invoke.FlagString(cx.Flags, "_refine-evidence-id"), TraceID: ObservabilityFilterTraceID(cx),
		Provider: invoke.FlagString(cx.Flags, "_provider"), Model: invoke.FlagString(cx.Flags, "_model"),
		Outcome: invoke.FlagString(cx.Flags, "_outcome"), Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"source": "refine", "entries": events}, nil
}

func RetrievalEvidenceQuery(cx *Invocation) (observability.RetrievalQuery, error) {
	limit, err := PageLimit(cx.Flags, DefaultAuditLimit, MaxAuditPageSize)
	if err != nil {
		return observability.RetrievalQuery{}, err
	}
	return observability.RetrievalQuery{
		EvidenceID: invoke.FlagString(cx.Flags, "_retrieval-evidence-id"), TraceID: ObservabilityFilterTraceID(cx),
		Operator: strings.ToUpper(invoke.FlagString(cx.Flags, "_operator")), Provider: invoke.FlagString(cx.Flags, "_provider"),
		Outcome: strings.ToUpper(invoke.FlagString(cx.Flags, "_outcome")), Limit: limit,
	}, nil
}

func VerbRetrievalLog(cx *Invocation) (any, error) {
	if err := RequireAuditAccess(cx); err != nil {
		return nil, err
	}
	query, err := RetrievalEvidenceQuery(cx)
	if err != nil {
		return nil, err
	}
	events, err := observability.NewFileStore(cx.Home).Retrieval(query)
	if err != nil {
		return nil, err
	}
	return map[string]any{"source": "retrieval", "entries": events}, nil
}

func VerbRetrievalTrainingSamples(cx *Invocation) (any, error) {
	if err := RequireAuditAccess(cx); err != nil {
		return nil, err
	}
	query, err := RetrievalEvidenceQuery(cx)
	if err != nil {
		return nil, err
	}
	samples, err := observability.NewFileStore(cx.Home).RetrievalTrainingSamples(query)
	if err != nil {
		return nil, err
	}
	return map[string]any{"source": "retrieval+feedback", "samples": samples}, nil
}

func VerbRerankTrainingSamples(cx *Invocation) (any, error) {
	if err := RequireAuditAccess(cx); err != nil {
		return nil, err
	}
	limit, err := PageLimit(cx.Flags, DefaultAuditLimit, MaxAuditPageSize)
	if err != nil {
		return nil, err
	}
	samples, err := observability.NewFileStore(cx.Home).RerankTrainingSamples(observability.RefineQuery{
		EvidenceID: invoke.FlagString(cx.Flags, "_refine-evidence-id"), TraceID: ObservabilityFilterTraceID(cx),
		Provider: invoke.FlagString(cx.Flags, "_provider"), Model: invoke.FlagString(cx.Flags, "_model"),
		Outcome: invoke.FlagString(cx.Flags, "_outcome"), Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"source": "refine+feedback", "samples": samples}, nil
}

func VerbRecordFeedback(cx *Invocation) (any, error) {
	traceID := strings.TrimSpace(invoke.FlagString(cx.Flags, "_feedback-target-trace-id"))
	if traceID == "" {
		var err error
		traceID, err = cx.Require("trace-id")
		if err != nil {
			return nil, err
		}
	}
	setID, err := cx.SetID()
	if err != nil {
		return nil, err
	}
	if err := Authorize(cx.Home, "read-workspace", cx.Flags, cx.Observation.Authorization); err != nil {
		return nil, err
	}
	outcome, err := cx.Require("outcome")
	if err != nil {
		return nil, err
	}
	switch outcome {
	case "answered", "accepted", "rejected", "corrected", "helpful", "unhelpful":
	default:
		return nil, fmt.Errorf("--outcome must be answered, accepted, rejected, corrected, helpful, or unhelpful")
	}
	message := strings.TrimSpace(cx.Flag("message"))
	if len(message) > 4096 {
		return nil, fmt.Errorf("--message is too long")
	}
	identity, err := IdentityContextFrom(cx.Flags)
	if err != nil {
		return nil, err
	}
	submissionTrace, err := TraceContextFrom(cx.Flags)
	if err != nil {
		return nil, err
	}
	trace := observability.TraceContext{TraceID: traceID}
	if err := trace.Validate(); err != nil {
		return nil, err
	}
	var recordedSubmissionTrace *observability.TraceContext
	if submissionTrace.TraceID == traceID {
		trace = submissionTrace
	} else if submissionTrace.TraceID != "" {
		copy := submissionTrace
		recordedSubmissionTrace = &copy
	}
	store := observability.NewFileStore(cx.Home)
	access, err := store.Access(context.Background(), observability.AccessQuery{TraceID: traceID, Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(access.Entries) == 0 {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "trace %s has no access", traceID)
	}
	refineEvidenceID := strings.TrimSpace(invoke.FlagString(cx.Flags, "_refine-evidence-id"))
	retrievalEvidenceID := strings.TrimSpace(invoke.FlagString(cx.Flags, "_retrieval-evidence-id"))
	answer := strings.TrimSpace(invoke.FlagString(cx.Flags, "_feedback-answer"))
	selected, _ := cx.Flags["_feedback-selected"].([]objectmodel.ObjectRef)
	ideal, _ := cx.Flags["_feedback-ideal"].([]observability.RefineRankGroup)
	if refineEvidenceID != "" {
		refines, err := store.Refine(observability.RefineQuery{EvidenceID: refineEvidenceID, Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(refines) != 1 || refines[0].Trace.TraceID != traceID || refines[0].Dataset != setID {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "refine evidence does not belong to this trace and Workspace")
		}
		if err := ValidateFeedbackRefs(refines[0], selected, ideal); err != nil {
			return nil, err
		}
		if retrievalEvidenceID == "" {
			retrievalEvidenceID = refines[0].RetrievalEvidenceID
		}
		if retrievalEvidenceID != "" && refines[0].RetrievalEvidenceID != retrievalEvidenceID {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "refine evidence does not belong to retrieval evidence")
		}
	}
	if retrievalEvidenceID != "" {
		retrievals, err := store.Retrieval(observability.RetrievalQuery{EvidenceID: retrievalEvidenceID, Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(retrievals) != 1 || retrievals[0].Trace.TraceID != traceID || retrievals[0].Dataset != setID {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "retrieval evidence does not belong to this trace and Workspace")
		}
		if refineEvidenceID == "" {
			if err := ValidateRetrievalFeedbackRefs(retrievals[0], selected, ideal); err != nil {
				return nil, err
			}
		}
	} else if refineEvidenceID == "" && (answer != "" || len(selected) > 0 || len(ideal) > 0) {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "structured retrieval feedback requires retrievalEvidenceId or refineEvidenceId")
	}
	labelSource := "agent"
	if PrincipalKind(identity.Principal) == "user" {
		labelSource = "user"
	}
	event := observability.FeedbackEvent{
		Identity: identity, Trace: trace, SubmissionTrace: recordedSubmissionTrace, Dataset: setID, Outcome: outcome, Message: message,
		RetrievalEvidenceID: retrievalEvidenceID,
		RefineEvidenceID:    refineEvidenceID, LabelSource: labelSource, Answer: answer,
		SelectedRefs: append([]objectmodel.ObjectRef(nil), selected...), IdealGroups: append([]observability.RefineRankGroup(nil), ideal...),
	}
	if err := RecordFeedbackWithTelemetry(cx.Observation, store, event); err != nil {
		return nil, err
	}
	return map[string]any{"traceId": traceID, "retrievalEvidenceId": retrievalEvidenceID, "refineEvidenceId": refineEvidenceID, "outcome": outcome, "recorded": true}, nil
}

func ValidateFeedbackRefs(refine observability.RefineEvent, selected []objectmodel.ObjectRef, ideal []observability.RefineRankGroup) error {
	allowed := map[objectmodel.ObjectRef]struct{}{}
	for _, candidate := range refine.Candidates {
		allowed[candidate.ObjectRef.ObjectRef] = struct{}{}
	}
	check := func(ref objectmodel.ObjectRef) error {
		if _, ok := allowed[ref]; !ok {
			return kernel.Fail(kernel.ErrUsageInvalid, "feedback ref is not a candidate in refine evidence")
		}
		return nil
	}
	for _, ref := range selected {
		if err := check(ref); err != nil {
			return err
		}
	}
	for _, group := range ideal {
		for _, ref := range group.Refs {
			if err := check(ref); err != nil {
				return err
			}
		}
	}
	return nil
}

func ValidateRetrievalFeedbackRefs(retrievalEvent observability.RetrievalEvent, selected []objectmodel.ObjectRef, ideal []observability.RefineRankGroup) error {
	allowed := map[objectmodel.ObjectRef]struct{}{}
	for _, candidate := range retrievalEvent.Candidates {
		allowed[candidate.ObjectRef.ObjectRef] = struct{}{}
	}
	check := func(ref objectmodel.ObjectRef) error {
		if _, ok := allowed[ref]; !ok {
			return kernel.Fail(kernel.ErrUsageInvalid, "feedback ref is not a candidate in retrieval evidence")
		}
		return nil
	}
	for _, ref := range selected {
		if err := check(ref); err != nil {
			return err
		}
	}
	for _, group := range ideal {
		for _, ref := range group.Refs {
			if err := check(ref); err != nil {
				return err
			}
		}
	}
	return nil
}

func PrincipalKind(principal string) string {
	principal = strings.ToLower(strings.TrimSpace(principal))
	switch {
	case principal == "owner" || strings.HasPrefix(principal, "owner:"):
		return "owner"
	case strings.HasPrefix(principal, "agent:"):
		return "agent"
	case strings.HasPrefix(principal, "service:") || strings.Contains(principal, ":service:"):
		return "service"
	case strings.HasPrefix(principal, "user:"), strings.HasPrefix(principal, "oidc:"), strings.HasPrefix(principal, "taihu:"):
		return "user"
	default:
		if _, err := identity.CanonicalUsername(principal); err == nil {
			return "user"
		}
		return "other"
	}
}

func RecordFeedbackWithTelemetry(observation *invoke.ObservationTelemetry, store *observability.FileStore, event observability.FeedbackEvent) error {
	started := TelemetryNow()
	err := store.RecordFeedback(context.Background(), event)
	if observe := invoke.NoObservationTelemetry(observation).Evidence; observe != nil {
		observe("feedback", TelemetryOutcome(err), TelemetrySince(started), store.LastAppendBytes())
		if observation != nil && observation.Runtime != nil {
			if used, ok := store.DiskUsedFraction(); ok {
				observation.Runtime.SetEvidenceStoreUsedRatio(used)
			}
		}
	}
	return err
}
