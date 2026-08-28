package cli

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"kc/catalog"
	"kc/kernel"
	"kc/knowledge"
)

// registerManagementRoutes owns the non-consumer service namespaces. The
// request DTOs below are resource-specific: no endpoint accepts a CLI verb or
// an arbitrary flag map.
func (f *httpFacade) registerManagementRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /catalog/v1/catalogs", f.catalogList)
	mux.HandleFunc("GET /catalog/v1/catalogs/{catalog}", f.catalogShow)
	mux.HandleFunc("GET /catalog/v1/catalogs/{catalog}/audit", f.catalogAudit)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/archive", f.catalogArchive)
	mux.HandleFunc("GET /catalog/v1/catalogs/{catalog}/repositories", f.catalogRepositories)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/repositories", f.catalogRepositoryRegister)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/repositories/{repository}/archive", f.catalogRepositoryArchive)
	mux.HandleFunc("GET /catalog/v1/catalogs/{catalog}/workspaces", f.catalogWorkspaces)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/workspaces", f.catalogWorkspaceDefine)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/workspaces/resolve", f.catalogWorkspaceResolveDefinition)
	mux.HandleFunc("GET /catalog/v1/catalogs/{catalog}/workspaces/{workspace}", f.catalogWorkspaceShow)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/workspaces/{workspace}/retire", f.catalogWorkspaceRetire)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/workspaces/{workspace}/resolve", f.catalogWorkspaceResolve)
	mux.HandleFunc("POST /catalog/v1/catalogs/{catalog}/workspaces/{workspace}/check", f.catalogWorkspaceCheck)

	mux.HandleFunc("POST /writer/v1/repositories/{repository}/commits", f.writerCommit)
	mux.HandleFunc("POST /writer/v1/repositories/{repository}/proposals", f.writerProposal)
	mux.HandleFunc("GET /writer/v1/receipts/{command}", f.writerReceipt)

	mux.HandleFunc("POST /governance/v1/proposals", f.governanceProposal)
	mux.HandleFunc("POST /governance/v1/previews", f.governancePreview)
	mux.HandleFunc("POST /governance/v1/previews:validate", f.governanceValidate)
	mux.HandleFunc("POST /governance/v1/validations", f.governanceRecordValidation)
	mux.HandleFunc("POST /governance/v1/proposals:merge", f.governanceMerge)

	mux.HandleFunc("GET /admin/v1/grants", f.adminGrantList)
	mux.HandleFunc("POST /admin/v1/grants", f.adminGrantAdd)
	mux.HandleFunc("POST /admin/v1/grants/{grant}/remove", f.adminGrantRemove)

	mux.HandleFunc("POST /operations/v1/projections:describe", f.projectionDescribe)
	mux.HandleFunc("POST /operations/v1/access-specs:describe", f.accessSpecDescribe)
	mux.HandleFunc("GET /operations/v1/hooks", f.hookList)
	mux.HandleFunc("POST /operations/v1/hooks", f.hookAdd)
	mux.HandleFunc("POST /operations/v1/hooks/{hook}/remove", f.hookRemove)
	mux.HandleFunc("GET /operations/v1/gates", f.gateList)
	mux.HandleFunc("POST /operations/v1/gates", f.gateAdd)
	mux.HandleFunc("POST /operations/v1/gates/{gate}/remove", f.gateRemove)
	mux.HandleFunc("POST /operations/v1/access-log:query", f.accessLog)
	mux.HandleFunc("POST /operations/v1/traces:get", f.traceGet)
	mux.HandleFunc("POST /operations/v1/hitmap:query", f.hitmap)
	mux.HandleFunc("POST /operations/v1/feedback", f.feedbackRecord)
}

func catalogListOperation(cx *invocation) (any, error) {
	file, err := ReadHome(cx.Home)
	if err != nil {
		return nil, err
	}
	return map[string]any{"catalogs": file.Catalogs}, nil
}

func (f *httpFacade) catalogList(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "catalog-list", "catalog.read", command{stage: stageHome, run: catalogListOperation}, map[string]FlagValue{})
}

