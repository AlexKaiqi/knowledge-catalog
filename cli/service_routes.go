package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
)

const maxServiceRequestBytes = 8 << 20

// registerServiceRoutes is an explicit API registry. It does not inspect the
// CLI surface or internal operation table, so adding a CLI command can never
// create an HTTP endpoint accidentally.
func (f *httpFacade) registerServiceRoutes(mux *http.ServeMux) {
	f.registerManagementRoutes(mux)
	mux.HandleFunc("GET /identity/v1/whoami", f.identityWhoAmI)
	mux.HandleFunc("POST /knowledge/v1/objects:read", f.knowledgeRead)
	mux.HandleFunc("POST /knowledge/v1/addresses:read", f.knowledgeRead)
	mux.HandleFunc("POST /knowledge/v1/search", f.knowledgeSearch)
	mux.HandleFunc("POST /knowledge/v1/search:rerank", f.knowledgeSearchRerank)
	mux.HandleFunc("POST /knowledge/v1/rerank", f.knowledgeRerank)
	mux.HandleFunc("POST /knowledge/v1/relations:query", f.knowledgeRelations)
	mux.HandleFunc("POST /knowledge/v1/provenance:get", f.knowledgeProvenance)
	mux.HandleFunc("POST /knowledge/v1/log:get", f.knowledgeLog)
	mux.HandleFunc("POST /knowledge/v1/schemas:get", f.knowledgeSchema)
	mux.HandleFunc("POST /knowledge/v1/schemas:page", f.knowledgeSchemaPage)
	mux.HandleFunc("POST /knowledge/v1/bindings:resolve", f.knowledgeBinding)
	mux.HandleFunc("POST /knowledge/v1/resources:access", f.knowledgeResourceAccess)
	mux.HandleFunc("POST /workspace-files/v1/mounts:list", f.workspaceFileMounts)
	mux.HandleFunc("POST /workspace-files/v1/tree:list", f.workspaceFileDirectory)
	mux.HandleFunc("POST /workspace-files/v1/file:read", f.workspaceFileRead)
	mux.HandleFunc("POST /operations/v1/projections:sync", f.projectionSync)
}

func (f *httpFacade) identityWhoAmI(w http.ResponseWriter, r *http.Request) {
	identity, ok := f.serviceIdentity(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"principal":  identity.Principal,
		"onBehalfOf": identity.OnBehalfOf,
	})
}

type knowledgeReadRequest struct {
	Catalog   string          `json:"catalog,omitempty"`
	Workspace string          `json:"workspace"`
	Pin       json.RawMessage `json:"pin,omitempty"`
	Object    string          `json:"object"`
	Aspect    string          `json:"aspect,omitempty"`
	Member    string          `json:"member,omitempty"`
	Include   []string        `json:"include,omitempty"`
	Exclude   []string        `json:"exclude,omitempty"`
}

type knowledgeSearchRequest struct {
	Catalog      string                  `json:"catalog,omitempty"`
	Workspace    string                  `json:"workspace"`
	Pin          json.RawMessage         `json:"pin,omitempty"`
	Query        string                  `json:"query,omitempty"`
	Match        []string                `json:"match,omitempty"`
	MatchMode    string                  `json:"matchMode,omitempty"`
	Equal        []string                `json:"equal,omitempty"`
	NotEqual     []string                `json:"notEqual,omitempty"`
	In           []string                `json:"in,omitempty"`
	Exists       []string                `json:"exists,omitempty"`
	Missing      []string                `json:"missing,omitempty"`
	Prefix       []string                `json:"prefix,omitempty"`
	GreaterThan  []string                `json:"greaterThan,omitempty"`
	GreaterEqual []string                `json:"greaterEqual,omitempty"`
	LessThan     []string                `json:"lessThan,omitempty"`
	LessEqual    []string                `json:"lessEqual,omitempty"`
	Sort         []string                `json:"sort,omitempty"`
	Limit        int                     `json:"limit,omitempty"`
	Continuation string                  `json:"continuation,omitempty"`
	Expression   *retrieval.SearchExpr   `json:"expression,omitempty"`
	Order        *retrieval.SearchClause `json:"order,omitempty"`
}

