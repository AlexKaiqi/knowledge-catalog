package cli

import (
	"context"
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
	limit, err := pageLimit(cx.Flags, defaultAuditLimit, maxAuditPageSize)
	if err != nil {
		return observability.AccessQuery{}, err
	}
	return observability.AccessQuery{
		EvidenceID:   cx.flag("evidence-id"),
		Since:        cx.flag("since"),
		Until:        cx.flag("until"),
		Principal:    cx.flag("filter-principal"),
		OnBehalfOf:   cx.flag("filter-on-behalf-of"),
		Action:       cx.flag("action"),
		TraceID:      observabilityFilterTraceID(cx),
		Repository:   kernel.RepositoryID(cx.flag("repo")),
		Object:       knowledge.ObjectID(cx.flag("object")),
		Limit:        limit,
		Continuation: cx.flag("continuation"),
	}, nil
}

func observabilityFilterTraceID(cx *invocation) string {
	if _, typedQuery := cx.Flags["_filter-trace-id"]; typedQuery {
		return strings.TrimSpace(FlagString(cx.Flags, "_filter-trace-id"))
	}
	// Local CLI calls still use --trace-id as the query operand. Typed HTTP
	// routes stamp _filter-trace-id before adding their own transport trace.
	return cx.flag("trace-id")
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
	page, err := observability.NewFileStore(cx.Home).Access(context.Background(), query)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"source": "access", "entries": page.Entries, "exhausted": page.Exhausted}
	if page.Continuation != "" {
		out["continuation"] = page.Continuation
	}
	if page.CompleteThrough != "" {
		out["completeThrough"] = page.CompleteThrough
	}
	return out, nil
}

