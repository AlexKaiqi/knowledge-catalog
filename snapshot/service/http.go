package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"kc/identity"
	"kc/kernel"
	"kc/snapshot/console"
)

const MaxRequestBytes = 8 << 20

// RequestMetadata carries diagnostic correlation only. These values are never
// identity proof, authorization input or downstream credentials.
type RequestMetadata struct {
	RequestID    string
	TraceID      string
	SpanID       string
	ParentSpanID string
	Dataset      string
	Repository   string
}

type requestMetadataKey struct{}

func ContextMetadata(ctx context.Context) RequestMetadata {
	metadata, _ := ctx.Value(requestMetadataKey{}).(RequestMetadata)
	return metadata
}

func requestContext(r *http.Request) context.Context {
	metadata := RequestMetadata{
		RequestID:    strings.TrimSpace(r.Header.Get("X-Kc-Request-Id")),
		TraceID:      strings.TrimSpace(r.Header.Get("X-Kc-Trace-Id")),
		SpanID:       strings.TrimSpace(r.Header.Get("X-Kc-Span-Id")),
		ParentSpanID: strings.TrimSpace(r.Header.Get("X-Kc-Parent-Span-Id")),
		Dataset:      r.PathValue("dataset"),
		Repository:   r.PathValue("repository"),
	}
	return context.WithValue(r.Context(), requestMetadataKey{}, metadata)
}

// requestWithDataset only carries a decoded scope for diagnostic and hook
// coordinates. The domain operation independently derives authorization.
func requestWithDataset(r *http.Request, datasetID string) *http.Request {
	r = r.Clone(r.Context())
	r.SetPathValue("dataset", datasetID)
	return r
}

func (h *handler) observeRequest(r *http.Request, caller identity.HTTPIdentity, operation string) (context.Context, func(any, error)) {
	ctx := requestContext(r)
	if h.config.Observe == nil {
		return ctx, nil
	}
	observed, finish := h.config.Observe(ctx, caller, operation)
	if observed != nil {
		ctx = observed
	}
	return ctx, finish
}

