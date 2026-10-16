package server

import (
	inv "kc/invocation"
	"kc/invoke"

	"encoding/json"
	"net/http"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/observability"
)

// registerManagementRoutes owns the non-consumer service namespaces. The
// request DTOs below are resource-specific: no endpoint accepts a CLI verb or
// an arbitrary flag map.
func (f *httpFacade) registerManagementRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /object/v1/plugins/{name}", f.pluginHelp)
	mux.HandleFunc("GET /object/v1/plugins/{name}/capabilities", f.pluginCapabilities)
	mux.HandleFunc("POST /object/v1/plugins/{name}/declarations:trial", f.trialDeclarations)
	f.registerRepositoryRoutes(mux)
	f.snapshotStoreService().Register(mux)
	mux.HandleFunc("POST /snapshot/v1/repositories:create", f.registryRepositoryCreate)
	mux.HandleFunc("POST /snapshot/v1/repositories/{repository}/archive", f.registryRepositoryArchive)

	mux.HandleFunc("POST /writer/v1/repositories/{repository}/commits", f.writerCommit)
	mux.HandleFunc("GET /writer/v1/repositories/{repository}/head", f.writerHead)
	mux.HandleFunc("GET /writer/v1/receipts/{command}", f.writerReceipt)
	mux.HandleFunc("POST /writer/v1/repositories/{repository}/branches", f.writerBranch)
	mux.HandleFunc("POST /writer/v1/repositories/{repository}/merges", f.writerMerge)

	mux.HandleFunc("GET /operations/v1/stores", f.observeStores)
	mux.HandleFunc("POST /operations/v1/projections:describe", f.projectionDescribe)
	mux.HandleFunc("POST /operations/v1/access-specs:describe", f.accessSpecDescribe)
	mux.HandleFunc("GET /operations/v1/hooks", f.hookList)
	mux.HandleFunc("POST /operations/v1/hooks", f.hookAdd)
	mux.HandleFunc("DELETE /operations/v1/hooks/{hook}", f.hookRemove)
	mux.HandleFunc("POST /operations/v1/access-log:query", f.accessLog)
	mux.HandleFunc("POST /operations/v1/traces:query", f.traceGet)
	mux.HandleFunc("POST /operations/v1/hitmap:query", f.hitmap)
	mux.HandleFunc("POST /operations/v1/retrieval-log:query", f.retrievalLog)
	mux.HandleFunc("POST /operations/v1/retrieval-training:query", f.retrievalTraining)
	mux.HandleFunc("POST /operations/v1/refine-log:query", f.refineLog)
	mux.HandleFunc("POST /operations/v1/rerank-training:query", f.rerankTraining)
	mux.HandleFunc("POST /operations/v1/feedback", f.feedbackRecord)
}

type registryRepositoryCreateRequest struct {
	Repository string `json:"repository"`
	CommandID  string `json:"commandId"`
}

func (f *httpFacade) registryRepositoryCreate(w http.ResponseWriter, r *http.Request) {
	var request registryRepositoryCreateRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := map[string]FlagValue{}
	flags["repo"], flags["command-id"] = request.Repository, request.CommandID
	if err := inv.ValidateManagedRepositoryCoordinates(flags); err != nil {
		invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(err))
		return
	}
	f.executeTyped(w, r, "repository-create", "repositories.create", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbCreateManagedRepository}, flags)
}

func (f *httpFacade) registryRepositoryArchive(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyServiceRequest(w, r) {
		return
	}
	flags := map[string]FlagValue{}
	flags["repo"] = r.PathValue("repository")
	f.executeTyped(w, r, "repository-archive", "repositories.manage", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbArchiveRepo}, flags)
}

type writerCommitRequest struct {
	CommandID string                `json:"commandId"`
	ChangeSet objectmodel.ChangeSet `json:"changeSet"`
}

