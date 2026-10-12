package server

import (
	inv "kc/invocation"
	"kc/invoke"

	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	kcclient "kc/client"
	"kc/kernel"
	"kc/knowledge"
	"kc/observability"
)

// registerManagementRoutes owns the non-consumer service namespaces. The
// request DTOs below are resource-specific: no endpoint accepts a CLI verb or
// an arbitrary flag map.
func (f *httpFacade) registerManagementRoutes(mux *http.ServeMux) {
	f.registerRepositoryRoutes(mux)
	f.snapshotStoreService().Register(mux)
	mux.HandleFunc("GET /catalog/v1/catalogs/{catalog}", f.catalogShow)
	mux.HandleFunc("GET /catalog/v1/catalogs/{catalog}/audit", f.catalogAudit)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/archive", f.catalogArchive)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/repositories", f.catalogRepositoryAttach)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/repositories:create", f.catalogRepositoryCreate)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/repositories/{repository}/archive", f.catalogRepositoryArchive)
	mux.HandleFunc("DELETE /catalog/v1/catalogs/{catalog}/repositories/{repository}", f.catalogRepositoryDetach)

	mux.HandleFunc("POST /writer/v1/repositories/{repository}/commits", f.writerCommit)
	mux.HandleFunc("GET /writer/v1/repositories/{repository}/head", f.writerHead)
	mux.HandleFunc("GET /writer/v1/receipts/{command}", f.writerReceipt)

	mux.HandleFunc("POST /governance/v1/proposals", f.governanceProposal)
	mux.HandleFunc("POST /governance/v1/previews", f.governancePreview)
	mux.HandleFunc("POST /governance/v1/previews:validate", f.governanceValidate)
	mux.HandleFunc("POST /governance/v1/validations", f.governanceRecordValidation)
	mux.HandleFunc("POST /governance/v1/proposals:merge", f.governanceMerge)

	mux.HandleFunc("GET /operations/v1/stores", f.observeStores)
	mux.HandleFunc("POST /operations/v1/projections:describe", f.projectionDescribe)
	mux.HandleFunc("POST /operations/v1/access-specs:describe", f.accessSpecDescribe)
	mux.HandleFunc("GET /operations/v1/hooks", f.hookList)
	mux.HandleFunc("POST /operations/v1/hooks", f.hookAdd)
	mux.HandleFunc("DELETE /operations/v1/hooks/{hook}", f.hookRemove)
	mux.HandleFunc("GET /operations/v1/gates", f.gateList)
	mux.HandleFunc("POST /operations/v1/gates", f.gateAdd)
	mux.HandleFunc("DELETE /operations/v1/gates/{gate}", f.gateRemove)
	mux.HandleFunc("POST /operations/v1/access-log:query", f.accessLog)
	mux.HandleFunc("POST /operations/v1/traces:query", f.traceGet)
	mux.HandleFunc("POST /operations/v1/hitmap:query", f.hitmap)
	mux.HandleFunc("POST /operations/v1/retrieval-log:query", f.retrievalLog)
	mux.HandleFunc("POST /operations/v1/retrieval-training:query", f.retrievalTraining)
	mux.HandleFunc("POST /operations/v1/refine-log:query", f.refineLog)
	mux.HandleFunc("POST /operations/v1/rerank-training:query", f.rerankTraining)
	mux.HandleFunc("POST /operations/v1/feedback", f.feedbackRecord)
}

type catalogRepositoryRequest struct {
	Repository string `json:"repository"`
}

type catalogRepositoryCreateRequest struct {
	Repository string `json:"repository"`
	CommandID  string `json:"commandId"`
}

func catalogFlags(r *http.Request) map[string]FlagValue {
	return map[string]FlagValue{"catalog": r.PathValue("catalog")}
}

func (f *httpFacade) catalogShow(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "catalog-show", "catalog.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.ReadCatalogState}, catalogFlags(r))
}

func (f *httpFacade) catalogAudit(w http.ResponseWriter, r *http.Request) {
	flags := catalogFlags(r)
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if _, err := strconv.Atoi(raw); err != nil {
			invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "limit must be an integer")))
			return
		}
		flags["limit"] = raw
	}
	f.executeTyped(w, r, "catalog-audit", "catalog.audit.read", inv.Command{Stage: invoke.StageHome, Run: inv.VerbAudit}, flags)
}

func (f *httpFacade) catalogArchive(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyServiceRequest(w, r) {
		return
	}
	f.executeTyped(w, r, "catalog-archive", "catalog.manage", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbArchiveCatalog}, catalogFlags(r))
}

