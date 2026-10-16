// Package serving 的 origin 绑定 State/Resource 查找实现：resource-access/v1
// 的通用 HTTP 客户端。调用目标来自每个请求所钉 Binding 的取值描述 origin，
// 不存在服务级全局地址——这正是 B6「缝不自行构造目标」的准确含义；
// local 组合由此获得与 server 一致的合同（2026-09-29 复核恢复）。
package liveaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"kc/kernel"
	objectmodel "kc/object"
)

// StateRuntimeIdentity / ResourceOperationRequest 等：resource-access/v1 的
// wire 形状（自 invocation 下移，invocation 保留别名维持公开形状）。
type StateRuntimeIdentity struct {
	Principal    string `json:"principal"`
	OnBehalfOf   string `json:"onBehalfOf,omitempty"`
	RequestID    string `json:"requestId,omitempty"`
	TraceID      string `json:"traceId,omitempty"`
	SpanID       string `json:"spanId,omitempty"`
	ParentSpanID string `json:"parentSpanId,omitempty"`
}
type ResourceDescriptorCoordinate struct {
	ObjectID   objectmodel.ObjectID `json:"objectId"`
	Repository kernel.RepositoryID  `json:"repository"`
	Commit     kernel.CommitID      `json:"commit"`
}
type ResourceOperationRequest struct {
	Descriptor        ResourceDescriptorCoordinate `json:"descriptor"`
	Implementation    *objectmodel.Address         `json:"implementation,omitempty"`
	DeclarationDigest kernel.Digest                `json:"declarationDigest,omitempty"`
	Runtime           string                       `json:"runtime"`
	Protocol          string                       `json:"protocol"`
	Origin            string                       `json:"-"`
	Operation         string                       `json:"operation"`
	Call              string                       `json:"call"`
	Input             any                          `json:"input"`
	Identity          StateRuntimeIdentity         `json:"identity"`
}
type ResourceOperationAccessor interface {
	AccessResource(context.Context, ResourceOperationRequest) (any, error)
}

const maxStateRuntimeResponseBytes = 8 << 20

// HTTPStateLookup is the Object Server adapter for resource-access/v1.
// Each request posts to the fixed ResourceDescriptor origin adopted with
// the unit's valueSource. The adapter does not pin a server-wide URL.
type HTTPStateLookup struct {
	client            *http.Client
	credentialOrigins map[string]bool
}

// NewHTTPStateLookup builds a generic HTTP client. The call target comes from
// the pinned descriptor origin on each request.
func NewHTTPStateLookup(client *http.Client) *HTTPStateLookup {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &HTTPStateLookup{client: client, credentialOrigins: map[string]bool{}}
}

func stateLookupOrigin(request StateLookupRequest) string {
	if origin := strings.TrimSpace(request.Origin); origin != "" {
		return origin
	}
	return strings.TrimSpace(request.Binding.Origin)
}

type stateRuntimeBinding struct {
	Repository        kernel.RepositoryID     `json:"repository"`
	DeclarationCommit kernel.CommitID         `json:"declarationCommit"`
	Address           objectmodel.Address     `json:"address"`
	DeclarationDigest kernel.Digest           `json:"declarationDigest"`
	Mode              objectmodel.BindingMode `json:"mode"`
	DescriptorRef     objectmodel.ObjectID    `json:"descriptorRef,omitempty"`
	DescriptorDigest  kernel.Digest           `json:"descriptorDigest,omitempty"`
}

type stateRuntimeRequest struct {
	Binding   stateRuntimeBinding  `json:"binding"`
	Runtime   string               `json:"runtime"`
	Protocol  string               `json:"protocol"`
	Operation string               `json:"operation"`
	Call      string               `json:"call"`
	Input     map[string]any       `json:"input"`
	SchemaRef string               `json:"schemaRef,omitempty"`
	Identity  StateRuntimeIdentity `json:"identity"`
}

type stateRuntimeResponse struct {
	Value json.RawMessage              `json:"value"`
	Basis objectmodel.ObservationBasis `json:"basis"`
}

// AccessResource invokes one operation declared by a pinned
// ResourceDescriptor. The operation name and call come from Canonical
// object; credentials and source-specific behavior remain in the runtime.
func (h *HTTPStateLookup) AccessResource(ctx context.Context, request ResourceOperationRequest) (any, error) {
	if h == nil || h.client == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "resource runtime HTTP adapter is not configured")
	}
	endpoint, err := objectmodel.ResourceAccessEndpoint(request.Origin)
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
	response, err := h.perform(ctx, httpRequest)
	if err != nil {
		if kernel.AsIngress(err) != nil {
			return nil, err
		}
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
		return nil, SafeFailure(ctx, runtimeHTTPError("resource runtime", response.StatusCode, raw))
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&value); err != nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "resource runtime returned invalid JSON: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "resource runtime returned more than one JSON value")
	}
	if err := CheckCredentialDisclosure(ctx, value); err != nil {
		return nil, err
	}
	return value, nil
}