func (f *httpFacade) writerCommit(w http.ResponseWriter, r *http.Request) {
	var request writerCommitRequest
	// Scale ChangeSets are whole-repository imports; the commit route carries
	// its own cap (KC_MAX_COMMIT_REQUEST_BYTES, default 512 MiB) instead of
	// the per-route 8 MiB service default.
	if !decodeServiceRequestBody(w, r, &request, maxCommitRequestBytes()) {
		return
	}
	repository := r.PathValue("repository")
	if request.ChangeSet.TargetRepository != "" && string(request.ChangeSet.TargetRepository) != repository {
		invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "repository path and ChangeSet target differ")))
		return
	}
	request.ChangeSet.TargetRepository = kernel.RepositoryID(repository)
	payload, _ := json.Marshal(request.ChangeSet)
	f.executeTyped(w, r, "writer-commit", "writer.commit", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbCommit}, map[string]FlagValue{"repo": repository, "command-id": request.CommandID, "payload": string(payload)})
}

func (f *httpFacade) writerHead(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "writer-head", "writer.preview", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbWriterHead},
		map[string]FlagValue{"repo": r.PathValue("repository"), "ref": r.URL.Query().Get("ref")})
}

func (f *httpFacade) writerReceipt(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "writer-receipt", "writer.receipt.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbReceipt}, map[string]FlagValue{"command-id": r.PathValue("command")})
}

// writerBranchRequest opens a work branch through Writer. The new branch is
// the authorization target; from/commit only select its starting commit.
type writerBranchRequest struct {
	Name   string `json:"name"`
	From   string `json:"from,omitempty"`
	Commit string `json:"commit,omitempty"`
}

func (f *httpFacade) writerBranch(w http.ResponseWriter, r *http.Request) {
	var request writerBranchRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.executeTyped(w, r, "writer-branch", "writer.branch", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbWriterBranch},
		compactFlags(map[string]FlagValue{"repo": r.PathValue("repository"), "name": request.Name, "from": request.From, "commit": request.Commit}))
}

// writerMergeRequest publishes one reviewed source commit onto the target ref.
// commit pins the reviewed source; expected pins the target the review saw.
type writerMergeRequest struct {
	CommandID string `json:"commandId"`
	From      string `json:"from"`
	Into      string `json:"into,omitempty"`
	Commit    string `json:"commit,omitempty"`
	Expected  string `json:"expected,omitempty"`
}

func (f *httpFacade) writerMerge(w http.ResponseWriter, r *http.Request) {
	var request writerMergeRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.executeTyped(w, r, "writer-merge", "writer.merge", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbWriterMerge},
		compactFlags(map[string]FlagValue{"repo": r.PathValue("repository"), "command-id": request.CommandID, "from": request.From, "into": request.Into, "commit": request.Commit, "expected": request.Expected}))
}

type projectionRequest struct {
	Dataset  string `json:"dataset"`
	Revision int    `json:"revision,omitempty"`
}

type accessSpecDescribeRequest struct {
	Dataset    string          `json:"dataset"`
	Revision   int             `json:"revision,omitempty"`
	Repository string          `json:"repository,omitempty"`
	Pin        json.RawMessage `json:"pin,omitempty"`
}
type policyBindingRequest struct {
	On         string `json:"on"`
	Phase      string `json:"phase,omitempty"`
	Repository string `json:"repository,omitempty"`
	Run        string `json:"run,omitempty"`
	URL        string `json:"url,omitempty"`
}
type auditQueryRequest struct {
	RequestID    string `json:"requestId,omitempty"`
	Principal    string `json:"principal,omitempty"`
	OnBehalfOf   string `json:"onBehalfOf,omitempty"`
	Action       string `json:"action,omitempty"`
	TraceID      string `json:"traceId,omitempty"`
	Repository   string `json:"repository,omitempty"`
	Object       string `json:"object,omitempty"`
	Since        string `json:"since,omitempty"`
	Until        string `json:"until,omitempty"`
	Continuation string `json:"continuation,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	EvidenceID   string `json:"evidenceId,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	Outcome      string `json:"outcome,omitempty"`
	Operator     string `json:"operator,omitempty"`
}

