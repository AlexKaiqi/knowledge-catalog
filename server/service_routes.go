package server

import (
	inv "kc/invocation"
	"kc/invoke"

	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"
	"kc/retrieval"
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
	mux.HandleFunc("POST /knowledge/v1/objects:read", f.knowledgeRead)
	mux.HandleFunc("POST /knowledge/v1/objects:resolve", f.knowledgeResolve)
	mux.HandleFunc("POST /knowledge/v1/search", f.knowledgeSearch)
	mux.HandleFunc("POST /knowledge/v1/search:rerank", f.knowledgeSearchRerank)
	mux.HandleFunc("POST /knowledge/v1/rerank", f.knowledgeRerank)
	mux.HandleFunc("POST /knowledge/v1/relations:query", f.knowledgeRelations)
	mux.HandleFunc("POST /knowledge/v1/traverse:query", f.knowledgeTraverse)
	mux.HandleFunc("POST /knowledge/v1/provenance:describe", f.knowledgeProvenance)
	mux.HandleFunc("POST /knowledge/v1/log:query", f.knowledgeLog)
	mux.HandleFunc("POST /knowledge/v1/schemas:describe", f.knowledgeSchema)
	mux.HandleFunc("POST /knowledge/v1/schemas:list", f.knowledgeSchemaPage)
	mux.HandleFunc("POST /knowledge/v1/bindings:resolve", f.knowledgeBinding)
	mux.HandleFunc("POST /knowledge/v1/resources:access", f.knowledgeResourceAccess)
	mux.HandleFunc("POST /operations/v1/projections:sync", f.projectionSync)
	mux.HandleFunc("POST /operations/v1/projections:notice", f.projectionNotify)
}

// knowledgeDatasetIdentity is the shared dataset-channel identity envelope:
// a named workspace, or an inline published pin/definition.
type knowledgeDatasetIdentity struct {
	Dataset    string                `json:"dataset,omitempty"`
	Pin        json.RawMessage       `json:"pin,omitempty"`
	Definition *dataset.KnowledgeSet `json:"definition,omitempty"`
}

// knowledgeScope is the full scope envelope of knowledge API requests: the
// dataset identity or one fixed repository basis. Kept flat so request
// literals stay readable; embedded in every request struct, the JSON wire
// contract is unchanged.
type knowledgeScope struct {
	Dataset    string                `json:"dataset,omitempty"`
	Pin        json.RawMessage       `json:"pin,omitempty"`
	Definition *dataset.KnowledgeSet `json:"definition,omitempty"`
	Repository string                `json:"repository,omitempty"`
	Commit     string                `json:"commit,omitempty"`
	Ref        string                `json:"ref,omitempty"`
}