type catalogWorkspaceRequest struct {
	Workspace string                    `json:"workspace"`
	Revision  int                       `json:"revision"`
	Sources   []catalog.WorkspaceSource `json:"sources"`
}

type catalogRepositoryRequest struct {
	Repository string `json:"repository"`
}

type catalogResolveRequest struct {
	Pin json.RawMessage `json:"pin,omitempty"`
}

type catalogAuditRequest struct {
	Limit int `json:"limit,omitempty"`
}

func catalogFlags(r *http.Request) map[string]FlagValue {
	return map[string]FlagValue{"catalog": r.PathValue("catalog")}
}

func (f *httpFacade) catalogShow(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "read", "catalog.read", command{stage: stageGoverned, run: verbRead}, catalogFlags(r))
}

func (f *httpFacade) catalogAudit(w http.ResponseWriter, r *http.Request) {
	flags := catalogFlags(r)
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if _, err := strconv.Atoi(raw); err != nil {
			writeJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "limit must be an integer")))
			return
		}
		flags["limit"] = raw
	}
	f.executeTyped(w, r, "audit", "catalog.audit.read", command{stage: stageHome, run: verbAudit}, flags)
}

func (f *httpFacade) catalogArchive(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyServiceRequest(w, r) {
		return
	}
	f.executeTyped(w, r, "archive-catalog", "catalog.manage", command{stage: stageGoverned, run: verbArchiveCatalog}, catalogFlags(r))
}

func (f *httpFacade) catalogRepositories(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "catalog-repositories", "catalog.read", command{stage: stageGoverned, run: readCatalogStatePart("repositories")}, catalogFlags(r))
}

func (f *httpFacade) catalogWorkspaces(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "catalog-workspaces", "catalog.read", command{stage: stageGoverned, run: readCatalogStatePart("workspaces")}, catalogFlags(r))
}

func (f *httpFacade) catalogWorkspaceShow(w http.ResponseWriter, r *http.Request) {
	flags := catalogFlags(r)
	flags["workspace"] = r.PathValue("workspace")
	f.executeTyped(w, r, "catalog-workspace", "catalog.read", command{stage: stageGoverned, run: readCatalogStatePart("workspace")}, flags)
}

func (f *httpFacade) catalogRepositoryRegister(w http.ResponseWriter, r *http.Request) {
	var request catalogRepositoryRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := catalogFlags(r)
	flags["repo"] = request.Repository
	f.executeTyped(w, r, "register", "catalog.repositories.manage", command{stage: stageGoverned, run: verbRegister}, flags)
}

func (f *httpFacade) catalogRepositoryArchive(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyServiceRequest(w, r) {
		return
	}
	flags := catalogFlags(r)
	flags["repo"] = r.PathValue("repository")
	f.executeTyped(w, r, "archive-repo", "catalog.repositories.manage", command{stage: stageGoverned, run: verbArchiveRepo}, flags)
}

func (f *httpFacade) catalogWorkspaceDefine(w http.ResponseWriter, r *http.Request) {
	var request catalogWorkspaceRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	payload, _ := json.Marshal(request.Sources)
	flags := catalogFlags(r)
	flags["workspace"] = request.Workspace
	flags["revision"] = request.Revision
	flags["payload"] = string(payload)
	f.executeTyped(w, r, "define-workspace", "workspace.manage", command{stage: stageGoverned, run: verbDefineWorkspace}, flags)
}

func (f *httpFacade) catalogWorkspaceRetire(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyServiceRequest(w, r) {
		return
	}
	flags := catalogFlags(r)
	flags["workspace"] = r.PathValue("workspace")
	f.executeTyped(w, r, "retire-workspace", "workspace.manage", command{stage: stageGoverned, run: verbRetireWorkspace}, flags)
}

func (f *httpFacade) catalogWorkspaceResolve(w http.ResponseWriter, r *http.Request) {
	var request catalogResolveRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := catalogFlags(r)
	flags["workspace"] = r.PathValue("workspace")
	if len(request.Pin) > 0 {
		flags["pin"] = string(request.Pin)
	}
	f.executeTyped(w, r, "resolve", "workspace.resolve", command{stage: stageGoverned, run: verbResolve}, flags)
}

