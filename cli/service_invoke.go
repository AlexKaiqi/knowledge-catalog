package cli

// Typed invocation plumbing for the service facade: shared execution entry,
// invocation locking, identity resolution and request-body decoding. Route
// registration and per-route handlers stay in service_routes.go.

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"kc/kernel"
)

func (f *httpFacade) executeTyped(w http.ResponseWriter, r *http.Request, name, action string, operation command, flags map[string]FlagValue) {
	identity, ok := f.serviceIdentity(w, r)
	if !ok {
		return
	}
	if strings.HasPrefix(action, "knowledge.") || action == "resource.access" || action == "dataset.resolve" {
		if err := hoistTaskPinDefinition(flags); err != nil {
			writeInvoke(w, errorResult(err))
			return
		}
	}
	if strings.HasPrefix(action, "knowledge.") || action == "resource.access" {
		if err := rejectMixedKnowledgeBasis(flags); err != nil {
			writeInvoke(w, errorResult(err))
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
		writeInvoke(w, errorResult(err))
		return
	}
	if err := prepareCatalogDiscovery(opened, action, flags); err != nil {
		writeInvoke(w, errorResult(err))
		return
	}
	ctx := r.Context()
	if !f.options.localAssertion() {
		ctx = contextWithCallerCredentials(ctx, callerCredentialsFromHeader(r.Header))
	}
	writeInvoke(w, invokeApplicationWithTelemetryAtHome(ctx, f.runtime, name, action, operation, flags, observeStateLookup(f.options.StateLookup, f.runtime), opened))
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
		"knowledge.binding.resolve", "knowledge.access.describe", "resource.access", "dataset.resolve":
		return true
	default:
		return false
	}
}

func (f *httpFacade) serviceIdentity(w http.ResponseWriter, r *http.Request) (HTTPIdentity, bool) {
	as := strings.TrimSpace(r.Header.Get("X-Kc-As"))
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	onBehalf := strings.TrimSpace(r.Header.Get("X-Kc-On-Behalf-Of"))
	mode := f.options.authMode()
	if f.options.localAssertion() {
		if authorization != "" {
			writeJSON(w, http.StatusUnauthorized, kernel.FaultJSON(kernel.Fail(kernel.ErrUnauthenticated,
				"this Server is --auth local; it does not accept Authorization (pairing mismatch)")))
			return HTTPIdentity{}, false
		}
		if onBehalf != "" {
			writeHTTPForbidden(w, "onBehalfOf requires a trusted authenticator")
			return HTTPIdentity{}, false
		}
		if as == "" {
			writeJSON(w, http.StatusUnauthorized, kernel.FaultJSON(kernel.Fail(kernel.ErrUnauthenticated,
				"this Server is --auth local; send X-Kc-As only")))
			return HTTPIdentity{}, false
		}
		identity := HTTPIdentity{Principal: as}
		recordHTTPIdentity(f.runtime, r.Context(), f.options, identity)
		return identity, true
	}
	if as != "" && authorization == "" {
		writeJSON(w, http.StatusUnauthorized, kernel.FaultJSON(kernel.Fail(kernel.ErrUnauthenticated,
			"this Server is --auth %s; send Authorization only (X-Kc-As is a pairing mismatch)", mode)))
		return HTTPIdentity{}, false
	}
	if !f.options.authenticated() {
		writeJSON(w, http.StatusUnauthorized, kernel.FaultJSON(kernel.Fail(kernel.ErrUnauthenticated,
			"this Server is --auth %s; authentication is not configured", mode)))
		return HTTPIdentity{}, false
	}
	identity, ok := authenticateHTTPRequest(w, r, f.options)
	if !ok || !f.validateIdentityHeaders(w, r, identity) {
		return HTTPIdentity{}, false
	}
	recordHTTPIdentity(f.runtime, r.Context(), f.options, identity)
	return identity, true
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
		writeJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "decode request: %v", err)))
		return false
	}
	decoder := json.NewDecoder(stream)
	decoder.DisallowUnknownFields()
	if err := kernel.DecodeJSON(decoder, target); err != nil {
		writeJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "decode request: %v", err)))
		return false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		writeJSON(w, http.StatusBadRequest, kernel.FaultJSON(kernel.Fail(kernel.ErrUsageInvalid, "request body must contain one JSON object")))
		return false
	}
	return true
}

func decodeServiceRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	return decodeServiceRequestBody(w, r, target, maxServiceRequestBytes)
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
