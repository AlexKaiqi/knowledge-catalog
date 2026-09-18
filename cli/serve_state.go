package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
)

const maxStateRuntimeResponseBytes = 8 << 20

// HTTPStateLookup is the Knowledge Server adapter for resource-access/v1.
// Each request posts to the origin declared on that Binding's Domain Schema
// (or ResourceDescriptor). The adapter does not pin a server-wide URL.
type HTTPStateLookup struct {
	client *http.Client
}

// NewHTTPStateLookup builds a generic HTTP client. The call target comes from
// the pinned Schema origin on each request.
func NewHTTPStateLookup(client *http.Client) *HTTPStateLookup {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &HTTPStateLookup{client: client}
}

func stateLookupOrigin(request knowledgeserving.StateLookupRequest) string {
	if origin := strings.TrimSpace(request.Origin); origin != "" {
		return origin
	}
	return strings.TrimSpace(request.Binding.Origin)
}

type stateRuntimeBinding struct {
	Repository        kernel.RepositoryID   `json:"repository"`
	DeclarationCommit kernel.CommitID       `json:"declarationCommit"`
	Address           knowledge.Address     `json:"address"`
	DeclarationDigest kernel.Digest         `json:"declarationDigest"`
	Mode              knowledge.BindingMode `json:"mode"`
	DescriptorRef     knowledge.ObjectID    `json:"descriptorRef,omitempty"`
	DescriptorDigest  kernel.Digest         `json:"descriptorDigest,omitempty"`
}

type stateRuntimeRequest struct {
	Binding   stateRuntimeBinding  `json:"binding"`
	Runtime   string               `json:"runtime"`
	Protocol  string               `json:"protocol"`
	Operation string               `json:"operation"`
	Call      string               `json:"call"`
	Input     map[string]any       `json:"input"`
	SchemaRef string               `json:"schemaRef,omitempty"`
	Identity  stateRuntimeIdentity `json:"identity"`
}

type stateRuntimeIdentity struct {
	Principal    string `json:"principal"`
	OnBehalfOf   string `json:"onBehalfOf,omitempty"`
	RequestID    string `json:"requestId,omitempty"`
	TraceID      string `json:"traceId,omitempty"`
	SpanID       string `json:"spanId,omitempty"`
	ParentSpanID string `json:"parentSpanId,omitempty"`
}

type stateRuntimeResponse struct {
	Value json.RawMessage            `json:"value"`
	Basis knowledge.ObservationBasis `json:"basis"`
}

type resourceDescriptorCoordinate struct {
	ObjectID   knowledge.ObjectID  `json:"objectId"`
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
}

type resourceOperationRequest struct {
	Descriptor resourceDescriptorCoordinate `json:"descriptor"`
	Runtime    string                       `json:"runtime"`
	Protocol   string                       `json:"protocol"`
	Origin     string                       `json:"-"`
	Operation  string                       `json:"operation"`
	Call       string                       `json:"call"`
	Input      any                          `json:"input"`
	Identity   stateRuntimeIdentity         `json:"identity"`
}

type resourceOperationAccessor interface {
	AccessResource(context.Context, resourceOperationRequest) (any, error)
}

// AccessResource invokes one operation declared by a pinned
// ResourceDescriptor. The operation name and call come from Canonical
// knowledge; credentials and source-specific behavior remain in the runtime.
func (h *HTTPStateLookup) AccessResource(ctx context.Context, request resourceOperationRequest) (any, error) {
	if h == nil || h.client == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "resource runtime HTTP adapter is not configured")
	}
	endpoint, err := knowledge.ResourceAccessEndpoint(request.Origin)
	if err != nil {
		return nil, err
	}
	if err := requireResourceAccessPrincipal(request.Identity.Principal); err != nil {
		return nil, err
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "encode resource operation request: %v", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "create resource operation request: %v", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	applyResourceAccessIdentity(httpRequest, ctx, request.Identity.Principal, request.Identity.OnBehalfOf, request.Identity.RequestID)
	response, err := h.client.Do(httpRequest)
	if err != nil {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "resource runtime request failed: %v", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxStateRuntimeResponseBytes+1))
	if err != nil {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "read resource runtime response: %v", err)
	}
	if len(raw) > maxStateRuntimeResponseBytes {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "resource runtime response exceeds %d bytes", maxStateRuntimeResponseBytes)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, resourceRuntimeHTTPError(response.StatusCode, raw)
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&value); err != nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "resource runtime returned invalid JSON: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "resource runtime returned more than one JSON value")
	}
	return value, nil
}