type knowledgeRelationsRequest struct {
	Catalog      string          `json:"catalog,omitempty"`
	Workspace    string          `json:"workspace"`
	Pin          json.RawMessage `json:"pin,omitempty"`
	Endpoint     string          `json:"endpoint"`
	RelationType string          `json:"relationType,omitempty"`
	Role         string          `json:"role,omitempty"`
	Direction    string          `json:"direction,omitempty"`
	Limit        int             `json:"limit,omitempty"`
	Continuation string          `json:"continuation,omitempty"`
}

type knowledgeRerankRequest struct {
	Catalog    string                         `json:"catalog,omitempty"`
	Workspace  string                         `json:"workspace"`
	Pin        json.RawMessage                `json:"pin,omitempty"`
	Candidates []knowledge.KnowledgeRef       `json:"candidates"`
	Spec       retrieval.SemanticOperatorSpec `json:"spec"`
}

type knowledgeSearchRerankRequest struct {
	knowledgeSearchRequest
	Spec retrieval.SemanticOperatorSpec `json:"spec"`
}

type knowledgeObjectRequest struct {
	Catalog   string          `json:"catalog,omitempty"`
	Workspace string          `json:"workspace"`
	Pin       json.RawMessage `json:"pin,omitempty"`
	Object    string          `json:"object"`
}

type knowledgeSchemaRequest struct {
	Catalog   string          `json:"catalog,omitempty"`
	Workspace string          `json:"workspace"`
	Pin       json.RawMessage `json:"pin,omitempty"`
	Object    string          `json:"object,omitempty"`
}

type knowledgeSchemaPageRequest struct {
	Repository   string `json:"repository"`
	Commit       string `json:"commit,omitempty"`
	Ref          string `json:"ref,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

type knowledgeBindingRequest struct {
	Catalog   string          `json:"catalog,omitempty"`
	Workspace string          `json:"workspace"`
	Pin       json.RawMessage `json:"pin,omitempty"`
	Object    string          `json:"object"`
	Aspect    string          `json:"aspect"`
	Member    string          `json:"member,omitempty"`
}

type knowledgeResourceAccessRequest struct {
	Catalog   string          `json:"catalog,omitempty"`
	Workspace string          `json:"workspace"`
	Pin       json.RawMessage `json:"pin,omitempty"`
	Object    string          `json:"object"`
	Aspect    string          `json:"aspect,omitempty"`
	Member    string          `json:"member,omitempty"`
	Operation string          `json:"operation,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
}

func (request knowledgeSearchRequest) flags() map[string]FlagValue {
	flags := map[string]FlagValue{
		"catalog": request.Catalog, "workspace": request.Workspace, "query": request.Query,
		"match": request.Match, "match-mode": request.MatchMode, "eq": request.Equal,
		"neq": request.NotEqual, "in": request.In, "exists": request.Exists,
		"missing": request.Missing, "prefix": request.Prefix,
		"gt": request.GreaterThan, "gte": request.GreaterEqual,
		"lt": request.LessThan, "lte": request.LessEqual,
		"sort": request.Sort, "continuation": request.Continuation,
	}
	if len(request.Pin) > 0 {
		flags["pin"] = string(request.Pin)
	}
	if request.Limit > 0 {
		flags["limit"] = request.Limit
	}
	return compactFlags(flags)
}

func (request knowledgeSearchRequest) searchRequest() (retrieval.SearchRequest, error) {
	result, err := searchRequestFromFlags(request.flags())
	if err != nil {
		return retrieval.SearchRequest{}, err
	}
	if request.Expression != nil {
		expression := *request.Expression
		result.Expression = &expression
	}
	if request.Order != nil {
		order := *request.Order
		result.Sort = &order
	}
	if err := retrieval.ValidateSearch(result); err != nil {
		return retrieval.SearchRequest{}, err
	}
	return result, nil
}

type projectionSyncRequest struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit,omitempty"`
	Ref        string `json:"ref,omitempty"`
}