// Register mounts the Snapshot Store's complete console, Dataset, file and
// explicit access APIs. The embedding server supplies its own unrelated APIs.
func (h *handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/", h.nativeAPI)
	mux.HandleFunc("POST /api/v1/", h.nativeAPI)
	mux.HandleFunc("PUT /api/v1/", h.nativeAPI)
	mux.HandleFunc("DELETE /api/v1/", h.nativeAPI)
	mux.HandleFunc("PATCH /api/v1/repositories/{repository}/pulls/{pull}", h.nativeAPI)

	mux.Handle("GET /ui/", console.Handler())
	// The slash-less UI root is an explicit redirect, not the ServeMux's
	// implicit subtree redirect: Go 1.23 answers it with 301 and later
	// toolchains with 307, so the route pins the status itself (DEBT-UI-REDIRECT-STATUS-01).
	mux.HandleFunc("GET /ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("GET /identity/v1/auth", h.identityAuth)
	mux.HandleFunc("GET /identity/v1/whoami", h.identityWhoAmI)
	mux.HandleFunc("GET /identity/v1/admission", h.admissionShow)
	mux.HandleFunc("GET /admin/v1/grants", h.adminGrantList)
	mux.HandleFunc("POST /admin/v1/grants", h.adminGrantAdd)
	mux.HandleFunc("PUT /admin/v1/grants/{grant}", h.adminGrantUpdate)
	mux.HandleFunc("DELETE /admin/v1/grants/{grant}", h.adminGrantRemove)
	h.registerDatasetRoutes(mux)
	h.registerInventoryRoutes(mux)
	h.registerFileRoutes(mux)
}

func (h *handler) withResources(w http.ResponseWriter, r *http.Request, operation string, run func(context.Context, identity.HTTPIdentity, Resources) (any, error)) {
	for key := range r.URL.Query() {
		if strings.EqualFold(key, "catalog") {
			writeError(w, kernel.Fail(kernel.ErrUsageInvalid, "Catalog selection is not supported; this service has one registry"))
			return
		}
	}
	caller, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	ctx, finish := h.observeRequest(r, caller, operation)
	var result any
	var err error
	if finish != nil {
		defer func() {
			if cause := recover(); cause != nil {
				finish(nil, fmt.Errorf("snapshot store request panicked"))
				panic(cause)
			}
			finish(result, err)
		}()
	}
	if h.config.Acquire == nil {
		err = kernel.Fail(kernel.ErrPreconditionFailed, "Snapshot Store resources are not configured")
		writeError(w, err)
		return
	}
	resources, release, err := h.config.Acquire(ctx, caller, operation)
	if release != nil {
		defer release()
	}
	if err == nil {
		resources.around = h.config.Around
		result, err = run(ctx, caller, resources)
		if err == nil {
			writeJSON(w, http.StatusOK, result)
			return
		}
	}
	writeError(w, err)
}

func (resources Resources) runOperation(ctx context.Context, caller identity.HTTPIdentity, operation string, run func() (any, error)) (any, error) {
	if resources.around == nil {
		return run()
	}
	called := false
	var result any
	var err error
	_, hookErr := resources.around(ctx, caller, operation, func() (any, error) {
		if !called {
			called = true
			result, err = run()
		}
		return result, err
	})
	if called {
		// The hook cannot erase a domain failure or change an accepted result.
		// Post-hook delivery failures are owned by the embedding hook system.
		return result, err
	}
	if hookErr != nil {
		return nil, hookErr
	}
	return nil, kernel.Fail(kernel.ErrPreconditionFailed, "request hook did not execute the Snapshot Store operation")
}

func requestBytes(r *http.Request, limit int) ([]byte, error) {
	var stream io.Reader = r.Body
	if stream == nil {
		stream = strings.NewReader("")
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Content-Encoding")), "gzip") {
		reader, err := gzip.NewReader(stream)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		stream = reader
	}
	raw, err := io.ReadAll(io.LimitReader(stream, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > limit {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "request body is too large")
	}
	return raw, nil
}

func decodeRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	return decodeRequestLimit(w, r, target, MaxRequestBytes)
}

func decodeRequestLimit(w http.ResponseWriter, r *http.Request, target any, limit int) bool {
	raw, err := requestBytes(r, limit)
	if err == nil {
		err = decodeRequestBytes(raw, target)
	}
	if err != nil {
		writeError(w, kernel.Fail(kernel.ErrUsageInvalid, "decode request: %v", err))
		return false
	}
	return true
}

func decodeRequestBytes(raw []byte, target any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return kernel.Fail(kernel.ErrUsageInvalid, "request body must contain one JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := kernel.DecodeJSON(decoder, target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return kernel.Fail(kernel.ErrUsageInvalid, "request body must contain one JSON object")
	}
	return nil
}

func decodeEmptyRequest(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil || r.ContentLength == 0 {
		return true
	}
	var empty struct{}
	return decodeRequest(w, r, &empty)
}

func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, errorStatus(err), kernel.FaultJSON(err))
}

func errorStatus(err error) int {
	switch kernel.CodeOf(err) {
	case kernel.ErrUnauthenticated:
		return http.StatusUnauthorized
	case kernel.ErrForbidden:
		return http.StatusForbidden
	case kernel.ErrObjectRefUnresolved, kernel.ErrVersionUnresolved:
		return http.StatusNotFound
	case kernel.ErrTemporaryUnavailable:
		return http.StatusServiceUnavailable
	case kernel.ErrNonFastForward, kernel.ErrIdempotencyConflict, kernel.ErrPreconditionFailed,
		kernel.ErrEventIDConflict, kernel.ErrCandidateMoved,
		kernel.ErrRepositoryArchived,
		kernel.ErrSchemaIncompatible:
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		status = http.StatusInternalServerError
		raw, _ = json.Marshal(kernel.FaultJSON(kernel.Fail(kernel.ErrTemporaryUnavailable, "response encoding failed")))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(raw, '\n'))
}
