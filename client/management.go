package client

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"kc/catalog"
	"kc/knowledge"
	"kc/observability"
)

func resourceSegment(value string) string { return url.PathEscape(value) }

type CatalogService struct{ client *Client }

func (c *Client) CatalogService() CatalogService { return CatalogService{client: c} }

type WorkspaceDefinitionRequest struct {
	Workspace string                    `json:"workspace"`
	Revision  int                       `json:"revision"`
	Sources   []catalog.WorkspaceSource `json:"sources"`
}
type WorkspaceResolveRequest struct {
	Pin json.RawMessage `json:"pin,omitempty"`
}
type RepositoryRegisterRequest struct {
	Repository string `json:"repository"`
}

func (s CatalogService) Catalogs(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/catalog/v1/catalogs", nil, o, out)
}

func (s CatalogService) Show(ctx context.Context, catalogID string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/catalog/v1/catalogs/"+resourceSegment(catalogID), nil, o, out)
}
func (s CatalogService) Audit(ctx context.Context, catalogID string, limit int, o RequestOptions, out any) error {
	path := "/catalog/v1/catalogs/" + resourceSegment(catalogID) + "/audit"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	return s.client.doJSON(ctx, "GET", path, nil, o, out)
}
func (s CatalogService) Archive(ctx context.Context, catalogID string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/catalog/v1/catalogs/"+resourceSegment(catalogID)+"/archive", struct{}{}, o, out)
}
func (s CatalogService) Repositories(ctx context.Context, catalogID string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/catalog/v1/catalogs/"+resourceSegment(catalogID)+"/repositories", nil, o, out)
}
func (s CatalogService) RegisterRepository(ctx context.Context, catalogID string, q RepositoryRegisterRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/catalog/v1/catalogs/"+resourceSegment(catalogID)+"/repositories", q, o, out)
}
func (s CatalogService) ArchiveRepository(ctx context.Context, catalogID, repository string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/catalog/v1/catalogs/"+resourceSegment(catalogID)+"/repositories/"+resourceSegment(repository)+"/archive", struct{}{}, o, out)
}
func (s CatalogService) Workspaces(ctx context.Context, catalogID string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/catalog/v1/catalogs/"+resourceSegment(catalogID)+"/workspaces", nil, o, out)
}
func (s CatalogService) DefineWorkspace(ctx context.Context, catalogID string, q WorkspaceDefinitionRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/catalog/v1/catalogs/"+resourceSegment(catalogID)+"/workspaces", q, o, out)
}
func (s CatalogService) ResolveDefinition(ctx context.Context, catalogID string, q WorkspaceDefinitionRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/catalog/v1/catalogs/"+resourceSegment(catalogID)+"/workspaces/resolve", q, o, out)
}
func (s CatalogService) Workspace(ctx context.Context, catalogID, workspace string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/catalog/v1/catalogs/"+resourceSegment(catalogID)+"/workspaces/"+resourceSegment(workspace), nil, o, out)
}
func (s CatalogService) RetireWorkspace(ctx context.Context, catalogID, workspace string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/catalog/v1/catalogs/"+resourceSegment(catalogID)+"/workspaces/"+resourceSegment(workspace)+"/retire", struct{}{}, o, out)
}
func (s CatalogService) ResolveWorkspace(ctx context.Context, catalogID, workspace string, q WorkspaceResolveRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/catalog/v1/catalogs/"+resourceSegment(catalogID)+"/workspaces/"+resourceSegment(workspace)+"/resolve", q, o, out)
}
func (s CatalogService) CheckWorkspace(ctx context.Context, catalogID, workspace string, q WorkspaceResolveRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/catalog/v1/catalogs/"+resourceSegment(catalogID)+"/workspaces/"+resourceSegment(workspace)+"/check", q, o, out)
}

type WriterService struct{ client *Client }

func (c *Client) WriterService() WriterService { return WriterService{client: c} }

