package client

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/observability"
	"kc/snapshot/dataset"
)

func resourceSegment(value string) string { return url.PathEscape(value) }

type SnapshotService struct{ client *Client }

func (c *Client) SnapshotService() SnapshotService { return SnapshotService{client: c} }

type KnowledgeSetRequest struct {
	Dataset  string                       `json:"dataset"`
	Revision int                          `json:"revision"`
	Sources  []dataset.KnowledgeSetSource `json:"sources"`
	Metadata map[string]string            `json:"metadata,omitempty"`
}
type KnowledgeSetResolveRequest struct {
	Pin                json.RawMessage `json:"pin,omitempty"`
	InventoryDiscovery bool            `json:"inventoryDiscovery,omitempty"`
}
type RepositoryAttachRequest struct {
	Repository string `json:"repository"`
}
type RepositoryCreateRequest struct {
	Repository string `json:"repository"`
	CommandID  string `json:"commandId"`
}

func (s SnapshotService) Show(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/snapshot/v1/inventory", nil, o, out)
}
func (s SnapshotService) Audit(ctx context.Context, limit int, continuation string, o RequestOptions, out any) error {
	query := url.Values{}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	if continuation != "" {
		query.Set("continuation", continuation)
	}
	path := "/snapshot/v1/audit"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	return s.client.doJSON(ctx, "GET", path, nil, o, out)
}
func (s SnapshotService) Repositories(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/snapshot/v1/repositories", nil, o, out)
}
func (s SnapshotService) AttachRepository(ctx context.Context, q RepositoryAttachRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/repositories", q, o, out)
}
func (s SnapshotService) CreateRepository(ctx context.Context, q RepositoryCreateRequest, o RequestOptions, out any) error {
	if strings.TrimSpace(q.Repository) == "" || strings.TrimSpace(q.CommandID) == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "repository creation requires explicit repository and commandId")
	}
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/repositories:create", q, o, out)
}
func (s SnapshotService) ArchiveRepository(ctx context.Context, repository string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/repositories/"+resourceSegment(repository)+"/archive", struct{}{}, o, out)
}
func (s SnapshotService) DetachRepository(ctx context.Context, repository string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "DELETE", "/snapshot/v1/repositories/"+resourceSegment(repository), nil, o, out)
}
func (s SnapshotService) KnowledgeSets(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/snapshot/v1/datasets", nil, o, out)
}
func (s SnapshotService) DefineKnowledgeSet(ctx context.Context, q KnowledgeSetRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/datasets", q, o, out)
}
func (s SnapshotService) ResolveDefinition(ctx context.Context, q KnowledgeSetRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/datasets:resolve", q, o, out)
}
func (s SnapshotService) KnowledgeSet(ctx context.Context, setID string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/snapshot/v1/datasets/"+resourceSegment(setID), nil, o, out)
}
func (s SnapshotService) RetireKnowledgeSet(ctx context.Context, workspace string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/datasets/"+resourceSegment(workspace)+"/retire", struct{}{}, o, out)
}
func (s SnapshotService) ResolveKnowledgeSet(ctx context.Context, setID string, q KnowledgeSetResolveRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/datasets/"+resourceSegment(setID)+"/resolve", q, o, out)
}
func (s SnapshotService) CheckKnowledgeSet(ctx context.Context, setID string, q KnowledgeSetResolveRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/snapshot/v1/datasets/"+resourceSegment(setID)+"/check", q, o, out)
}

type WriterService struct{ client *Client }

func (c *Client) WriterService() WriterService { return WriterService{client: c} }