func (f *httpFacade) catalogWorkspaceResolveDefinition(w http.ResponseWriter, r *http.Request) {
	var request catalogWorkspaceRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := catalogFlags(r)
	flags["workspace"] = request.Workspace
	op := command{stage: stageGoverned, run: func(cx *invocation) (any, error) {
		cat, err := pickCatalog(cx.WS, cx.Flags)
		if err != nil {
			return nil, err
		}
		resolved, err := cat.ResolveDefinition(catalog.WorkspaceDefinition{WorkspaceID: request.Workspace, Revision: request.Revision, Sources: request.Sources})
		if err != nil {
			return nil, err
		}
		if err := requireCompleteWorkspaceRead(cx.Home, cx.Flags, workspacePin(resolved), ""); err != nil {
			return nil, err
		}
		return resolved, nil
	}}
	f.executeTyped(w, r, "resolve-definition", "workspace.resolve", op, flags)
}

func (f *httpFacade) catalogWorkspaceCheck(w http.ResponseWriter, r *http.Request) {
	var request catalogResolveRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := catalogFlags(r)
	flags["workspace"] = r.PathValue("workspace")
	if len(request.Pin) > 0 {
		flags["pin"] = string(request.Pin)
	}
	f.executeTyped(w, r, "check-workspace", "workspace.resolve", command{stage: stageGoverned, run: verbCheckWorkspace}, flags)
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
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	repository := r.PathValue("repository")
	if request.ChangeSet.TargetRepository != "" && string(request.ChangeSet.TargetRepository) != repository {
		writeJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "repository path and ChangeSet target differ")))
		return
	}
	request.ChangeSet.TargetRepository = kernel.RepositoryID(repository)
	payload, _ := json.Marshal(request.ChangeSet)
	f.executeTyped(w, r, "commit", "writer.commit", command{stage: stageGoverned, run: verbCommit}, map[string]FlagValue{"repo": repository, "command-id": request.CommandID, "payload": string(payload)})
}

func proposalFlags(request proposalRequest) map[string]FlagValue {
	payload, _ := json.Marshal(request.Operations)
	return compactFlags(map[string]FlagValue{"catalog": request.Catalog, "repo": request.Repository, "proposal-id": request.ProposalID, "candidate": request.CandidateRef, "target": request.TargetRef, "base": request.BaseCommit, "message": request.Message, "payload": string(payload)})
}

func (f *httpFacade) writerProposal(w http.ResponseWriter, r *http.Request) {
	var request proposalRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	request.Repository = r.PathValue("repository")
	f.executeTyped(w, r, "propose", "governance.proposal.create", command{stage: stageGoverned, run: verbPropose}, proposalFlags(request))
}

func (f *httpFacade) governanceProposal(w http.ResponseWriter, r *http.Request) {
	var request proposalRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.executeTyped(w, r, "propose", "governance.proposal.create", command{stage: stageGoverned, run: verbPropose}, proposalFlags(request))
}

func (f *httpFacade) writerReceipt(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "receipt", "writer.receipt.read", command{stage: stageGoverned, run: verbReceipt}, map[string]FlagValue{"command-id": r.PathValue("command")})
}

type governanceReferenceRequest struct{ Catalog, Workspace, Proposal, Preview, Validation string }
type governanceValidationRequest struct {
	Catalog string `json:"catalog,omitempty"`
	Preview string `json:"preview"`
	Suite   string `json:"suite"`
	Outcome string `json:"outcome"`
}

func (f *httpFacade) governancePreview(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Catalog   string `json:"catalog,omitempty"`
		Workspace string `json:"workspace"`
		Proposal  string `json:"proposal"`
	}
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.executeTyped(w, r, "preview", "governance.preview.create", command{stage: stageGoverned, run: verbPreview}, compactFlags(map[string]FlagValue{"catalog": request.Catalog, "workspace": request.Workspace, "proposal": request.Proposal}))
}