func (h *HTTPStateLookup) LookupState(ctx context.Context, request StateLookupRequest) (StateObservation, error) {
	if h == nil || h.client == nil {
		return StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State runtime HTTP adapter is not configured")
	}
	endpoint, err := objectmodel.ResourceAccessEndpoint(stateLookupOrigin(request))
	if err != nil {
		return StateObservation{}, err
	}
	operation, call, err := stateReadOperation(request.Binding)
	if err != nil {
		return StateObservation{}, err
	}
	if err := requireResourceAccessPrincipal(request.Identity.Principal); err != nil {
		return StateObservation{}, err
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
		Identity: StateRuntimeIdentity{
			Principal: request.Identity.Principal, OnBehalfOf: request.Identity.OnBehalfOf,
			RequestID: request.RequestID, TraceID: request.Trace.TraceID,
			SpanID: request.Trace.SpanID, ParentSpanID: request.Trace.ParentSpanID,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "encode State runtime request: %v", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "create State runtime request: %v", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	applyResourceAccessIdentity(httpRequest, ctx, request.Identity.Principal, request.Identity.OnBehalfOf, request.RequestID)
	response, err := h.perform(ctx, httpRequest)
	if err != nil {
		if kernel.AsIngress(err) != nil {
			return StateObservation{}, err
		}
		return StateObservation{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "State runtime request failed: %v", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxStateRuntimeResponseBytes+1))
	if err != nil {
		return StateObservation{}, kernel.Fail(kernel.ErrTemporaryUnavailable, "read State runtime response: %v", err)
	}
	if len(raw) > maxStateRuntimeResponseBytes {
		return StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State runtime response exceeds %d bytes", maxStateRuntimeResponseBytes)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return StateObservation{}, SafeFailure(ctx, runtimeHTTPError("State runtime", response.StatusCode, raw))
	}
	var envelope stateRuntimeResponse
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&envelope); err != nil {
		return StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State runtime returned invalid JSON: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State runtime returned more than one JSON value")
	}
	if len(envelope.Value) == 0 {
		return StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State runtime response requires value (use null for an observed null)")
	}
	var value any
	if err := json.Unmarshal(envelope.Value, &value); err != nil {
		return StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "State runtime value is invalid JSON: %v", err)
	}
	observation := StateObservation{Value: value, Basis: envelope.Basis}
	if err := CheckCredentialDisclosure(ctx, observation); err != nil {
		return StateObservation{}, err
	}
	return observation, nil
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
}

func stateReadOperation(binding ResolvedBinding) (string, string, error) {
	for _, name := range []string{"lookup", "read"} {
		if operation, ok := binding.Operations[name]; ok && strings.TrimSpace(operation.Call) != "" {
			return name, operation.Call, nil
		}
	}
	if len(binding.Operations) == 0 {
		return "lookup", "lookup", nil
	}
	return "", "", kernel.Fail(kernel.ErrCapabilityUnsatisfied,
		"State Binding at %s must declare a lookup or read operation for ordinary READ", objectmodel.AddressKey(binding.Address))
}

// runtimeHTTPError maps one source failure. A source error code that names a
// stable public failure is kept, so a source that does not know the unit says
// so instead of looking like an empty value; transport-shaped codes stay local.
func runtimeHTTPError(subject string, status int, raw []byte) error {
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
		return kernel.Fail(kernel.ErrForbidden, "%s denied access: %s", subject, message)
	}
	switch code {
	case kernel.ErrUsageInvalid, kernel.ErrPreconditionFailed, kernel.ErrObjectRefUnresolved,
		kernel.ErrCapabilityUnsatisfied, kernel.ErrForbidden, kernel.ErrUnauthenticated:
		return kernel.Fail(code, "%s: %s", subject, message)
	}
	if status >= 400 && status < 500 && status != http.StatusTooManyRequests && status != http.StatusRequestTimeout {
		return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "%s cannot satisfy the request: %s", subject, message)
	}
	return kernel.Fail(kernel.ErrTemporaryUnavailable, "%s returned HTTP %d: %s", subject, status, message)
}