type knowledgeReadRequest struct {
	knowledgeScope
	Object  string   `json:"object"`
	Aspect  string   `json:"aspect,omitempty"`
	Member  string   `json:"member,omitempty"`
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

type KnowledgeSearchRequest struct {
	knowledgeScope
	InventoryDiscovery bool                    `json:"inventoryDiscovery,omitempty"`
	Query              string                  `json:"query,omitempty"`
	Match              []string                `json:"match,omitempty"`
	MatchMode          string                  `json:"matchMode,omitempty"`
	Equal              []string                `json:"equal,omitempty"`
	NotEqual           []string                `json:"notEqual,omitempty"`
	In                 []string                `json:"in,omitempty"`
	Exists             []string                `json:"exists,omitempty"`
	Missing            []string                `json:"missing,omitempty"`
	Prefix             []string                `json:"prefix,omitempty"`
	Contains           []string                `json:"contains,omitempty"`
	GreaterThan        []string                `json:"greaterThan,omitempty"`
	GreaterEqual       []string                `json:"greaterEqual,omitempty"`
	LessThan           []string                `json:"lessThan,omitempty"`
	LessEqual          []string                `json:"lessEqual,omitempty"`
	Sort               []string                `json:"sort,omitempty"`
	Limit              int                     `json:"limit,omitempty"`
	Continuation       string                  `json:"continuation,omitempty"`
	Expression         *retrieval.SearchExpr   `json:"expression,omitempty"`
	Order              *retrieval.SearchClause `json:"order,omitempty"`
	Recall             string                  `json:"recall,omitempty"`
}

type knowledgeRelationsRequest struct {
	knowledgeScope
	Endpoint     string `json:"endpoint"`
	RelationType string `json:"relationType,omitempty"`
	Role         string `json:"role,omitempty"`
	Direction    string `json:"direction,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

type knowledgeTraverseRequest struct {
	knowledgeScope
	Endpoint     string `json:"endpoint"`
	RelationType string `json:"relationType,omitempty"`
	Role         string `json:"role,omitempty"`
	Direction    string `json:"direction,omitempty"`
	MinHops      int    `json:"minHops,omitempty"`
	MaxHops      int    `json:"maxHops"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

type knowledgeRerankRequest struct {
	knowledgeDatasetIdentity
	Candidates []knowledge.KnowledgeRef       `json:"candidates"`
	Spec       retrieval.SemanticOperatorSpec `json:"spec"`
}

type knowledgeSearchRerankRequest struct {
	KnowledgeSearchRequest
	Spec retrieval.SemanticOperatorSpec `json:"spec"`
}

type knowledgeObjectRequest struct {
	knowledgeScope
	Object       string `json:"object"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

type knowledgeSchemaRequest struct {
	knowledgeScope
	Object string `json:"object,omitempty"`
}

type knowledgeSchemaPageRequest struct {
	Repository   string `json:"repository"`
	Commit       string `json:"commit,omitempty"`
	Ref          string `json:"ref,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

type knowledgeBindingRequest struct {
	knowledgeScope
	Object string `json:"object"`
	Aspect string `json:"aspect"`
	Member string `json:"member,omitempty"`
}

type knowledgeResourceAccessRequest struct {
	knowledgeScope
	Object         string          `json:"object"`
	Aspect         string          `json:"aspect,omitempty"`
	Member         string          `json:"member,omitempty"`
	Operation      string          `json:"operation,omitempty"`
	Input          json.RawMessage `json:"input,omitempty"`
	Implementation string          `json:"implementation,omitempty"`
}

func (request KnowledgeSearchRequest) Flags() map[string]FlagValue {
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
	if request.InventoryDiscovery {
		flags[inv.InventoryDiscoveryFlag] = true
	}
	if len(request.Pin) > 0 {
		flags["pin"] = string(request.Pin)
	}
	if request.Limit > 0 {
		flags["limit"] = request.Limit
	}
	return compactFlags(flags)
}

func (request KnowledgeSearchRequest) searchRequest() (retrieval.SearchRequest, error) {
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

type projectionSyncRequest struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit,omitempty"`
	Ref        string `json:"ref,omitempty"`
}

func knowledgeCoordinateFlags(workspace, repository, commit, ref string, pin json.RawMessage, definition ...*dataset.KnowledgeSet) map[string]FlagValue {
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

func (request knowledgeReadRequest) Flags() map[string]FlagValue {
	flags := knowledgeCoordinateFlags(request.Dataset, request.Repository, request.Commit, request.Ref, request.Pin, request.Definition)
	if request.Object != "" {
		flags["object"] = request.Object
	}
	if request.Aspect != "" {
		flags["aspect"] = request.Aspect
	}
	if request.Member != "" {
		flags["member"] = request.Member
	}
	if len(request.Include) > 0 {
		flags["include"] = request.Include
	}
	if len(request.Exclude) > 0 {
		flags["exclude"] = request.Exclude
	}
	return flags
}

func (f *httpFacade) knowledgeRead(w http.ResponseWriter, r *http.Request) {
	var request knowledgeReadRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	f.executeTyped(w, r, "knowledge-read", "knowledge.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbRead}, request.Flags())
}

func (f *httpFacade) knowledgeResolve(w http.ResponseWriter, r *http.Request) {
	var request knowledgeBindingRequest
	if decodeServiceRequest(w, r, &request) {
		f.executeTyped(w, r, "knowledge-resolve", "knowledge.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbResolveObject}, request.Flags())
	}
}

func (f *httpFacade) knowledgeSearch(w http.ResponseWriter, r *http.Request) {
	var request KnowledgeSearchRequest
	if !decodeServiceRequest(w, r, &request) {
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
	f.executeTyped(w, r, "knowledge-search", "knowledge.search", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbSearch}, flags)
}

func (f *httpFacade) knowledgeRerank(w http.ResponseWriter, r *http.Request) {
	var request knowledgeRerankRequest
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
	input := inv.RerankApplicationRequest{Candidates: append([]knowledge.KnowledgeRef(nil), request.Candidates...), Spec: request.Spec}
	provider := f.options.Reranker
	f.executeTyped(w, r, "rerank", "knowledge.rerank", inv.Command{Stage: invoke.StageGoverned, Run: func(cx *inv.Invocation) (any, error) {
		return inv.RerankWorkspace(cx, input, provider)
	}}, flags)
}

func (f *httpFacade) knowledgeSearchRerank(w http.ResponseWriter, r *http.Request) {
	var request knowledgeSearchRerankRequest
	if !decodeServiceRequest(w, r, &request) {
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
	f.executeTyped(w, r, "search-rerank", "knowledge.rerank", inv.Command{Stage: invoke.StageGoverned, Run: func(cx *inv.Invocation) (any, error) {
		return inv.SearchAndRerankWorkspace(cx, spec, provider)
	}}, flags)
}

func (f *httpFacade) knowledgeRelations(w http.ResponseWriter, r *http.Request) {
	var request knowledgeRelationsRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := knowledgeCoordinateFlags(request.Dataset, request.Repository, request.Commit, request.Ref, request.Pin, request.Definition)
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
	f.executeTyped(w, r, "knowledge-relations", "knowledge.relations", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbRelations}, flags)
}

func (f *httpFacade) knowledgeTraverse(w http.ResponseWriter, r *http.Request) {
	var request knowledgeTraverseRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := knowledgeCoordinateFlags(request.Dataset, request.Repository, request.Commit, request.Ref, request.Pin, request.Definition)
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
	f.executeTyped(w, r, "knowledge-traverse", "knowledge.traverse", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbTraverse}, flags)
}

func (request knowledgeObjectRequest) Flags() map[string]FlagValue {
	flags := knowledgeCoordinateFlags(request.Dataset, request.Repository, request.Commit, request.Ref, request.Pin, request.Definition)
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

func (request knowledgeSchemaRequest) Flags() map[string]FlagValue {
	flags := knowledgeCoordinateFlags(request.Dataset, request.Repository, request.Commit, request.Ref, request.Pin, request.Definition)
	if request.Object != "" {
		flags["object"] = request.Object
	}
	return flags
}

func (request knowledgeBindingRequest) Flags() map[string]FlagValue {
	flags := knowledgeCoordinateFlags(request.Dataset, request.Repository, request.Commit, request.Ref, request.Pin, request.Definition)
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

func (request knowledgeResourceAccessRequest) Flags() map[string]FlagValue {
	flags := compactFlags(map[string]FlagValue{
		"dataset": request.Dataset, "object": request.Object,
		"repo": request.Repository, "commit": request.Commit, "ref": request.Ref,
		"aspect": request.Aspect, "member": request.Member, "operation": request.Operation,
		"implementation": request.Implementation,
	})
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

func (f *httpFacade) knowledgeProvenance(w http.ResponseWriter, r *http.Request) {
	var request knowledgeObjectRequest
	if decodeServiceRequest(w, r, &request) {
		f.executeTyped(w, r, "knowledge-provenance", "knowledge.provenance", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbProvenance}, request.Flags())
	}
}

func (f *httpFacade) knowledgeLog(w http.ResponseWriter, r *http.Request) {
	var request knowledgeObjectRequest
	if decodeServiceRequest(w, r, &request) {
		f.executeTyped(w, r, "knowledge-log", "knowledge.history.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbLog}, request.Flags())
	}
}

func (f *httpFacade) knowledgeSchema(w http.ResponseWriter, r *http.Request) {
	var request knowledgeSchemaRequest
	if decodeServiceRequest(w, r, &request) {
		f.executeTyped(w, r, "knowledge-schema-describe", "knowledge.schema.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbDescribeSchema}, request.Flags())
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
	f.executeTyped(w, r, "knowledge-schema-list", "knowledge.schema.read", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbBrowseSchemas}, flags)
}

func (f *httpFacade) knowledgeBinding(w http.ResponseWriter, r *http.Request) {
	var request knowledgeBindingRequest
	if decodeServiceRequest(w, r, &request) {
		f.executeTyped(w, r, "knowledge-binding-show", "knowledge.binding.resolve", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbResolveBinding}, request.Flags())
	}
}

func (f *httpFacade) knowledgeResourceAccess(w http.ResponseWriter, r *http.Request) {
	var request knowledgeResourceAccessRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	handler := "knowledge-access"
	run := inv.VerbResourceAccess
	if strings.TrimSpace(request.Operation) != "" || strings.TrimSpace(request.Implementation) != "" {
		handler = "knowledge-invoke"
		run = inv.VerbResourceInvoke
	}
	f.executeTyped(w, r, handler, "resource.access", inv.Command{Stage: invoke.StageGoverned, Run: run}, request.Flags())
}

func (f *httpFacade) projectionSync(w http.ResponseWriter, r *http.Request) {
	var request projectionSyncRequest
	if !decodeServiceRequest(w, r, &request) {
		return
	}
	flags := compactFlags(map[string]FlagValue{"repo": request.Repository, "commit": request.Commit, "ref": request.Ref})
	f.executeTyped(w, r, "operations-projection-sync", "projection.manage", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbIndexSync}, flags)
}

func (f *httpFacade) projectionNotify(w http.ResponseWriter, r *http.Request) {
	stream, err := requestJSONStream(r, inv.MaxServiceRequestBytes)
	if err != nil {
		invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "decode request: %v", err)))
		return
	}
	raw, err := io.ReadAll(stream)
	if err != nil {
		invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "decode request: %v", err)))
		return
	}
	if len(raw) > inv.MaxServiceRequestBytes {
		invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "request body is too large")))
		return
	}
	notice, err := index.ParseChangeNotice(raw)
	if err != nil {
		invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(err))
		return
	}
	flags := compactFlags(map[string]FlagValue{
		"repo": string(notice.Repository), "ref": notice.Ref, "source-revision": notice.SourceRevision,
	})
	if notice.Address != nil {
		flags["object"] = string(notice.Address.ObjectID)
		flags["aspect"] = notice.Address.AspectName
		if notice.Address.Kind != "" {
			flags["kind"] = string(notice.Address.Kind)
		}
	}
	f.executeTyped(w, r, "operations-projection-notice", "projection.manage", inv.Command{Stage: invoke.StageGoverned, Run: inv.VerbIndexNotify}, flags)
}