func (request knowledgeReadRequest) flags() map[string]FlagValue {
	flags := map[string]FlagValue{
		"catalog": request.Catalog, "workspace": request.Workspace,
		"object": request.Object, "aspect": request.Aspect, "member": request.Member,
	}
	if len(request.Pin) > 0 {
		flags["pin"] = string(request.Pin)
	}
	if len(request.Include) > 0 {
		flags["include"] = request.Include
	}
	if len(request.Exclude) > 0 {
		flags["exclude"] = request.Exclude
	}
	return compactFlags(flags)
}

func (f *httpFacade) knowledgeRead(w http.ResponseWriter, r *http.Request) {
	var request knowledgeReadRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.executeTyped(w, r, "read", "knowledge.read", command{stage: stageGoverned, run: verbRead}, request.flags())
}

func (f *httpFacade) knowledgeSearch(w http.ResponseWriter, r *http.Request) {
	var request knowledgeSearchRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	search, err := request.searchRequest()
	if err != nil {
		writeInvoke(w, errorResult(err))
		return
	}
	flags := request.flags()
	flags["_search-request"] = search
	f.executeTyped(w, r, "search", "knowledge.search", command{stage: stageGoverned, run: verbSearch}, flags)
}

func (f *httpFacade) knowledgeRerank(w http.ResponseWriter, r *http.Request) {
	var request knowledgeRerankRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := compactFlags(map[string]FlagValue{"catalog": request.Catalog, "workspace": request.Workspace})
	if len(request.Pin) > 0 {
		flags["pin"] = string(request.Pin)
	}
	input := rerankApplicationRequest{Candidates: append([]knowledge.KnowledgeRef(nil), request.Candidates...), Spec: request.Spec}
	provider := f.options.Reranker
	f.executeTyped(w, r, "rerank", "knowledge.rerank", command{stage: stageGoverned, run: func(cx *invocation) (any, error) {
		return rerankWorkspace(cx, input, provider)
	}}, flags)
}

func (f *httpFacade) knowledgeSearchRerank(w http.ResponseWriter, r *http.Request) {
	var request knowledgeSearchRerankRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	search, err := request.searchRequest()
	if err != nil {
		writeInvoke(w, errorResult(err))
		return
	}
	if search.Continuation != "" {
		writeInvoke(w, errorResult(kernel.Fail(kernel.ErrUsageInvalid, "search:rerank requires a fresh bounded candidate window")))
		return
	}
	if search.Limit > retrieval.MaxRerankCandidates {
		writeInvoke(w, errorResult(kernel.Fail(kernel.ErrUsageInvalid,
			"search:rerank candidate limit must not exceed %d", retrieval.MaxRerankCandidates)))
		return
	}
	flags := request.flags()
	flags["_search-request"] = search
	provider := f.options.Reranker
	spec := request.Spec
	f.executeTyped(w, r, "search-rerank", "knowledge.rerank", command{stage: stageGoverned, run: func(cx *invocation) (any, error) {
		return searchAndRerankWorkspace(cx, spec, provider)
	}}, flags)
}

func (f *httpFacade) knowledgeRelations(w http.ResponseWriter, r *http.Request) {
	var request knowledgeRelationsRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := compactFlags(map[string]FlagValue{
		"catalog": request.Catalog, "workspace": request.Workspace, "object": request.Endpoint,
		"relation-type": request.RelationType, "role": request.Role, "direction": request.Direction, "limit": request.Limit,
		"continuation": request.Continuation,
	})
	if len(request.Pin) > 0 {
		flags["pin"] = string(request.Pin)
	}
	f.executeTyped(w, r, "relations", "knowledge.relations", command{stage: stageGoverned, run: verbRelations}, flags)
}

func (request knowledgeObjectRequest) flags() map[string]FlagValue {
	flags := compactFlags(map[string]FlagValue{
		"catalog": request.Catalog, "workspace": request.Workspace, "object": request.Object,
	})
	if len(request.Pin) > 0 {
		flags["pin"] = string(request.Pin)
	}
	return flags
}