type CommitRequest struct {
	CommandID string              `json:"commandId"`
	ChangeSet knowledge.ChangeSet `json:"changeSet"`
}
type ProposalRequest struct {
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

func (s WriterService) Commit(ctx context.Context, repository string, q CommitRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/writer/v1/repositories/"+resourceSegment(repository)+"/commits", q, o, out)
}
func (s WriterService) Head(ctx context.Context, repository, ref string, o RequestOptions, out any) error {
	path := "/writer/v1/repositories/" + resourceSegment(repository) + "/head"
	if strings.TrimSpace(ref) != "" {
		path += "?ref=" + url.QueryEscape(ref)
	}
	return s.client.doJSON(ctx, "GET", path, nil, o, out)
}
func (s WriterService) Proposal(ctx context.Context, repository string, q ProposalRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/writer/v1/repositories/"+resourceSegment(repository)+"/proposals", q, o, out)
}
func (s WriterService) Receipt(ctx context.Context, command string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/writer/v1/receipts/"+resourceSegment(command), nil, o, out)
}

type GovernanceService struct{ client *Client }

func (c *Client) GovernanceService() GovernanceService { return GovernanceService{client: c} }

type PreviewRequest struct {
	Catalog   string `json:"catalog,omitempty"`
	Workspace string `json:"workspace"`
	Proposal  string `json:"proposal"`
}
type ValidateRequest struct {
	Catalog string `json:"catalog,omitempty"`
	Preview string `json:"preview"`
}
type ValidationRequest struct {
	Catalog string `json:"catalog,omitempty"`
	Preview string `json:"preview"`
	Suite   string `json:"suite"`
	Outcome string `json:"outcome"`
}
type MergeRequest struct {
	Catalog    string `json:"catalog,omitempty"`
	Proposal   string `json:"proposal"`
	Preview    string `json:"preview"`
	Validation string `json:"validation,omitempty"`
}

func (s GovernanceService) Proposal(ctx context.Context, q ProposalRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/governance/v1/proposals", q, o, out)
}
func (s GovernanceService) Preview(ctx context.Context, q PreviewRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/governance/v1/previews", q, o, out)
}
func (s GovernanceService) Validate(ctx context.Context, q ValidateRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/governance/v1/previews:validate", q, o, out)
}
func (s GovernanceService) RecordValidation(ctx context.Context, q ValidationRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/governance/v1/validations", q, o, out)
}
func (s GovernanceService) Merge(ctx context.Context, q MergeRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/governance/v1/proposals:merge", q, o, out)
}

type AdminService struct{ client *Client }

func (c *Client) AdminService() AdminService { return AdminService{client: c} }

type GrantRequest struct {
	Principal  string   `json:"principal"`
	Actions    []string `json:"actions"`
	Repository string   `json:"repository,omitempty"`
	Catalog    string   `json:"catalog,omitempty"`
	Ref        string   `json:"ref,omitempty"`
	Object     string   `json:"object,omitempty"`
	Aspect     string   `json:"aspect,omitempty"`
	Workspace  string   `json:"workspace,omitempty"`
}

func (s AdminService) AddGrant(ctx context.Context, q GrantRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/admin/v1/grants", q, o, out)
}
func (s AdminService) Grants(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/admin/v1/grants", nil, o, out)
}
func (s AdminService) RemoveGrant(ctx context.Context, id string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/admin/v1/grants/"+resourceSegment(id)+"/remove", struct{}{}, o, out)
}

type PolicyBindingRequest struct {
	On         string   `json:"on"`
	Phase      string   `json:"phase,omitempty"`
	Repository string   `json:"repository,omitempty"`
	Catalog    string   `json:"catalog,omitempty"`
	Run        string   `json:"run,omitempty"`
	URL        string   `json:"url,omitempty"`
	Require    []string `json:"require,omitempty"`
}
type AuditQueryRequest struct {
	Principal  string `json:"principal,omitempty"`
	OnBehalfOf string `json:"onBehalfOf,omitempty"`
	Action     string `json:"action,omitempty"`
	TraceID    string `json:"traceId,omitempty"`
	Repository string `json:"repository,omitempty"`
	Object     string `json:"object,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}
type RefineQueryRequest struct {
	EvidenceID string `json:"evidenceId,omitempty"`
	TraceID    string `json:"traceId,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	Outcome    string `json:"outcome,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}
type RetrievalQueryRequest struct {
	EvidenceID string `json:"evidenceId,omitempty"`
	TraceID    string `json:"traceId,omitempty"`
	Operator   string `json:"operator,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Outcome    string `json:"outcome,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}
type FeedbackRequest struct {
	Workspace           string                          `json:"workspace"`
	TraceID             string                          `json:"traceId"`
	Outcome             string                          `json:"outcome"`
	Message             string                          `json:"message,omitempty"`
	RetrievalEvidenceID string                          `json:"retrievalEvidenceId,omitempty"`
	RefineEvidenceID    string                          `json:"refineEvidenceId,omitempty"`
	Answer              string                          `json:"answer,omitempty"`
	SelectedRefs        []knowledge.KnowledgeRef        `json:"selectedRefs,omitempty"`
	IdealGroups         []observability.RefineRankGroup `json:"idealGroups,omitempty"`
}

func (s OperationsService) DescribeProjection(ctx context.Context, q ProjectionSyncRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/projections:describe", q, o, out)
}
func (s OperationsService) DescribeAccessSpec(ctx context.Context, q ProjectionSyncRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/access-specs:describe", q, o, out)
}
func (s OperationsService) AddHook(ctx context.Context, q PolicyBindingRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/hooks", q, o, out)
}
func (s OperationsService) Hooks(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/operations/v1/hooks", nil, o, out)
}
func (s OperationsService) RemoveHook(ctx context.Context, id string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/hooks/"+resourceSegment(id)+"/remove", struct{}{}, o, out)
}
func (s OperationsService) AddGate(ctx context.Context, q PolicyBindingRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/gates", q, o, out)
}
func (s OperationsService) Gates(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/operations/v1/gates", nil, o, out)
}
func (s OperationsService) RemoveGate(ctx context.Context, id string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/gates/"+resourceSegment(id)+"/remove", struct{}{}, o, out)
}
func (s OperationsService) AccessLog(ctx context.Context, q AuditQueryRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/access-log:query", q, o, out)
}
func (s OperationsService) Trace(ctx context.Context, traceID string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/traces:get", map[string]string{"traceId": traceID}, o, out)
}
func (s OperationsService) Hitmap(ctx context.Context, q AuditQueryRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/hitmap:query", q, o, out)
}
func (s OperationsService) RefineLog(ctx context.Context, q RefineQueryRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/refine-log:query", q, o, out)
}
func (s OperationsService) RetrievalLog(ctx context.Context, q RetrievalQueryRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/retrieval-log:query", q, o, out)
}
func (s OperationsService) RetrievalTraining(ctx context.Context, q RetrievalQueryRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/retrieval-training:query", q, o, out)
}
func (s OperationsService) RerankTraining(ctx context.Context, q RefineQueryRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/rerank-training:query", q, o, out)
}
func (s OperationsService) Feedback(ctx context.Context, q FeedbackRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/feedback", q, o, out)
}
