package server

import (
	inv "kc/invocation"
	"kc/invoke"

	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	retrieval "kc/discovery"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot/dataset"
)

// Commit bodies legitimately carry whole-repository ChangeSets (a 53k object
// import measured ~228 MB raw, ~tens of MB gzipped), so the writer commit
// route enforces its own cap instead of the per-route service default.
const defaultMaxCommitRequestBytes = 512 << 20

// maxCommitRequestBytes reads the optional commit-route body-cap override.
// Accepts plain bytes or a Go-style size ("32MiB"); invalid values fall back
// to the default.
func maxCommitRequestBytes() int {
	raw := strings.TrimSpace(os.Getenv("KC_MAX_COMMIT_REQUEST_BYTES"))
	if raw == "" {
		return defaultMaxCommitRequestBytes
	}
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return n
	}
	if n, err := parseSize(raw); err == nil && n > 0 {
		return n
	}
	return defaultMaxCommitRequestBytes
}

// parseSize parses "32MiB"/"512MB"-style sizes; bytes fall through to Atoi.
func parseSize(raw string) (int, error) {
	upper := strings.ToUpper(raw)
	multiplier := 0
	switch {
	case strings.HasSuffix(upper, "KIB"):
		multiplier = 1 << 10
		upper = strings.TrimSuffix(upper, "KIB")
	case strings.HasSuffix(upper, "MIB"):
		multiplier = 1 << 20
		upper = strings.TrimSuffix(upper, "MIB")
	case strings.HasSuffix(upper, "GIB"):
		multiplier = 1 << 30
		upper = strings.TrimSuffix(upper, "GIB")
	default:
		return 0, fmt.Errorf("unsupported size %q", raw)
	}
	n, err := strconv.Atoi(strings.TrimSpace(upper))
	if err != nil {
		return 0, err
	}
	if n > (1<<62)/multiplier {
		return 0, fmt.Errorf("size %q overflows", raw)
	}
	return n * multiplier, nil
}

// registerServiceRoutes is an explicit API registry. It does not inspect the
// CLI surface or internal operation table, so adding a CLI inv.Command can never
// create an HTTP endpoint accidentally.
func (f *httpFacade) registerServiceRoutes(mux *http.ServeMux) {
	f.registerAdmissionSharingRoutes(mux)
	f.registerConnectionRoutes(mux)
	f.registerManagementRoutes(mux)
	mux.HandleFunc("POST /identity/v1/token", f.identityToken)
	mux.HandleFunc("POST /identity/v1/authorize", f.identityAuthorize)
	mux.HandleFunc("POST /identity/v1/authorize:poll", f.identityAuthorizePoll)
	mux.HandleFunc("POST /object/v1/objects:read", f.objectRead)
	mux.HandleFunc("POST /object/v1/objects:diff", f.objectDiff)
	mux.HandleFunc("POST /object/v1/objects:put", f.objectPut)
	mux.HandleFunc("POST /object/v1/objects:remove", f.objectRemove)
	mux.HandleFunc("POST /object/v1/objects:resolve", f.objectResolve)
	mux.HandleFunc("POST /object/v1/search", f.objectSearch)
	mux.HandleFunc("POST /object/v1/search:rerank", f.objectSearchRerank)
	mux.HandleFunc("POST /object/v1/rerank", f.objectRerank)
	mux.HandleFunc("POST /object/v1/relations:query", f.objectRelations)
	mux.HandleFunc("POST /object/v1/traverse:query", f.objectTraverse)
	mux.HandleFunc("POST /object/v1/provenance:describe", f.objectProvenance)
	mux.HandleFunc("POST /object/v1/log:query", f.objectLog)
	mux.HandleFunc("POST /object/v1/schemas:describe", f.objectSchema)
	mux.HandleFunc("POST /object/v1/schemas:list", f.objectSchemaPage)
	mux.HandleFunc("POST /object/v1/bindings:resolve", f.objectBinding)
	mux.HandleFunc("POST /object/v1/resources:access", f.objectResourceAccess)
}

// objectDatasetIdentity is the shared dataset-channel identity envelope:
// a named workspace, or an inline published pin/definition.
type objectDatasetIdentity struct {
	Dataset    string           `json:"dataset,omitempty"`
	Revision   int              `json:"revision,omitempty"`
	Pin        json.RawMessage  `json:"pin,omitempty"`
	Definition *dataset.Dataset `json:"definition,omitempty"`
}