type CommitRequest struct {
	CommandID string              `json:"commandId"`
	ChangeSet knowledge.ChangeSet `json:"changeSet"`
}
type ProposalRequest struct {
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
func (s WriterService) Receipt(ctx context.Context, command string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/writer/v1/receipts/"+resourceSegment(command), nil, o, out)
}

type GovernanceService struct{ client *Client }

func (c *Client) GovernanceService() GovernanceService { return GovernanceService{client: c} }

type PreviewRequest struct {
	Pin      json.RawMessage `json:"pin,omitempty"`
	Dataset  string          `json:"dataset,omitempty"`
	Proposal string          `json:"proposal"`
}
type ValidateRequest struct {
	Preview string `json:"preview"`
}
type ValidationRequest struct {
	Preview string `json:"preview"`
	Suite   string `json:"suite"`
	Outcome string `json:"outcome"`
}
type MergeRequest struct {
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
	Ref        string   `json:"ref,omitempty"`
	Object     string   `json:"object,omitempty"`
	Aspect     string   `json:"aspect,omitempty"`
	Dataset    string   `json:"dataset,omitempty"`
}

func (s AdminService) AddGrant(ctx context.Context, q GrantRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/admin/v1/grants", q, o, out)
}
func (s AdminService) Grants(ctx context.Context, o RequestOptions, out any) error {
	return s.GrantsMatching(ctx, GrantRequest{}, o, out)
}

// GrantsMatching reuses grant coordinates for inventory filters. Actions may
// contain one requested semantic action; this remains a list, not evaluation.
func (s AdminService) GrantsMatching(ctx context.Context, q GrantRequest, o RequestOptions, out any) error {
	if len(q.Actions) > 1 {
		return kernel.Fail(kernel.ErrUsageInvalid, "grant inventory accepts at most one action filter")
	}
	query := url.Values{}
	for key, value := range map[string]string{"principal": q.Principal, "repository": q.Repository, "dataset": q.Dataset, "ref": q.Ref, "object": q.Object, "aspect": q.Aspect} {
		if value != "" {
			query.Set(key, value)
		}
	}
	if len(q.Actions) == 1 {
		query.Set("action", q.Actions[0])
	}
	path := "/admin/v1/grants"
	if len(query) != 0 {
		path += "?" + query.Encode()
	}
	return s.client.doJSON(ctx, "GET", path, nil, o, out)
}
func (s AdminService) RemoveGrant(ctx context.Context, id string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "DELETE", "/admin/v1/grants/"+resourceSegment(id), nil, o, out)
}

type PolicyBindingRequest struct {
	On         string   `json:"on"`
	Phase      string   `json:"phase,omitempty"`
	Repository string   `json:"repository,omitempty"`
	Run        string   `json:"run,omitempty"`
	URL        string   `json:"url,omitempty"`
	Require    []string `json:"require,omitempty"`
}
type AuditQueryRequest struct {
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

func (s OperationsService) DescribeProjection(ctx context.Context, q ProjectionSyncRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/projections:describe", q, o, out)
}
func (s OperationsService) DescribeAccessSpec(ctx context.Context, q AccessSpecDescribeRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/access-specs:describe", q, o, out)
}
func (s OperationsService) AddHook(ctx context.Context, q PolicyBindingRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/hooks", q, o, out)
}
func (s OperationsService) Hooks(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/operations/v1/hooks", nil, o, out)
}
func (s OperationsService) RemoveHook(ctx context.Context, id string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "DELETE", "/operations/v1/hooks/"+resourceSegment(id), nil, o, out)
}
func (s OperationsService) AddGate(ctx context.Context, q PolicyBindingRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/gates", q, o, out)
}
func (s OperationsService) Gates(ctx context.Context, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "GET", "/operations/v1/gates", nil, o, out)
}
func (s OperationsService) RemoveGate(ctx context.Context, id string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "DELETE", "/operations/v1/gates/"+resourceSegment(id), nil, o, out)
}
func (s OperationsService) AccessLog(ctx context.Context, q AuditQueryRequest, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/access-log:query", q, o, out)
}
func (s OperationsService) Trace(ctx context.Context, traceID string, o RequestOptions, out any) error {
	return s.client.doJSON(ctx, "POST", "/operations/v1/traces:query", map[string]string{"traceId": traceID}, o, out)
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