func (h *HTTPStateLookup) LookupState(ctx context.Context, request knowledgeserving.StateLookupRequest) (knowledgeserving.StateObservation, error) {
	if h == nil || h.client == nil {
		return knowledgeserving.StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State runtime HTTP adapter is not configured")
	}
	endpoint, err := knowledge.ResourceAccessEndpoint(stateLookupOrigin(request))
	if err != nil {
		return knowledgeserving.StateObservation{}, err
	}
	operation, call, err := stateReadOperation(request.Binding)
	if err != nil {
		return knowledgeserving.StateObservation{}, err
	}
	if err := requireResourceAccessPrincipal(request.Identity.Principal); err != nil {
		return knowledgeserving.StateObservation{}, err
	}
	payload := stateRuntimeRequest{
		Binding: stateRuntimeBinding{
			Repository: request.Binding.Repository, DeclarationCommit: request.Binding.DeclarationCommit,
			Address: request.Binding.Address, DeclarationDigest: request.Binding.DeclarationDigest,
			Mode: request.Binding.Mode, DescriptorRef: request.Binding.DescriptorRef,
			DescriptorDigest: request.Binding.DescriptorDigest,
		},
		Runtime: request.Binding.Runtime, Protocol: request.Binding.Protocol,
		Operation: operation, Call: call, Input: map[string]any{}, SchemaRef: request.SchemaRef,
		Identity: stateRuntimeIdentity{
			Principal: request.Identity.Principal, OnBehalfOf: request.Identity.OnBehalfOf,
			RequestID: request.RequestID, TraceID: request.Trace.TraceID,
			SpanID: request.Trace.SpanID, ParentSpanID: request.Trace.ParentSpanID,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return knowledgeserving.StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "encode State runtime request: %v", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return knowledgeserving.StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "create State runtime request: %v", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	applyResourceAccessIdentity(httpRequest, ctx, request.Identity.Principal, request.Identity.OnBehalfOf, request.RequestID)
	response, err := h.client.Do(httpRequest)
	if err != nil {
		return knowledgeserving.StateObservation{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "State runtime request failed: %v", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxStateRuntimeResponseBytes+1))
	if err != nil {
		return knowledgeserving.StateObservation{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "read State runtime response: %v", err)
	}
	if len(raw) > maxStateRuntimeResponseBytes {
		return knowledgeserving.StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State runtime response exceeds %d bytes", maxStateRuntimeResponseBytes)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return knowledgeserving.StateObservation{}, stateRuntimeHTTPError(response.StatusCode, raw)
	}
	var envelope stateRuntimeResponse
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&envelope); err != nil {
		return knowledgeserving.StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State runtime returned invalid JSON: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return knowledgeserving.StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State runtime returned more than one JSON value")
	}
	if len(envelope.Value) == 0 {
		return knowledgeserving.StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State runtime response requires value (use null for an observed null)")
	}
	var value any
	if err := json.Unmarshal(envelope.Value, &value); err != nil {
		return knowledgeserving.StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State runtime value is invalid JSON: %v", err)
	}
	return knowledgeserving.StateObservation{Value: value, Basis: envelope.Basis}, nil
}

func requireResourceAccessPrincipal(principal string) error {
	if strings.TrimSpace(principal) == "" {
		return kernel.Fail(kernel.ErrUnauthenticated, "resource-access requires the caller's verified principal")
	}
	return nil
}

func applyResourceAccessIdentity(req *http.Request, ctx context.Context, principal, onBehalfOf, requestID string) {
	req.Header.Set("X-Resource-Principal", principal)
	if onBehalfOf != "" {
		req.Header.Set("X-Resource-On-Behalf-Of", onBehalfOf)
	}
	if requestID != "" {
		req.Header.Set("X-Resource-Request-Id", requestID)
	}
	applyCallerCredentials(req, callerCredentialsFromContext(ctx))
}

func stateReadOperation(binding reader.ResolvedBinding) (string, string, error) {
	for _, name := range []string{"lookup", "read"} {
		if operation, ok := binding.Operations[name]; ok && strings.TrimSpace(operation.Call) != "" {
			return name, operation.Call, nil
		}
	}
	if len(binding.Operations) == 0 {
		return "lookup", "lookup", nil
	}
	return "", "", kernel.Fail(kernel.ErrCapabilityUnsatisfied,
		"State Binding at %s must declare a lookup or read operation for ordinary READ", knowledge.AddressKey(binding.Address))
}

func stateRuntimeHTTPError(status int, raw []byte) error {
	message := strings.TrimSpace(http.StatusText(status))
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) == nil && strings.TrimSpace(envelope.Error.Message) != "" {
		message = strings.TrimSpace(envelope.Error.Message)
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return kernel.Fail(kernel.ErrForbidden, "State runtime denied access: %s", message)
	case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity:
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State runtime cannot satisfy the Binding: %s", message)
	default:
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "State runtime returned HTTP %d: %s", status, message)
	}
}

func resourceRuntimeHTTPError(status int, raw []byte) error {
	message := strings.TrimSpace(http.StatusText(status))
	code := kernel.ErrorCode("")
	var envelope struct {
		Error struct {
			Code    kernel.ErrorCode `json:"code"`
			Message string           `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) == nil {
		code = envelope.Error.Code
		if strings.TrimSpace(envelope.Error.Message) != "" {
			message = strings.TrimSpace(envelope.Error.Message)
		}
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return kernel.Fail(kernel.ErrForbidden, "resource runtime denied access: %s", message)
	}
	switch code {
	case kernel.ErrUsageInvalid, kernel.ErrPreconditionFailed, kernel.ErrKnowledgeRefUnresolved,
		kernel.ErrCapabilityUnsatisfied, kernel.ErrForbidden, kernel.ErrUnauthenticated:
		return kernel.Fail(code, "resource runtime: %s", message)
	}
	if status >= 400 && status < 500 {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "resource runtime cannot satisfy the operation: %s", message)
	}
	return kernel.Fail(kernel.ErrTemporaryUnavailable, "resource runtime returned HTTP %d: %s", status, message)
}