func (request knowledgeSchemaRequest) flags() map[string]FlagValue {
	flags := compactFlags(map[string]FlagValue{
		"catalog": request.Catalog, "workspace": request.Workspace, "object": request.Object,
	})
	if len(request.Pin) > 0 {
		flags["pin"] = string(request.Pin)
	}
	return flags
}

func (request knowledgeBindingRequest) flags() map[string]FlagValue {
	flags := compactFlags(map[string]FlagValue{
		"catalog": request.Catalog, "workspace": request.Workspace, "object": request.Object,
		"aspect": request.Aspect, "member": request.Member,
	})
	if len(request.Pin) > 0 {
		flags["pin"] = string(request.Pin)
	}
	return flags
}

func (request knowledgeResourceAccessRequest) flags() map[string]FlagValue {
	flags := compactFlags(map[string]FlagValue{
		"catalog": request.Catalog, "workspace": request.Workspace, "object": request.Object,
		"aspect": request.Aspect, "member": request.Member, "operation": request.Operation,
	})
	if len(request.Pin) > 0 {
		flags["pin"] = string(request.Pin)
	}
	if len(request.Input) > 0 {
		flags["input"] = string(request.Input)
	}
	return flags
}

func (f *httpFacade) knowledgeProvenance(w http.ResponseWriter, r *http.Request) {
	var request knowledgeObjectRequest
	if decodeServiceRequest(w, r, &request) {
		f.executeTyped(w, r, "provenance", "knowledge.provenance", command{stage: stageGoverned, run: verbProvenance}, request.flags())
	}
}

func (f *httpFacade) knowledgeLog(w http.ResponseWriter, r *http.Request) {
	var request knowledgeObjectRequest
	if decodeServiceRequest(w, r, &request) {
		f.executeTyped(w, r, "log", "knowledge.history.read", command{stage: stageGoverned, run: verbLog}, request.flags())
	}
}

func (f *httpFacade) knowledgeSchema(w http.ResponseWriter, r *http.Request) {
	var request knowledgeSchemaRequest
	if decodeServiceRequest(w, r, &request) {
		f.executeTyped(w, r, "describe-schema", "knowledge.schema.read", command{stage: stageGoverned, run: verbDescribeSchema}, request.flags())
	}
}

func (f *httpFacade) knowledgeSchemaPage(w http.ResponseWriter, r *http.Request) {
	var request knowledgeSchemaPageRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := compactFlags(map[string]FlagValue{
		"repo": request.Repository, "commit": request.Commit, "ref": request.Ref,
		"continuation": request.Continuation,
	})
	if request.Limit > 0 {
		flags["limit"] = request.Limit
	}
	f.executeTyped(w, r, "browse-schemas", "knowledge.schema.read", command{stage: stageGoverned, run: verbBrowseSchemas}, flags)
}

func (f *httpFacade) knowledgeBinding(w http.ResponseWriter, r *http.Request) {
	var request knowledgeBindingRequest
	if decodeServiceRequest(w, r, &request) {
		f.executeTyped(w, r, "resolve-binding", "knowledge.binding.resolve", command{stage: stageGoverned, run: verbResolveBinding}, request.flags())
	}
}

func (f *httpFacade) knowledgeResourceAccess(w http.ResponseWriter, r *http.Request) {
	var request knowledgeResourceAccessRequest
	if decodeServiceRequest(w, r, &request) {
		f.executeTyped(w, r, "resource-access", "resource.access", command{stage: stageGoverned, run: verbResourceAccess}, request.flags())
	}
}

func (f *httpFacade) projectionSync(w http.ResponseWriter, r *http.Request) {
	var request projectionSyncRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := compactFlags(map[string]FlagValue{"repo": request.Repository, "commit": request.Commit, "ref": request.Ref})
	f.executeTyped(w, r, "index-sync", "projection.manage", command{stage: stageGoverned, run: verbIndexSync}, flags)
}

func (f *httpFacade) workspaceFileMounts(w http.ResponseWriter, r *http.Request) {
	var request workspaceFileMountsRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.withWorkspaceFiles(w, r, "file-mounts", request.workspaceFileCoordinate, false, func(view *workspaceFileView) (any, error) {
		return workspaceFileMountsResponse{Pin: view.pin, Mounts: view.mounts}, nil
	})
}