func (f *httpFacade) catalogRepositoryAttach(w http.ResponseWriter, r *http.Request) {
	var request catalogRepositoryRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := catalogFlags(r)
	flags["repo"] = request.Repository
	f.executeTyped(w, r, "catalog-repo-attach", "catalog.repositories.manage", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbRegister}, flags)
}

func (f *httpFacade) catalogRepositoryCreate(w http.ResponseWriter, r *http.Request) {
	var request catalogRepositoryCreateRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := catalogFlags(r)
	flags["repo"], flags["command-id"] = request.Repository, request.CommandID
	if err := inv.ValidateManagedRepositoryCoordinates(flags); err != nil {
		invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(err))
		return
	}
	f.executeTyped(w, r, "catalog-repo-create", "catalog.repositories.create", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbCreateManagedRepository}, flags)
}

func (f *httpFacade) catalogRepositoryArchive(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyServiceRequest(w, r) {
		return
	}
	flags := catalogFlags(r)
	flags["repo"] = r.PathValue("repository")
	f.executeTyped(w, r, "catalog-repo-archive", "catalog.repositories.manage", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbArchiveRepo}, flags)
}

func (f *httpFacade) catalogRepositoryDetach(w http.ResponseWriter, r *http.Request) {
	flags := catalogFlags(r)
	flags["repo"] = r.PathValue("repository")
	f.executeTyped(w, r, "detach", "catalog.repositories.manage", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbDetach}, flags)
}

type writerCommitRequest struct {
	CommandID string              `json:"commandId"`
	ChangeSet knowledge.ChangeSet `json:"changeSet"`
}

type proposalRequest struct {
	Catalog      string                        `json:"catalog,omitempty"`
	Repository   string                        `json:"repository,omitempty"`
	ProposalID   string                        `json:"proposalId"`
	CandidateRef string                        `json:"candidateRef"`
	TargetRef    string                        `json:"targetRef,omitempty"`
	BaseCommit   string                        `json:"baseCommit,omitempty"`
	Operations   []knowledge.Operation         `json:"operations"`
	Message      string                        `json:"message,omitempty"`
	Provenance   *knowledge.ProvenanceEnvelope `json:"provenance,omitempty"`
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

func proposalFlags(request proposalRequest) map[string]FlagValue {
	payload, _ := json.Marshal(request.Operations)
	return compactFlags(map[string]FlagValue{"catalog": request.Catalog, "repo": request.Repository, "proposal-id": request.ProposalID, "candidate": request.CandidateRef, "target": request.TargetRef, "base": request.BaseCommit, "message": request.Message, "payload": string(payload)})
}

func (f *httpFacade) governanceProposal(w http.ResponseWriter, r *http.Request) {
	var request proposalRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.executeTyped(w, r, "governance-proposal-create", "governance.proposal.create", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbPropose}, proposalFlags(request))
}

func (f *httpFacade) writerReceipt(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "writer-receipt", "writer.receipt.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbReceipt}, map[string]FlagValue{"command-id": r.PathValue("command")})
}

type governanceValidationRequest struct {
	Catalog string `json:"catalog,omitempty"`
	Preview string `json:"preview"`
	Suite   string `json:"suite"`
	Outcome string `json:"outcome"`
}

func (f *httpFacade) governancePreview(w http.ResponseWriter, r *http.Request) {
	var request kcclient.PreviewRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.executeTyped(w, r, "governance-preview-create", "governance.preview.create", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbPreview}, compactFlags(map[string]FlagValue{"pin": string(request.Pin), "dataset": request.Dataset, "proposal": request.Proposal}))
}

func (f *httpFacade) governanceValidate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Catalog string `json:"catalog,omitempty"`
		Preview string `json:"preview"`
	}
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.executeTyped(w, r, "governance-preview-validate", "governance.validate", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbValidate}, compactFlags(map[string]FlagValue{"catalog": request.Catalog, "preview": request.Preview}))
}

func (f *httpFacade) governanceRecordValidation(w http.ResponseWriter, r *http.Request) {
	var request governanceValidationRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.executeTyped(w, r, "governance-validation-record", "governance.validation.record", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbRecordValidation}, compactFlags(map[string]FlagValue{"catalog": request.Catalog, "preview": request.Preview, "suite": request.Suite, "outcome": request.Outcome}))
}

func (f *httpFacade) governanceMerge(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Catalog    string `json:"catalog,omitempty"`
		Proposal   string `json:"proposal"`
		Preview    string `json:"preview"`
		Validation string `json:"validation,omitempty"`
	}
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.executeTyped(w, r, "governance-proposal-merge", "governance.merge", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbMerge}, compactFlags(map[string]FlagValue{"catalog": request.Catalog, "proposal": request.Proposal, "preview": request.Preview, "validation": request.Validation}))
}