func (f *httpFacade) governanceValidate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Catalog string `json:"catalog,omitempty"`
		Preview string `json:"preview"`
	}
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.executeTyped(w, r, "validate", "governance.validate", command{stage: stageGoverned, run: verbValidate}, compactFlags(map[string]FlagValue{"catalog": request.Catalog, "preview": request.Preview}))
}

func (f *httpFacade) governanceRecordValidation(w http.ResponseWriter, r *http.Request) {
	var request governanceValidationRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.executeTyped(w, r, "record-validation", "governance.validation.record", command{stage: stageGoverned, run: verbRecordValidation}, compactFlags(map[string]FlagValue{"catalog": request.Catalog, "preview": request.Preview, "suite": request.Suite, "outcome": request.Outcome}))
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
	f.executeTyped(w, r, "merge", "governance.merge", command{stage: stageGoverned, run: verbMerge}, compactFlags(map[string]FlagValue{"catalog": request.Catalog, "proposal": request.Proposal, "preview": request.Preview, "validation": request.Validation}))
}

type grantRequest struct {
	Principal  string   `json:"principal"`
	Actions    []string `json:"actions"`
	Repository string   `json:"repository,omitempty"`
	Catalog    string   `json:"catalog,omitempty"`
	Ref        string   `json:"ref,omitempty"`
	Object     string   `json:"object,omitempty"`
	Aspect     string   `json:"aspect,omitempty"`
	Workspace  string   `json:"workspace,omitempty"`
}

func (f *httpFacade) adminGrantAdd(w http.ResponseWriter, r *http.Request) {
	var request grantRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.executeTyped(w, r, "allow", "admin.grants.manage", command{stage: stageHome, run: verbAllow}, compactFlags(map[string]FlagValue{"principal": request.Principal, "action": strings.Join(request.Actions, ","), "repo": request.Repository, "catalog": request.Catalog, "ref": request.Ref, "object": request.Object, "aspect": request.Aspect, "workspace": request.Workspace}))
}

func (f *httpFacade) adminGrantList(w http.ResponseWriter, r *http.Request) {
	flags := compactFlags(map[string]FlagValue{"principal": r.URL.Query().Get("principal"), "action": r.URL.Query().Get("action"), "repo": r.URL.Query().Get("repository"), "catalog": r.URL.Query().Get("catalog"), "workspace": r.URL.Query().Get("workspace")})
	f.executeTyped(w, r, "allowed", "admin.grants.read", command{stage: stageHome, run: verbAllowed}, flags)
}

func (f *httpFacade) adminGrantRemove(w http.ResponseWriter, r *http.Request) {
	if !decodeEmptyServiceRequest(w, r) {
		return
	}
	f.executeTyped(w, r, "revoke", "admin.grants.manage", command{stage: stageHome, run: verbRevoke}, map[string]FlagValue{"id": r.PathValue("grant")})
}