func (f *httpFacade) workspaceFileDirectory(w http.ResponseWriter, r *http.Request) {
	var request workspaceFileDirectoryRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.withWorkspaceFiles(w, r, "file-list", request.workspaceFileCoordinate, true, func(view *workspaceFileView) (any, error) {
		return view.list(request)
	})
}

func (f *httpFacade) workspaceFileRead(w http.ResponseWriter, r *http.Request) {
	var request workspaceFileReadRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.withWorkspaceFiles(w, r, "file-read", request.workspaceFileCoordinate, true, func(view *workspaceFileView) (any, error) {
		return view.read(request)
	})
}

func (f *httpFacade) executeTyped(w http.ResponseWriter, r *http.Request, name, action string, operation command, flags map[string]FlagValue) {
	identity, ok := f.serviceIdentity(w, r)
	if !ok {
		return
	}
	flags["home"] = f.home
	f.addIdentityFlags(flags, r, identity)
	addHTTPTraceFlags(flags, r)
	unlock := f.lockTypedInvocation(action)
	defer unlock()
	opened, err := f.readHomeForRequest()
	if err != nil {
		writeInvoke(w, errorResult(err))
		return
	}
	writeInvoke(w, invokeApplicationWithTelemetryAtHome(r.Context(), f.runtime, name, action, operation, flags, observeStateLookup(f.options.StateLookup, f.runtime), opened))
}

// lockTypedInvocation allows independent fixed-basis reads to proceed in
// parallel while mutations retain the reference implementation's single-home
// serialization. This is a process-safety boundary, not a Repository locking
// protocol; distributed writers still rely on authority CAS.
func (f *httpFacade) lockTypedInvocation(action string) func() {
	if typedInvocationReadOnly(action) {
		f.invoke.RLock()
		return f.invoke.RUnlock
	}
	f.invoke.Lock()
	return f.invoke.Unlock
}

func typedInvocationReadOnly(action string) bool {
	if strings.HasSuffix(action, ".read") {
		return true
	}
	switch action {
	case "knowledge.search", "knowledge.rerank", "knowledge.relations", "knowledge.provenance",
		"knowledge.binding.resolve", "knowledge.access.describe", "resource.access", "workspace.resolve":
		return true
	default:
		return false
	}
}

func (f *httpFacade) serviceIdentity(w http.ResponseWriter, r *http.Request) (HTTPIdentity, bool) {
	identity, ok := authenticateHTTPRequest(w, r, f.options)
	if !ok || !f.validateIdentityHeaders(w, r, identity) {
		return HTTPIdentity{}, false
	}
	if f.options.authenticated() {
		recordHTTPIdentity(f.runtime, r.Context(), f.options, identity)
		return identity, true
	}
	identity.Principal = strings.TrimSpace(r.Header.Get("X-Kc-As"))
	if identity.Principal == "" {
		writeJSON(w, http.StatusUnauthorized, kernel.FaultJSON(kernel.Fail(kernel.ErrUnauthenticated,
			"remote service requests require X-Kc-As or configured authentication")))
		return HTTPIdentity{}, false
	}
	if strings.TrimSpace(r.Header.Get("X-Kc-On-Behalf-Of")) != "" {
		writeHTTPForbidden(w, "onBehalfOf requires a trusted authenticator")
		return HTTPIdentity{}, false
	}
	recordHTTPIdentity(f.runtime, r.Context(), f.options, identity)
	return identity, true
}

func decodeServiceRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxServiceRequestBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "decode request: %v", err)))
		return false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		writeJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "request body must contain one JSON object")))
		return false
	}
	return true
}

func compactFlags(flags map[string]FlagValue) map[string]FlagValue {
	for name, value := range flags {
		switch typed := value.(type) {
		case string:
			if typed == "" {
				delete(flags, name)
			}
		case []string:
			if len(typed) == 0 {
				delete(flags, name)
			}
		}
	}
	return flags
}
