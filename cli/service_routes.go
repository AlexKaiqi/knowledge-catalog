package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"kc/kernel"
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
	mux.HandleFunc("POST /knowledge/v1/relations:query", f.knowledgeRelations)
	mux.HandleFunc("POST /knowledge/v1/provenance:get", f.knowledgeProvenance)
	mux.HandleFunc("POST /knowledge/v1/log:get", f.knowledgeLog)
	mux.HandleFunc("POST /knowledge/v1/schemas:get", f.knowledgeSchema)
	mux.HandleFunc("POST /knowledge/v1/bindings:resolve", f.knowledgeBinding)
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
	Catalog      string          `json:"catalog,omitempty"`
	Workspace    string          `json:"workspace"`
	Pin          json.RawMessage `json:"pin,omitempty"`
	Query        string          `json:"query,omitempty"`
	Match        []string        `json:"match,omitempty"`
	MatchMode    string          `json:"matchMode,omitempty"`
	Equal        []string        `json:"equal,omitempty"`
	NotEqual     []string        `json:"notEqual,omitempty"`
	Sort         []string        `json:"sort,omitempty"`
	Limit        int             `json:"limit,omitempty"`
	Continuation string          `json:"continuation,omitempty"`
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

type knowledgeBindingRequest struct {
	Catalog   string          `json:"catalog,omitempty"`
	Workspace string          `json:"workspace"`
	Pin       json.RawMessage `json:"pin,omitempty"`
	Object    string          `json:"object"`
	Aspect    string          `json:"aspect"`
	Member    string          `json:"member,omitempty"`
}

func (request knowledgeSearchRequest) flags() map[string]FlagValue {
	flags := map[string]FlagValue{
		"catalog": request.Catalog, "workspace": request.Workspace, "query": request.Query,
		"match": request.Match, "match-mode": request.MatchMode, "eq": request.Equal,
		"neq": request.NotEqual, "sort": request.Sort, "continuation": request.Continuation,
	}
	if len(request.Pin) > 0 {
		flags["pin"] = string(request.Pin)
	}
	if request.Limit > 0 {
		flags["limit"] = request.Limit
	}
	return compactFlags(flags)
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
	f.executeTyped(w, r, "search", "knowledge.search", command{stage: stageGoverned, run: verbSearch}, request.flags())
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

func (f *httpFacade) knowledgeBinding(w http.ResponseWriter, r *http.Request) {
	var request knowledgeBindingRequest
	if decodeServiceRequest(w, r, &request) {
		f.executeTyped(w, r, "resolve-binding", "knowledge.binding.resolve", command{stage: stageGoverned, run: verbResolveBinding}, request.flags())
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
	f.withWorkspaceFiles(w, r, request.workspaceFileCoordinate, false, func(view *workspaceFileView) (any, error) {
		return workspaceFileMountsResponse{Pin: view.pin, Mounts: view.mounts}, nil
	})
}

func (f *httpFacade) workspaceFileDirectory(w http.ResponseWriter, r *http.Request) {
	var request workspaceFileDirectoryRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.withWorkspaceFiles(w, r, request.workspaceFileCoordinate, true, func(view *workspaceFileView) (any, error) {
		return view.list(request)
	})
}

func (f *httpFacade) workspaceFileRead(w http.ResponseWriter, r *http.Request) {
	var request workspaceFileReadRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.withWorkspaceFiles(w, r, request.workspaceFileCoordinate, true, func(view *workspaceFileView) (any, error) {
		return view.read(request)
	})
}

func (f *httpFacade) withWorkspaceFiles(w http.ResponseWriter, r *http.Request, coordinate workspaceFileCoordinate, requirePin bool, run func(*workspaceFileView) (any, error)) {
	identity, ok := f.serviceIdentity(w, r)
	if !ok {
		return
	}
	view, err := openWorkspaceFileView(f.home, identity.Principal, coordinate, requirePin)
	if err != nil {
		writeInvoke(w, errorResult(err))
		return
	}
	defer view.Close()
	result, err := run(view)
	if err != nil {
		writeInvoke(w, errorResult(err))
		return
	}
	writeInvoke(w, RunResult{Stdout: jsonOut(result)})
}

func (f *httpFacade) executeTyped(w http.ResponseWriter, r *http.Request, name, action string, operation command, flags map[string]FlagValue) {
	identity, ok := f.serviceIdentity(w, r)
	if !ok {
		return
	}
	flags["home"] = f.home
	f.addIdentityFlags(flags, r, identity)
	addHTTPTraceFlags(flags, r)
	f.invoke.Lock()
	defer f.invoke.Unlock()
	opened, err := f.readHomeForRequest()
	if err != nil {
		writeInvoke(w, errorResult(err))
		return
	}
	writeInvoke(w, invokeApplicationAtHome(r.Context(), name, action, operation, flags, f.options.StateLookup, opened))
}

func (f *httpFacade) serviceIdentity(w http.ResponseWriter, r *http.Request) (HTTPIdentity, bool) {
	identity, ok := authenticateHTTPRequest(w, r, f.options)
	if !ok || !f.validateIdentityHeaders(w, r, identity) {
		return HTTPIdentity{}, false
	}
	if f.options.authenticated() {
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