type projectionRequest struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit,omitempty"`
	Ref        string `json:"ref,omitempty"`
}

type accessSpecDescribeRequest struct {
	Catalog    string          `json:"catalog,omitempty"`
	Dataset    string          `json:"dataset"`
	Repository string          `json:"repository,omitempty"`
	Pin        json.RawMessage `json:"pin,omitempty"`
}
type policyBindingRequest struct {
	On         string   `json:"on"`
	Phase      string   `json:"phase,omitempty"`
	Repository string   `json:"repository,omitempty"`
	Catalog    string   `json:"catalog,omitempty"`
	Run        string   `json:"run,omitempty"`
	URL        string   `json:"url,omitempty"`
	Require    []string `json:"require,omitempty"`
}
type auditQueryRequest struct {
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
	return compactFlags(map[string]FlagValue{"repo": request.Repository, "commit": request.Commit, "ref": request.Ref})
}

func (f *httpFacade) observeStores(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "operations-stores", "catalog.read", inv.Command{Stage: invoke.StageHome, Run: inv.VerbObserveStores}, map[string]FlagValue{})
}

func (f *httpFacade) projectionDescribe(w http.ResponseWriter, r *http.Request) {
	var q projectionRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "operations-projection-describe", "projection.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbDescribeIndex}, projectionFlags(q))
	}
}
func (f *httpFacade) accessSpecDescribe(w http.ResponseWriter, r *http.Request) {
	var q accessSpecDescribeRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "operations-access-spec-describe", "knowledge.access.describe", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbDescribeAccess}, knowledgeCoordinateFlags(q.Catalog, q.Dataset, q.Repository, "", "", q.Pin))
	}
}

func policyFlags(request policyBindingRequest) map[string]FlagValue {
	return compactFlags(map[string]FlagValue{"on": request.On, "phase": request.Phase, "repo": request.Repository, "catalog": request.Catalog, "run": request.Run, "url": request.URL, "require": strings.Join(request.Require, ",")})
}
func (f *httpFacade) hookAdd(w http.ResponseWriter, r *http.Request) {
	var q policyBindingRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "operations-hook-add", "operations.hooks.manage", inv.Command{Stage: invoke.StageHome, Run: inv.VerbHookAdd}, policyFlags(q))
	}
}
func (f *httpFacade) hookList(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "operations-hook-list", "operations.hooks.read", inv.Command{Stage: invoke.StageHome, Run: inv.VerbHookLs}, compactFlags(map[string]FlagValue{"on": r.URL.Query().Get("on"), "repo": r.URL.Query().Get("repository"), "catalog": r.URL.Query().Get("catalog")}))
}
func (f *httpFacade) hookRemove(w http.ResponseWriter, r *http.Request) {
	if decodeEmptyServiceRequest(w, r) {
		f.executeTyped(w, r, "operations-hook-remove", "operations.hooks.manage", inv.Command{Stage: invoke.StageHome, Run: inv.VerbHookRm}, map[string]FlagValue{"id": r.PathValue("hook")})
	}
}
func (f *httpFacade) gateAdd(w http.ResponseWriter, r *http.Request) {
	var q policyBindingRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "operations-gate-add", "operations.gates.manage", inv.Command{Stage: invoke.StageHome, Run: inv.VerbGateAdd}, policyFlags(q))
	}
}
func (f *httpFacade) gateList(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "operations-gate-list", "operations.gates.read", inv.Command{Stage: invoke.StageHome, Run: inv.VerbGateLs}, compactFlags(map[string]FlagValue{"on": r.URL.Query().Get("on"), "repo": r.URL.Query().Get("repository"), "catalog": r.URL.Query().Get("catalog")}))
}
func (f *httpFacade) gateRemove(w http.ResponseWriter, r *http.Request) {
	if decodeEmptyServiceRequest(w, r) {
		f.executeTyped(w, r, "operations-gate-remove", "operations.gates.manage", inv.Command{Stage: invoke.StageHome, Run: inv.VerbGateRm}, map[string]FlagValue{"id": r.PathValue("gate")})
	}
}

func auditFlags(q auditQueryRequest) map[string]FlagValue {
	// The HTTP request has its own trace-id. Keep the historical trace selected
	// by the query separate so transport tracing cannot silently change audit
	// results after addHTTPTraceFlags runs.
	flags := compactFlags(map[string]FlagValue{
		"filter-principal": q.Principal, "filter-on-behalf-of": q.OnBehalfOf, "action": q.Action,
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
		SelectedRefs        []knowledge.KnowledgeRef        `json:"selectedRefs,omitempty"`
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
		flags["_feedback-selected"] = append([]knowledge.KnowledgeRef(nil), q.SelectedRefs...)
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