// objectScope is the full scope envelope of object API requests: the
// dataset identity or one fixed repository basis. Kept flat so request
// literals stay readable; embedded in every request struct, the JSON wire
// contract is unchanged.
type objectScope struct {
	Dataset    string           `json:"dataset,omitempty"`
	Revision   int              `json:"revision,omitempty"`
	Pin        json.RawMessage  `json:"pin,omitempty"`
	Definition *dataset.Dataset `json:"definition,omitempty"`
	Repository string           `json:"repository,omitempty"`
	Commit     string           `json:"commit,omitempty"`
	Ref        string           `json:"ref,omitempty"`
}

type objectReadRequest struct {
	objectDatasetIdentity
	Basis      string `json:"basis,omitempty"`
	Reference  string `json:"reference"`
	Shape      string `json:"shape,omitempty"`
	Repository string `json:"repository,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Ref        string `json:"ref,omitempty"`
}

type ObjectSearchRequest struct {
	objectScope
	Query        string                  `json:"query,omitempty"`
	Match        []string                `json:"match,omitempty"`
	MatchMode    string                  `json:"matchMode,omitempty"`
	Equal        []string                `json:"equal,omitempty"`
	NotEqual     []string                `json:"notEqual,omitempty"`
	In           []string                `json:"in,omitempty"`
	Exists       []string                `json:"exists,omitempty"`
	Missing      []string                `json:"missing,omitempty"`
	Prefix       []string                `json:"prefix,omitempty"`
	Contains     []string                `json:"contains,omitempty"`
	GreaterThan  []string                `json:"greaterThan,omitempty"`
	GreaterEqual []string                `json:"greaterEqual,omitempty"`
	LessThan     []string                `json:"lessThan,omitempty"`
	LessEqual    []string                `json:"lessEqual,omitempty"`
	Sort         []string                `json:"sort,omitempty"`
	Limit        int                     `json:"limit,omitempty"`
	Continuation string                  `json:"continuation,omitempty"`
	Expression   *retrieval.SearchExpr   `json:"expression,omitempty"`
	Order        *retrieval.SearchClause `json:"order,omitempty"`
	Recall       string                  `json:"recall,omitempty"`
}

type objectRelationsRequest struct {
	objectScope
	Endpoint     string `json:"endpoint"`
	RelationType string `json:"relationType,omitempty"`
	Role         string `json:"role,omitempty"`
	Direction    string `json:"direction,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

type objectTraverseRequest struct {
	objectScope
	Endpoint     string `json:"endpoint"`
	RelationType string `json:"relationType,omitempty"`
	Role         string `json:"role,omitempty"`
	Direction    string `json:"direction,omitempty"`
	MinHops      int    `json:"minHops,omitempty"`
	MaxHops      int    `json:"maxHops"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

type objectRerankRequest struct {
	objectDatasetIdentity
	Candidates []objectmodel.ObjectRef        `json:"candidates"`
	Spec       retrieval.SemanticOperatorSpec `json:"spec"`
}

type objectSearchRerankRequest struct {
	ObjectSearchRequest
	Spec retrieval.SemanticOperatorSpec `json:"spec"`
}

type objectRequest struct {
	objectScope
	Object       string `json:"object"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

type objectSchemaRequest struct {
	objectScope
	Object string `json:"object,omitempty"`
}

type objectSchemaPageRequest struct {
	Dataset      string `json:"dataset,omitempty"`
	Revision     int    `json:"revision,omitempty"`
	Repository   string `json:"repository,omitempty"`
	Commit       string `json:"commit,omitempty"`
	Ref          string `json:"ref,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

type objectBindingRequest struct {
	objectScope
	Object string `json:"object"`
	Aspect string `json:"aspect"`
	Member string `json:"member,omitempty"`
}

type objectResourceAccessRequest struct {
	objectScope
	Object         string          `json:"object"`
	Aspect         string          `json:"aspect,omitempty"`
	Member         string          `json:"member,omitempty"`
	Operation      string          `json:"operation,omitempty"`
	Input          json.RawMessage `json:"input,omitempty"`
	Implementation string          `json:"implementation,omitempty"`
}

func (request ObjectSearchRequest) Flags() map[string]FlagValue {
	flags := map[string]FlagValue{
		"dataset": request.Dataset, "query": request.Query,
		"repo": request.Repository, "commit": request.Commit, "ref": request.Ref,
		"match": request.Match, "match-mode": request.MatchMode, "eq": request.Equal,
		"neq": request.NotEqual, "in": request.In, "exists": request.Exists,
		"missing": request.Missing, "prefix": request.Prefix, "contains": request.Contains,
		"gt": request.GreaterThan, "gte": request.GreaterEqual,
		"lt": request.LessThan, "lte": request.LessEqual,
		"sort": request.Sort, "continuation": request.Continuation,
		"recall": request.Recall,
	}
	if request.Definition != nil {
		flags[inv.WorkspaceDefinitionFlag] = request.Definition
	}

	if len(request.Pin) > 0 {
		flags["pin"] = string(request.Pin)
	}
	if request.Limit > 0 {
		flags["limit"] = request.Limit
	}
	return withRevision(compactFlags(flags), request.Revision)
}

func (request ObjectSearchRequest) searchRequest() (retrieval.SearchRequest, error) {
	result, err := inv.SearchRequestFromFlags(request.Flags())
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

// withRevision carries a named Dataset's accepted revision; invocation turns
// it into a replayed published pin, so it never widens scope.
func withRevision(flags map[string]FlagValue, revision int) map[string]FlagValue {
	if revision != 0 {
		flags["revision"] = strconv.Itoa(revision)
	}
	return flags
}

func objectCoordinateFlags(workspace, repository, commit, ref string, pin json.RawMessage, definition ...*dataset.Dataset) map[string]FlagValue {
	flags := compactFlags(map[string]FlagValue{
		"dataset": workspace,
		"repo":    repository, "commit": commit, "ref": ref,
	})
	if len(pin) > 0 {
		flags["pin"] = string(pin)
	}
	if len(definition) > 0 && definition[0] != nil {
		flags[inv.WorkspaceDefinitionFlag] = definition[0]
	}
	return flags
}

func (request objectReadRequest) Flags() map[string]FlagValue {
	flags := objectCoordinateFlags(request.Dataset, request.Repository, request.Commit, request.Ref, request.Pin, request.Definition)
	if request.Basis != "" {
		flags["basis"] = request.Basis
	}
	flags["reference"] = request.Reference
	if request.Shape != "" {
		flags["shape"] = request.Shape
	}
	return withRevision(flags, request.Revision)
}

func (f *httpFacade) objectRead(w http.ResponseWriter, r *http.Request) {
	var request objectReadRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	if !requireObjectReference(w, request.Reference) {
		return
	}
	f.executeTyped(w, r, "object-read", "object.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbRead}, request.Flags())
}

func (f *httpFacade) objectResolve(w http.ResponseWriter, r *http.Request) {
	var request objectBindingRequest
	if decodeServiceRequest(w, r, &request) {
		f.executeTyped(w, r, "object-resolve", "object.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbResolveObject}, request.Flags())
	}
}

func (f *httpFacade) objectSearch(w http.ResponseWriter, r *http.Request) {
	var request ObjectSearchRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	if err := inv.RequireDatasetDiscoveryScope(request.Flags()); err != nil {
		invoke.WriteInvoke(w, inv.ErrorResult(err))
		return
	}
	search, err := request.searchRequest()
	if err != nil {
		invoke.WriteInvoke(w, inv.ErrorResult(err))
		return
	}
	flags := request.Flags()
	flags["_search-request"] = search
	if f.options.Embedder != nil {
		flags["_embedder"] = f.options.Embedder
	}
	f.executeTyped(w, r, "object-search", "file.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbSearch}, flags)
}

func (f *httpFacade) objectRerank(w http.ResponseWriter, r *http.Request) {
	var request objectRerankRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := compactFlags(map[string]FlagValue{"dataset": request.Dataset})
	if request.Definition != nil {
		flags[inv.WorkspaceDefinitionFlag] = request.Definition
	}
	if len(request.Pin) > 0 {
		flags["pin"] = string(request.Pin)
	}
	input := inv.RerankApplicationRequest{Candidates: append([]objectmodel.ObjectRef(nil), request.Candidates...), Spec: request.Spec}
	provider := f.options.Reranker
	f.executeTyped(w, r, "rerank", "object.rerank", inv.Command{Stage: invoke.StageGoverned, Run: func(cx *inv.Invocation) (any, error) {
		return inv.RerankWorkspace(cx, input, provider)
	}}, flags)
}

func (f *httpFacade) objectSearchRerank(w http.ResponseWriter, r *http.Request) {
	var request objectSearchRerankRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	if err := inv.RequireDatasetDiscoveryScope(request.Flags()); err != nil {
		invoke.WriteInvoke(w, inv.ErrorResult(err))
		return
	}
	search, err := request.searchRequest()
	if err != nil {
		invoke.WriteInvoke(w, inv.ErrorResult(err))
		return
	}
	if search.Continuation != "" {
		invoke.WriteInvoke(w, inv.ErrorResult(kernel.Fail(kernel.ErrUsageInvalid, "search:rerank requires a fresh bounded candidate window")))
		return
	}
	if search.Limit > retrieval.MaxRerankCandidates {
		invoke.WriteInvoke(w, inv.ErrorResult(kernel.Fail(kernel.ErrUsageInvalid,
			"search:rerank candidate limit must not exceed %d", retrieval.MaxRerankCandidates)))
		return
	}
	flags := request.Flags()
	flags["_search-request"] = search
	provider := f.options.Reranker
	spec := request.Spec
	f.executeTyped(w, r, "search-rerank", "object.rerank", inv.Command{Stage: invoke.StageGoverned, Run: func(cx *inv.Invocation) (any, error) {
		return inv.SearchAndRerankWorkspace(cx, spec, provider)
	}}, flags)
}

func (f *httpFacade) objectRelations(w http.ResponseWriter, r *http.Request) {
	var request objectRelationsRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	if err := inv.RequireDatasetDiscoveryScope(objectCoordinateFlags(request.Dataset, request.Repository, request.Commit, request.Ref, request.Pin, request.Definition)); err != nil {
		invoke.WriteInvoke(w, inv.ErrorResult(err))
		return
	}
	flags := withRevision(objectCoordinateFlags(request.Dataset, request.Repository, request.Commit, request.Ref, request.Pin, request.Definition), request.Revision)
	if request.Endpoint != "" {
		flags["object"] = request.Endpoint
	}
	if request.RelationType != "" {
		flags["relation-type"] = request.RelationType
	}
	if request.Role != "" {
		flags["role"] = request.Role
	}
	if request.Direction != "" {
		flags["direction"] = request.Direction
	}
	if request.Limit > 0 {
		flags["limit"] = request.Limit
	}
	if request.Continuation != "" {
		flags["continuation"] = request.Continuation
	}
	f.executeTyped(w, r, "object-relations", "file.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbRelations}, flags)
}

func (f *httpFacade) objectTraverse(w http.ResponseWriter, r *http.Request) {
	var request objectTraverseRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	if err := inv.RequireDatasetDiscoveryScope(objectCoordinateFlags(request.Dataset, request.Repository, request.Commit, request.Ref, request.Pin, request.Definition)); err != nil {
		invoke.WriteInvoke(w, inv.ErrorResult(err))
		return
	}
	flags := withRevision(objectCoordinateFlags(request.Dataset, request.Repository, request.Commit, request.Ref, request.Pin, request.Definition), request.Revision)
	if request.Endpoint != "" {
		flags["object"] = request.Endpoint
	}
	if request.RelationType != "" {
		flags["relation-type"] = request.RelationType
	}
	if request.Role != "" {
		flags["role"] = request.Role
	}
	if request.Direction != "" {
		flags["direction"] = request.Direction
	}
	if request.MaxHops > 0 {
		flags["max-hops"] = request.MaxHops
	}
	if request.MinHops > 0 {
		flags["min-hops"] = request.MinHops
	}
	if request.Limit > 0 {
		flags["limit"] = request.Limit
	}
	if request.Continuation != "" {
		flags["continuation"] = request.Continuation
	}
	f.executeTyped(w, r, "object-traverse", "file.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbTraverse}, flags)
}

func (request objectRequest) Flags() map[string]FlagValue {
	flags := withRevision(objectCoordinateFlags(request.Dataset, request.Repository, request.Commit, request.Ref, request.Pin, request.Definition), request.Revision)
	if request.Object != "" {
		flags["object"] = request.Object
	}
	if request.Limit > 0 {
		flags["limit"] = request.Limit
	}
	if request.Continuation != "" {
		flags["continuation"] = request.Continuation
	}
	return flags
}

func (request objectSchemaRequest) Flags() map[string]FlagValue {
	flags := withRevision(objectCoordinateFlags(request.Dataset, request.Repository, request.Commit, request.Ref, request.Pin, request.Definition), request.Revision)
	if request.Object != "" {
		flags["object"] = request.Object
	}
	return flags
}

func (request objectBindingRequest) Flags() map[string]FlagValue {
	flags := withRevision(objectCoordinateFlags(request.Dataset, request.Repository, request.Commit, request.Ref, request.Pin, request.Definition), request.Revision)
	if request.Object != "" {
		flags["object"] = request.Object
	}
	if request.Aspect != "" {
		flags["aspect"] = request.Aspect
	}
	if request.Member != "" {
		flags["member"] = request.Member
	}
	return flags
}

func (request objectResourceAccessRequest) Flags() map[string]FlagValue {
	flags := compactFlags(map[string]FlagValue{
		"dataset": request.Dataset, "object": request.Object,
		"repo": request.Repository, "commit": request.Commit, "ref": request.Ref,
		"aspect": request.Aspect, "member": request.Member, "operation": request.Operation,
		"implementation": request.Implementation,
	})
	withRevision(flags, request.Revision)
	if request.Definition != nil {
		flags[inv.WorkspaceDefinitionFlag] = request.Definition
	}
	if len(request.Pin) > 0 {
		flags["pin"] = string(request.Pin)
	}
	if len(request.Input) > 0 {
		flags["input"] = string(request.Input)
	}
	return flags
}

func (f *httpFacade) objectProvenance(w http.ResponseWriter, r *http.Request) {
	var request objectRequest
	if decodeServiceRequest(w, r, &request) {
		f.executeTyped(w, r, "object-provenance", "object.provenance", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbProvenance}, request.Flags())
	}
}

func (f *httpFacade) objectLog(w http.ResponseWriter, r *http.Request) {
	var request objectRequest
	if decodeServiceRequest(w, r, &request) {
		f.executeTyped(w, r, "object-log", "object.history.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbLog}, request.Flags())
	}
}

func (f *httpFacade) objectSchema(w http.ResponseWriter, r *http.Request) {
	var request objectSchemaRequest
	if decodeServiceRequest(w, r, &request) {
		f.executeTyped(w, r, "object-schema-describe", "object.schema.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbDescribeSchema}, request.Flags())
	}
}

func (f *httpFacade) objectSchemaPage(w http.ResponseWriter, r *http.Request) {
	var request objectSchemaPageRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := withRevision(compactFlags(map[string]FlagValue{
		"dataset": request.Dataset,
		"repo":    request.Repository, "commit": request.Commit, "ref": request.Ref,
		"continuation": request.Continuation,
	}), request.Revision)
	if request.Limit > 0 {
		flags["limit"] = request.Limit
	}
	f.executeTyped(w, r, "object-schema-list", "object.schema.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbBrowseSchemas}, flags)
}

func (f *httpFacade) objectBinding(w http.ResponseWriter, r *http.Request) {
	var request objectBindingRequest
	if decodeServiceRequest(w, r, &request) {
		f.executeTyped(w, r, "object-binding-show", "object.binding.resolve", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbResolveBinding}, request.Flags())
	}
}

func (f *httpFacade) objectResourceAccess(w http.ResponseWriter, r *http.Request) {
	var request objectResourceAccessRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	handler := "object-access"
	run := inv.VerbResourceAccess
	if strings.TrimSpace(request.Operation) != "" || strings.TrimSpace(request.Implementation) != "" {
		handler = "object-invoke"
		run = inv.VerbResourceInvoke
	}
	f.executeTyped(w, r, handler, "resource.access", inv.Command{Stage: invoke.StageGoverned, Run: run}, request.Flags())
}