func verbTrace(cx *invocation) (any, error) {
	if err := requireAuditAccess(cx); err != nil {
		return nil, err
	}
	traceID := observabilityFilterTraceID(cx)
	if traceID == "" {
		return nil, fmt.Errorf("missing --trace-id")
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
	hits, err := observability.NewFileStore(cx.Home).Hitmap(query)
	if err != nil {
		return nil, err
	}
	if query.Limit > 0 && len(hits) > query.Limit {
		hits = hits[:query.Limit]
	}
	return map[string]any{"source": "access", "hits": hits}, nil
}

func verbRefineLog(cx *invocation) (any, error) {
	if err := requireAuditAccess(cx); err != nil {
		return nil, err
	}
	limit, err := pageLimit(cx.Flags, defaultAuditLimit, maxAuditPageSize)
	if err != nil {
		return nil, err
	}
	events, err := observability.NewFileStore(cx.Home).Refine(observability.RefineQuery{
		EvidenceID: FlagString(cx.Flags, "_refine-evidence-id"), TraceID: observabilityFilterTraceID(cx),
		Provider: FlagString(cx.Flags, "_provider"), Model: FlagString(cx.Flags, "_model"),
		Outcome: FlagString(cx.Flags, "_outcome"), Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"source": "refine", "entries": events}, nil
}

func retrievalEvidenceQuery(cx *invocation) (observability.RetrievalQuery, error) {
	limit, err := pageLimit(cx.Flags, defaultAuditLimit, maxAuditPageSize)
	if err != nil {
		return observability.RetrievalQuery{}, err
	}
	return observability.RetrievalQuery{
		EvidenceID: FlagString(cx.Flags, "_retrieval-evidence-id"), TraceID: observabilityFilterTraceID(cx),
		Operator: strings.ToUpper(FlagString(cx.Flags, "_operator")), Provider: FlagString(cx.Flags, "_provider"),
		Outcome: strings.ToUpper(FlagString(cx.Flags, "_outcome")), Limit: limit,
	}, nil
}

func verbRetrievalLog(cx *invocation) (any, error) {
	if err := requireAuditAccess(cx); err != nil {
		return nil, err
	}
	query, err := retrievalEvidenceQuery(cx)
	if err != nil {
		return nil, err
	}
	events, err := observability.NewFileStore(cx.Home).Retrieval(query)
	if err != nil {
		return nil, err
	}
	return map[string]any{"source": "retrieval", "entries": events}, nil
}

func verbRetrievalTrainingSamples(cx *invocation) (any, error) {
	if err := requireAuditAccess(cx); err != nil {
		return nil, err
	}
	query, err := retrievalEvidenceQuery(cx)
	if err != nil {
		return nil, err
	}
	samples, err := observability.NewFileStore(cx.Home).RetrievalTrainingSamples(query)
	if err != nil {
		return nil, err
	}
	return map[string]any{"source": "retrieval+feedback", "samples": samples}, nil
}

func verbRerankTrainingSamples(cx *invocation) (any, error) {
	if err := requireAuditAccess(cx); err != nil {
		return nil, err
	}
	limit, err := pageLimit(cx.Flags, defaultAuditLimit, maxAuditPageSize)
	if err != nil {
		return nil, err
	}
	samples, err := observability.NewFileStore(cx.Home).RerankTrainingSamples(observability.RefineQuery{
		EvidenceID: FlagString(cx.Flags, "_refine-evidence-id"), TraceID: observabilityFilterTraceID(cx),
		Provider: FlagString(cx.Flags, "_provider"), Model: FlagString(cx.Flags, "_model"),
		Outcome: FlagString(cx.Flags, "_outcome"), Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"source": "refine+feedback", "samples": samples}, nil
}

func verbRecordFeedback(cx *invocation) (any, error) {
	traceID := strings.TrimSpace(FlagString(cx.Flags, "_feedback-target-trace-id"))
	if traceID == "" {
		var err error
		traceID, err = cx.require("trace-id")
		if err != nil {
			return nil, err
		}
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
	case "answered", "accepted", "rejected", "corrected", "helpful", "unhelpful":
	default:
		return nil, fmt.Errorf("--outcome must be answered, accepted, rejected, corrected, helpful, or unhelpful")
	}
	message := strings.TrimSpace(cx.flag("message"))
	if len(message) > 4096 {
		return nil, fmt.Errorf("--message is too long")
	}
	identity, err := identityContextFrom(cx.Flags)
	if err != nil {
		return nil, err
	}
	submissionTrace, err := traceContextFrom(cx.Flags)
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
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "trace %s has no knowledge access", traceID)
	}
	refineEvidenceID := strings.TrimSpace(FlagString(cx.Flags, "_refine-evidence-id"))
	retrievalEvidenceID := strings.TrimSpace(FlagString(cx.Flags, "_retrieval-evidence-id"))
	answer := strings.TrimSpace(FlagString(cx.Flags, "_feedback-answer"))
	selected, _ := cx.Flags["_feedback-selected"].([]knowledge.KnowledgeRef)
	ideal, _ := cx.Flags["_feedback-ideal"].([]observability.RefineRankGroup)
	if refineEvidenceID != "" {
		refines, err := store.Refine(observability.RefineQuery{EvidenceID: refineEvidenceID, Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(refines) != 1 || refines[0].Trace.TraceID != traceID || refines[0].Workspace != workspaceID {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "refine evidence does not belong to this trace and Workspace")
		}
		if err := validateFeedbackRefs(refines[0], selected, ideal); err != nil {
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
		if len(retrievals) != 1 || retrievals[0].Trace.TraceID != traceID || retrievals[0].Workspace != workspaceID {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "retrieval evidence does not belong to this trace and Workspace")
		}
		if refineEvidenceID == "" {
			if err := validateRetrievalFeedbackRefs(retrievals[0], selected, ideal); err != nil {
				return nil, err
			}
		}
	} else if refineEvidenceID == "" && (answer != "" || len(selected) > 0 || len(ideal) > 0) {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "structured retrieval feedback requires retrievalEvidenceId or refineEvidenceId")
	}
	labelSource := "agent"
	if principalKind(identity.Principal) == "user" {
		labelSource = "user"
	}
	event := observability.FeedbackEvent{
		Identity: identity, Trace: trace, SubmissionTrace: recordedSubmissionTrace, Workspace: workspaceID, Outcome: outcome, Message: message,
		RetrievalEvidenceID: retrievalEvidenceID,
		RefineEvidenceID:    refineEvidenceID, LabelSource: labelSource, Answer: answer,
		SelectedRefs: append([]knowledge.KnowledgeRef(nil), selected...), IdealGroups: append([]observability.RefineRankGroup(nil), ideal...),
	}
	if err := recordFeedbackWithTelemetry(cx.Observation, store, event); err != nil {
		return nil, err
	}
	return map[string]any{"traceId": traceID, "retrievalEvidenceId": retrievalEvidenceID, "refineEvidenceId": refineEvidenceID, "outcome": outcome, "recorded": true}, nil
}

func validateFeedbackRefs(refine observability.RefineEvent, selected []knowledge.KnowledgeRef, ideal []observability.RefineRankGroup) error {
	allowed := map[knowledge.KnowledgeRef]struct{}{}
	for _, candidate := range refine.Candidates {
		allowed[candidate.KnowledgeRef.KnowledgeRef] = struct{}{}
	}
	check := func(ref knowledge.KnowledgeRef) error {
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

func validateRetrievalFeedbackRefs(retrievalEvent observability.RetrievalEvent, selected []knowledge.KnowledgeRef, ideal []observability.RefineRankGroup) error {
	allowed := map[knowledge.KnowledgeRef]struct{}{}
	for _, candidate := range retrievalEvent.Candidates {
		allowed[candidate.KnowledgeRef.KnowledgeRef] = struct{}{}
	}
	check := func(ref knowledge.KnowledgeRef) error {
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
