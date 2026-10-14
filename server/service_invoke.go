package server

// Typed invocation plumbing for the service facade: shared execution entry,
// invocation locking, identity resolution and request-body decoding. Route
// registration and per-route handlers stay in service_routes.go.

import (
	inv "kc/invocation"
	"kc/invoke"
	objectserving "kc/liveaccess"

	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"

	"kc/kernel"
	"kc/snapshot/dataset"
	snapshotservice "kc/snapshot/service"
)

func (f *httpFacade) executeTyped(w http.ResponseWriter, r *http.Request, name, action string, operation inv.Command, flags map[string]FlagValue) {
	for name := range r.URL.Query() {
		if strings.EqualFold(name, "catalog") {
			invoke.WriteInvoke(w, inv.ErrorResult(kernel.Fail(kernel.ErrUsageInvalid, "Catalog selection is not supported; this service has one registry")))
			return
		}
	}
	identity, ok := f.serviceIdentity(w, r)
	if !ok {
		return
	}
	if strings.HasPrefix(action, "object.") || action == "resource.access" || action == "dataset.resolve" {
		if err := inv.HoistTaskPinDefinition(flags); err != nil {
			invoke.WriteInvoke(w, inv.ErrorResult(err))
			return
		}
	}
	if strings.HasPrefix(action, "object.") || action == "resource.access" {
		if err := inv.NormalizeReferenceFlags(flags); err != nil {
			invoke.WriteInvoke(w, inv.ErrorResult(err))
			return
		}
		if err := inv.RejectMixedObjectBasis(flags); err != nil {
			invoke.WriteInvoke(w, inv.ErrorResult(err))
			return
		}
	}
	flags["home"] = f.home
	f.addIdentityFlags(flags, r, identity)
	addHTTPTraceFlags(flags, r)
	unlock := f.lockTypedInvocation(action)
	defer unlock()
	opened, err := f.readHomeForRequest()
	if err != nil {
		invoke.WriteInvoke(w, inv.ErrorResult(err))
		return
	}
	if err := inv.PrepareInventoryDiscovery(opened, action, flags); err != nil {
		invoke.WriteInvoke(w, inv.ErrorResult(err))
		return
	}
	ctx := r.Context()
	if !f.options.localAssertion() {
		ctx = objectserving.ContextWithCallerCredentials(ctx, objectserving.CallerCredentialsFromHeader(r.Header))
	}
	var access objectserving.StateLookup
	if opened.LiveAccess != nil && opened.LiveAccess.Configured() {
		access = opened.LiveAccess
	}
	invoke.WriteInvoke(w, inv.InvokeApplicationWithTelemetryAtHome(ctx, f.runtime, name, action, operation, flags, observeStateLookup(access, f.runtime), opened))
}

// lockTypedInvocation decides the invocation lock from the facade's serving
// mode. Deployment serving runs read-only actions on an immutable per-request
// view (executeApplicationOperation builds ws.ReadView with a private request
// journal), so independent fixed-basis reads may share the read lock.
// Component/fixture serving retains the legacy adoption path where every
// invocation installs its request journal on the shared Home (observeHome →
// SetJournal fans out to Writer, Reader, ControlPlane and registry);
// concurrent reads there would race on that shared state and could observe
// another request's stamp, so fixture-mode reads serialize with mutations.
// This is a process-safety boundary, not a Repository locking protocol;
// distributed writers still rely on authority CAS.
func (f *httpFacade) lockTypedInvocation(action string) func() {
	if f.deployment != nil && inv.TypedInvocationReadOnly(action) {
		f.invoke.RLock()
		return f.invoke.RUnlock
	}
	f.invoke.Lock()
	return f.invoke.Unlock
}

func (f *httpFacade) serviceIdentity(w http.ResponseWriter, r *http.Request) (inv.HTTPIdentity, bool) {
	caller, err := snapshotservice.AuthenticateRequest(r, f.snapshotIdentityOptions())
	if err == nil {
		recordHTTPIdentity(f.runtime, r.Context(), f.options, caller)
		return caller, true
	}
	status := http.StatusUnauthorized
	switch kernel.CodeOf(err) {
	case kernel.ErrTemporaryUnavailable, kernel.ErrPreconditionFailed:
		status = http.StatusServiceUnavailable
	case kernel.ErrForbidden:
		status = http.StatusForbidden
	default:
		w.Header().Set("WWW-Authenticate", `Bearer realm="kc"`)
	}
	invoke.WriteJSON(w, status, kernel.FaultJSON(err))
	return inv.HTTPIdentity{}, false
}

// requestJSONStream returns the request body as a bounded JSON stream,
// transparently decompressing Content-Encoding: gzip. The size cap applies to
// the decompressed bytes, so a small gzip payload cannot bypass the limit.
func requestJSONStream(r *http.Request, limit int) (io.Reader, error) {
	var body io.Reader = r.Body
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Content-Encoding")), "gzip") {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, err
		}
		body = gz
	}
	return io.LimitReader(body, int64(limit)+1), nil
}

// decodeServiceRequestBody decodes exactly one JSON object from the request
// body under the given decompressed-size cap.
func decodeServiceRequestBody(w http.ResponseWriter, r *http.Request, target any, limit int) bool {
	stream, err := requestJSONStream(r, limit)
	if err != nil {
		invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "decode request: %v", err)))
		return false
	}
	var definitionJSON bytes.Buffer
	if requestHasDatasetDefinition(target) {
		stream = io.TeeReader(stream, &definitionJSON)
	}
	decoder := json.NewDecoder(stream)
	decoder.DisallowUnknownFields()
	if err := kernel.DecodeJSON(decoder, target); err != nil {
		invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "decode request: %v", err)))
		return false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "request body must contain one JSON object")))
		return false
	}
	if definitionJSON.Len() > 0 {
		if err := dataset.ValidateMetadataJSON(definitionJSON.Bytes(), target); err != nil {
			invoke.WriteJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "decode request: %v", err)))
			return false
		}
	}
	return true
}

// Only inline Dataset envelopes need the original bytes for strict metadata
// validation. Writer bodies keep their existing streaming and number semantics.
func requestHasDatasetDefinition(target any) bool {
	shape := reflect.TypeOf(target)
	for shape != nil && shape.Kind() == reflect.Pointer {
		shape = shape.Elem()
	}
	if shape == nil || shape.Kind() != reflect.Struct {
		return false
	}
	field, exists := shape.FieldByName("Definition")
	return exists && field.Type == reflect.TypeOf((*dataset.Dataset)(nil))
}

func decodeServiceRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	return decodeServiceRequestBody(w, r, target, inv.MaxServiceRequestBytes)
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
