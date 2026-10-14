package invocation

// The result envelope and the ordered evidence pipeline are shared by both
// transports: one implementation guarantees the CLI and the typed service
// record identical evidence for the same operation (CAP-ORG-01 B5b.3).

import (
	"context"
	"strings"

	retrieval "kc/discovery"
	"kc/internal/telemetry"
	"kc/invoke"
	"kc/kernel"
	"kc/observability"
)

func ShapeInvocationResult(result any, err error) invoke.RunResult {
	if err != nil {
		return ErrorResult(err)
	}
	if ResultOutcome(result) == "partial" {
		// Some repositories have already committed, so this is not an error
		// envelope and must retain every per-repository receipt. It is still a
		// non-zero CLI outcome so automation cannot mistake it for full success.
		return invoke.RunResult{Status: 2, Stdout: invoke.JSONOut(result)}
	}
	if text, ok := result.(string); ok {
		if strings.HasSuffix(text, "\n") {
			return invoke.RunResult{Status: 0, Stdout: text}
		}
		return invoke.RunResult{Status: 0, Stdout: text + "\n"}
	}
	return invoke.RunResult{Status: 0, Stdout: invoke.JSONOut(result)}
}

// EvidenceChain is the single ordered evidence pipeline both public entry
// points owe: access, then retrieval, then refine, then audit. Keeping one
// implementation is what guarantees the CLI and the typed HTTP service record
// identical evidence for the same operation. A persistence failure becomes the
// caller's error so a response is never delivered without its evidence.
func EvidenceChain(
	ctx context.Context,
	runtime *telemetry.Runtime,
	home, command string,
	flags map[string]invoke.FlagValue,
	result any,
	callErr error,
) (any, error) {
	if callErr == nil {
		var retainErr error
		result, retainErr = retainReadResults(ctx, home, flags, result)
		if retainErr != nil {
			callErr = retainErr
			result = nil
		}
	}
	accessStarted := TelemetryNow()
	evidenceID, accessBytes, accessErr := RecordKnowledgeAccess(home, command, flags, result, callErr)
	if runtime != nil && knowledgeAccessCommand(command, flags) {
		runtime.RecordEvidence(ctx, "access", TelemetryOutcome(accessErr), TelemetrySince(accessStarted), accessBytes)
		ObserveEvidenceDisk(runtime, home)
	}
	if accessErr != nil && callErr == nil {
		callErr = accessErr
		result = nil
	}
	if evidenceID != "" {
		flags["_evidence-id"] = evidenceID
	}
	retrievalStarted := TelemetryNow()
	retrievalID, retrievalBytes, retrievalErr := RecordRetrievalEvidence(home, command, flags, result, evidenceID, callErr)
	if runtime != nil && (retrievalID != "" || retrievalErr != nil) {
		runtime.RecordEvidence(ctx, "retrieval", TelemetryOutcome(retrievalErr), TelemetrySince(retrievalStarted), retrievalBytes)
		ObserveEvidenceDisk(runtime, home)
	}
	if retrievalErr != nil && callErr == nil {
		callErr = retrievalErr
	}
	if retrievalID != "" {
		flags["_retrieval-evidence-id"] = retrievalID
		result = attachRetrievalEvidenceID(result, retrievalID)
	}
	refineStarted := TelemetryNow()
	refineID, refineBytes, refineErr := RecordRefineEvidence(home, result, evidenceID, retrievalID)
	if runtime != nil && (refineID != "" || refineErr != nil) {
		runtime.RecordEvidence(ctx, "refine", TelemetryOutcome(refineErr), TelemetrySince(refineStarted), refineBytes)
		ObserveEvidenceDisk(runtime, home)
	}
	if refineErr != nil && callErr == nil {
		callErr = refineErr
		result = nil
	}
	if refineID != "" {
		flags["_refine-evidence-id"] = refineID
		result = AttachRefineEvidenceID(result, refineID)
	}
	result = accessOutput(result)
	auditStarted := TelemetryNow()
	auditBytes, auditErr := RecordAudit(home, command, flags, result, callErr)
	if runtime != nil && shouldAudit(command, flags) {
		runtime.RecordEvidence(ctx, "audit", TelemetryOutcome(auditErr), TelemetrySince(auditStarted), auditBytes)
		ObserveEvidenceDisk(runtime, home)
	}
	if auditErr != nil && callErr == nil {
		callErr = auditErr
		result = nil
	}
	return result, callErr
}

func ObserveEvidenceDisk(runtime *telemetry.Runtime, home string) {
	if runtime == nil || strings.TrimSpace(home) == "" {
		return
	}
	if used, ok := observability.NewFileStore(home).DiskUsedFraction(); ok {
		runtime.SetEvidenceStoreUsedRatio(used)
	}
}

func ErrorResult(err error) invoke.RunResult {
	return invoke.RunResult{Status: 1, Stdout: invoke.JSONOut(kernel.FaultJSON(err))}
}

func ResultOutcome(result any) string {
	if row, ok := JSONValue(accessOutput(result)).(map[string]any); ok {
		return strings.ToLower(StringValue(row["outcome"]))
	}
	return ""
}

func AttachRefineEvidenceID(result any, evidenceID string) any {
	observed, ok := result.(ObservedAccessResult)
	if !ok || evidenceID == "" {
		return result
	}
	switch output := observed.Output.(type) {
	case retrieval.SemanticRerankResult:
		if output.Evidence != nil {
			output.Evidence.RefineEvidenceID = evidenceID
		}
		observed.Output = output
	case SearchRerankResult:
		if output.Rerank.Evidence != nil {
			output.Rerank.Evidence.RefineEvidenceID = evidenceID
		}
		observed.Output = output
	}
	return observed
}

// SearchRerankResult is the pipeline's combined SEARCH+RERANK output: the
// rerank Run closure produces it and the evidence chain attaches refine
// evidence to its lane. (CLI alias: searchRerankResult.)
type SearchRerankResult struct {
	Retrieval retrieval.SearchResult         `json:"retrieval"`
	Rerank    retrieval.SemanticRerankResult `json:"rerank"`
}
