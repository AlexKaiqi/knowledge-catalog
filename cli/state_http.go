package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
	knowledgeserving "kc/knowledge/serving"
)

const maxStateRuntimeResponseBytes = 8 << 20

// HTTPStateLookup is the Knowledge Server adapter for one independent
// resource-access/v1 runtime service. The runtime may be another container;
// this adapter deliberately assumes neither a shared filesystem nor an
// in-process plugin host.
type HTTPStateLookup struct {
	endpoint string
	client   *http.Client
}

// NewHTTPStateLookup configures a network boundary, not a source-specific
// client. The selected runtime and call still come exclusively from the pinned
// Binding declaration supplied with each request.
func NewHTTPStateLookup(origin string, client *http.Client) (*HTTPStateLookup, error) {
	origin = strings.TrimSpace(origin)
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("resource access URL must be an http(s) origin")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("resource access URL must be an http(s) origin")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("resource access URL must not contain credentials, query, or fragment")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(u.Path, "/v1/access") {
		return nil, fmt.Errorf("resource access URL is the service origin, without /v1/access")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &HTTPStateLookup{endpoint: strings.TrimRight(u.String(), "/") + "/v1/access", client: client}, nil
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

func (h *HTTPStateLookup) LookupState(ctx context.Context, request knowledgeserving.StateLookupRequest) (knowledgeserving.StateObservation, error) {
	if h == nil || h.client == nil || h.endpoint == "" {
		return knowledgeserving.StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State runtime HTTP adapter is not configured")
	}
	operation, call, err := stateReadOperation(request.Binding)
	if err != nil {
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
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint, bytes.NewReader(body))
	if err != nil {
		return knowledgeserving.StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "create State runtime request: %v", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("X-Resource-Principal", request.Identity.Principal)
	if request.Identity.OnBehalfOf != "" {
		httpRequest.Header.Set("X-Resource-On-Behalf-Of", request.Identity.OnBehalfOf)
	}
	if request.RequestID != "" {
		httpRequest.Header.Set("X-Resource-Request-Id", request.RequestID)
	}
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

func stateReadOperation(binding reader.ResolvedBinding) (string, string, error) {
	for _, name := range []string{"lookup", "read"} {
		if operation, ok := binding.Operations[name]; ok && strings.TrimSpace(operation.Call) != "" {
			return name, operation.Call, nil
		}
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