func refineAuditFlags(q auditQueryRequest) map[string]FlagValue {
	flags := auditFlags(q)
	flags["_refine-evidence-id"] = q.EvidenceID
	flags["_provider"] = q.Provider
	flags["_model"] = q.Model
	flags["_outcome"] = q.Outcome
	return flags
}

func retrievalAuditFlags(q auditQueryRequest) map[string]FlagValue {
	flags := auditFlags(q)
	flags["_retrieval-evidence-id"] = q.EvidenceID
	flags["_provider"] = q.Provider
	flags["_operator"] = q.Operator
	flags["_outcome"] = q.Outcome
	return flags
}

func projectionFlags(request projectionRequest) map[string]FlagValue {
	return withRevision(compactFlags(map[string]FlagValue{"dataset": request.Dataset}), request.Revision)
}

func (f *httpFacade) observeStores(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "operations-stores", "inventory.read", inv.Command{Stage: invoke.StageHome, Run: inv.VerbObserveStores}, map[string]FlagValue{})
}

func (f *httpFacade) projectionDescribe(w http.ResponseWriter, r *http.Request) {
	var q projectionRequest
	if decodeServiceRequest(w, r, &q) {
		flags := projectionFlags(q)
		if err := inv.RequireDatasetDiscoveryScope(flags); err != nil {
			invoke.WriteInvoke(w, inv.ErrorResult(err))
			return
		}
		f.executeTyped(w, r, "operations-projection-describe", "file.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbDescribeIndex}, flags)
	}
}
func (f *httpFacade) accessSpecDescribe(w http.ResponseWriter, r *http.Request) {
	var q accessSpecDescribeRequest
	if decodeServiceRequest(w, r, &q) {
		flags := withRevision(objectCoordinateFlags(q.Dataset, q.Repository, "", "", q.Pin), q.Revision)
		if err := inv.RequireDatasetDiscoveryScope(flags); err != nil {
			invoke.WriteInvoke(w, inv.ErrorResult(err))
			return
		}
		f.executeTyped(w, r, "operations-access-spec-describe", "file.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbDescribeAccess}, flags)
	}
}

func policyFlags(request policyBindingRequest) map[string]FlagValue {
	return compactFlags(map[string]FlagValue{"on": request.On, "phase": request.Phase, "repo": request.Repository, "run": request.Run, "url": request.URL})
}
func (f *httpFacade) hookAdd(w http.ResponseWriter, r *http.Request) {
	var q policyBindingRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "operations-hook-add", "operations.hooks.manage", inv.Command{Stage: invoke.StageHome, Run: inv.VerbHookAdd}, policyFlags(q))
	}
}
func (f *httpFacade) hookList(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "operations-hook-list", "operations.hooks.read", inv.Command{Stage: invoke.StageHome, Run: inv.VerbHookLs}, compactFlags(map[string]FlagValue{"on": r.URL.Query().Get("on"), "repo": r.URL.Query().Get("repository")}))
}
func (f *httpFacade) hookRemove(w http.ResponseWriter, r *http.Request) {
	if decodeEmptyServiceRequest(w, r) {
		f.executeTyped(w, r, "operations-hook-remove", "operations.hooks.manage", inv.Command{Stage: invoke.StageHome, Run: inv.VerbHookRm}, map[string]FlagValue{"id": r.PathValue("hook")})
	}
}
func auditFlags(q auditQueryRequest) map[string]FlagValue {
	// The HTTP request has its own trace-id. Keep the historical trace selected
	// by the query separate so transport tracing cannot silently change audit
	// results after addHTTPTraceFlags runs.
	flags := compactFlags(map[string]FlagValue{
		"filter-request-id": q.RequestID,
		"filter-principal":  q.Principal, "filter-on-behalf-of": q.OnBehalfOf, "action": q.Action,
		"repo": q.Repository, "object": q.Object, "since": q.Since, "until": q.Until,
		"continuation": q.Continuation, "evidence-id": q.EvidenceID,
	})
	// Presence is significant: an empty selector means "all traces" for the
	// typed query, rather than falling back to this HTTP request's trace.
	flags["_filter-trace-id"] = q.TraceID
	if q.Limit > 0 {
		flags["limit"] = q.Limit
	}
	return flags
}
func (f *httpFacade) accessLog(w http.ResponseWriter, r *http.Request) {
	var q auditQueryRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "operations-audit-access", "audit.read", inv.Command{Stage: invoke.StageHome, Run: inv.VerbAccessLog}, auditFlags(q))
	}
}
func (f *httpFacade) hitmap(w http.ResponseWriter, r *http.Request) {
	var q auditQueryRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "operations-audit-hitmap", "audit.read", inv.Command{Stage: invoke.StageHome, Run: inv.VerbHitmap}, auditFlags(q))
	}
}
func (f *httpFacade) refineLog(w http.ResponseWriter, r *http.Request) {
	var q auditQueryRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "refine-log", "audit.read", inv.Command{Stage: invoke.StageHome, Run: inv.VerbRefineLog}, refineAuditFlags(q))
	}
}
func (f *httpFacade) retrievalLog(w http.ResponseWriter, r *http.Request) {
	var q auditQueryRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "retrieval-log", "audit.read", inv.Command{Stage: invoke.StageHome, Run: inv.VerbRetrievalLog}, retrievalAuditFlags(q))
	}
}
func (f *httpFacade) retrievalTraining(w http.ResponseWriter, r *http.Request) {
	var q auditQueryRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "retrieval-training", "audit.read", inv.Command{Stage: invoke.StageHome, Run: inv.VerbRetrievalTrainingSamples}, retrievalAuditFlags(q))
	}
}
func (f *httpFacade) rerankTraining(w http.ResponseWriter, r *http.Request) {
	var q auditQueryRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "rerank-training", "audit.read", inv.Command{Stage: invoke.StageHome, Run: inv.VerbRerankTrainingSamples}, refineAuditFlags(q))
	}
}
func (f *httpFacade) traceGet(w http.ResponseWriter, r *http.Request) {
	var q struct {
		TraceID string `json:"traceId"`
	}
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "operations-audit-trace", "audit.read", inv.Command{Stage: invoke.StageHome, Run: inv.VerbTrace}, map[string]FlagValue{"_filter-trace-id": q.TraceID})
	}
}
func (f *httpFacade) feedbackRecord(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Dataset             string                          `json:"dataset"`
		TraceID             string                          `json:"traceId"`
		Outcome             string                          `json:"outcome"`
		Message             string                          `json:"message,omitempty"`
		RetrievalEvidenceID string                          `json:"retrievalEvidenceId,omitempty"`
		RefineEvidenceID    string                          `json:"refineEvidenceId,omitempty"`
		Answer              string                          `json:"answer,omitempty"`
		SelectedRefs        []objectmodel.ObjectRef         `json:"selectedRefs,omitempty"`
		IdealGroups         []observability.RefineRankGroup `json:"idealGroups,omitempty"`
	}
	if decodeServiceRequest(w, r, &q) {
		flags := compactFlags(map[string]FlagValue{
			"dataset": q.Dataset, "outcome": q.Outcome, "message": q.Message,
			"_retrieval-evidence-id": q.RetrievalEvidenceID,
			"_refine-evidence-id":    q.RefineEvidenceID, "_feedback-answer": q.Answer,
		})
		// A feedback submission is a new traced request. Keep its transport
		// trace separate from the earlier Agent trace being labelled.
		flags["_feedback-target-trace-id"] = q.TraceID
		flags["_feedback-selected"] = append([]objectmodel.ObjectRef(nil), q.SelectedRefs...)
		flags["_feedback-ideal"] = append([]observability.RefineRankGroup(nil), q.IdealGroups...)
		f.executeTyped(w, r, "operations-feedback-record", "feedback.write", inv.Command{Stage: invoke.StageHome, Run: inv.VerbRecordFeedback}, flags)
	}
}

func decodeEmptyServiceRequest(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil || r.ContentLength == 0 {
		return true
	}
	var empty struct{}
	return decodeServiceRequest(w, r, &empty)
}