type projectionRequest struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit,omitempty"`
	Ref        string `json:"ref,omitempty"`
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
	Principal  string `json:"principal,omitempty"`
	OnBehalfOf string `json:"onBehalfOf,omitempty"`
	Action     string `json:"action,omitempty"`
	TraceID    string `json:"traceId,omitempty"`
	Repository string `json:"repository,omitempty"`
	Object     string `json:"object,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

func projectionFlags(request projectionRequest) map[string]FlagValue {
	return compactFlags(map[string]FlagValue{"repo": request.Repository, "commit": request.Commit, "ref": request.Ref})
}
func (f *httpFacade) projectionDescribe(w http.ResponseWriter, r *http.Request) {
	var q projectionRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "describe-index", "projection.read", command{stage: stageGoverned, run: verbDescribeIndex}, projectionFlags(q))
	}
}
func (f *httpFacade) accessSpecDescribe(w http.ResponseWriter, r *http.Request) {
	var q projectionRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "describe-access", "knowledge.access.describe", command{stage: stageGoverned, run: verbDescribeAccess}, projectionFlags(q))
	}
}

func policyFlags(request policyBindingRequest) map[string]FlagValue {
	return compactFlags(map[string]FlagValue{"on": request.On, "phase": request.Phase, "repo": request.Repository, "catalog": request.Catalog, "run": request.Run, "url": request.URL, "require": strings.Join(request.Require, ",")})
}
func (f *httpFacade) hookAdd(w http.ResponseWriter, r *http.Request) {
	var q policyBindingRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "hook-add", "operations.hooks.manage", command{stage: stageHome, run: verbHookAdd}, policyFlags(q))
	}
}
func (f *httpFacade) hookList(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "hook-ls", "operations.hooks.read", command{stage: stageHome, run: verbHookLs}, compactFlags(map[string]FlagValue{"on": r.URL.Query().Get("on"), "repo": r.URL.Query().Get("repository"), "catalog": r.URL.Query().Get("catalog")}))
}
func (f *httpFacade) hookRemove(w http.ResponseWriter, r *http.Request) {
	if decodeEmptyServiceRequest(w, r) {
		f.executeTyped(w, r, "hook-rm", "operations.hooks.manage", command{stage: stageHome, run: verbHookRm}, map[string]FlagValue{"id": r.PathValue("hook")})
	}
}
func (f *httpFacade) gateAdd(w http.ResponseWriter, r *http.Request) {
	var q policyBindingRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "gate-add", "operations.gates.manage", command{stage: stageHome, run: verbGateAdd}, policyFlags(q))
	}
}
func (f *httpFacade) gateList(w http.ResponseWriter, r *http.Request) {
	f.executeTyped(w, r, "gate-ls", "operations.gates.read", command{stage: stageHome, run: verbGateLs}, compactFlags(map[string]FlagValue{"on": r.URL.Query().Get("on"), "repo": r.URL.Query().Get("repository"), "catalog": r.URL.Query().Get("catalog")}))
}
func (f *httpFacade) gateRemove(w http.ResponseWriter, r *http.Request) {
	if decodeEmptyServiceRequest(w, r) {
		f.executeTyped(w, r, "gate-rm", "operations.gates.manage", command{stage: stageHome, run: verbGateRm}, map[string]FlagValue{"id": r.PathValue("gate")})
	}
}

func auditFlags(q auditQueryRequest) map[string]FlagValue {
	flags := compactFlags(map[string]FlagValue{"filter-principal": q.Principal, "filter-on-behalf-of": q.OnBehalfOf, "action": q.Action, "trace-id": q.TraceID, "repo": q.Repository, "object": q.Object})
	if q.Limit > 0 {
		flags["limit"] = q.Limit
	}
	return flags
}
func (f *httpFacade) accessLog(w http.ResponseWriter, r *http.Request) {
	var q auditQueryRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "access-log", "audit.read", command{stage: stageHome, run: verbAccessLog}, auditFlags(q))
	}
}
func (f *httpFacade) hitmap(w http.ResponseWriter, r *http.Request) {
	var q auditQueryRequest
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "hitmap", "audit.read", command{stage: stageHome, run: verbHitmap}, auditFlags(q))
	}
}
func (f *httpFacade) traceGet(w http.ResponseWriter, r *http.Request) {
	var q struct {
		TraceID string `json:"traceId"`
	}
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "trace", "audit.read", command{stage: stageHome, run: verbTrace}, map[string]FlagValue{"trace-id": q.TraceID})
	}
}
func (f *httpFacade) feedbackRecord(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Workspace string `json:"workspace"`
		TraceID   string `json:"traceId"`
		Outcome   string `json:"outcome"`
		Message   string `json:"message,omitempty"`
	}
	if decodeServiceRequest(w, r, &q) {
		f.executeTyped(w, r, "record-feedback", "feedback.write", command{stage: stageHome, run: verbRecordFeedback}, compactFlags(map[string]FlagValue{"workspace": q.Workspace, "trace-id": q.TraceID, "outcome": q.Outcome, "message": q.Message}))
	}
}

func decodeEmptyServiceRequest(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil || r.ContentLength == 0 {
		return true
	}
	var empty struct{}
	return decodeServiceRequest(w, r, &empty)
}
