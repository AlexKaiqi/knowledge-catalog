package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"kc/identity"
	"kc/kernel"
)

func TestSnapshotJSONTransportRejectsUnknownTrailingAndOversizedDecodedBodies(t *testing.T) {
	for _, raw := range []string{`{"unknown":true}`, `{} {}`, `null`, `[]`, `{} trailing`, "{}" + strings.Repeat(" ", 63)} {
		var target struct{}
		request := httptest.NewRequest(http.MethodPost, "/admin/v1/grants", strings.NewReader(raw))
		response := httptest.NewRecorder()
		if decodeRequestLimit(response, request, &target, 64) || response.Code != http.StatusBadRequest {
			t.Fatalf("invalid JSON accepted: %q %d %s", raw, response.Code, response.Body.String())
		}
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, _ = writer.Write([]byte(`{}` + strings.Repeat(" ", 1000)))
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/admin/v1/grants", &compressed)
	request.Header.Set("Content-Encoding", "gzip")
	response := httptest.NewRecorder()
	var target struct{}
	if decodeRequestLimit(response, request, &target, 64) || response.Code != http.StatusBadRequest {
		t.Fatalf("gzip bypassed decompressed limit: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/admin/v1/grants", strings.NewReader(`{"ok":true}`))
	response = httptest.NewRecorder()
	var valid struct {
		OK bool `json:"ok"`
	}
	if !decodeRequest(response, request, &valid) || !valid.OK {
		t.Fatalf("valid JSON request failed: %s", response.Body.String())
	}
}

func TestSnapshotResourceLeasesAndObservationPreserveActualFailure(t *testing.T) {
	for _, acquireFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "operation-failure", true: "acquire-failure"}[acquireFailure], func(t *testing.T) {
			failure := kernel.Fail(kernel.ErrTemporaryUnavailable, "source offline")
			acquired, released, observed := 0, 0, 0
			metadata := RequestMetadata{RequestID: "request", TraceID: "trace", SpanID: "span", ParentSpanID: "parent"}
			handler := newHandler(Config{
				Identity: IdentityOptions{Local: true},
				Acquire: func(ctx context.Context, caller identity.HTTPIdentity, operation string) (Resources, func(), error) {
					acquired++
					if caller.Principal != "alice" || operation != "probe" || !reflect.DeepEqual(ContextMetadata(ctx), metadata) {
						t.Fatalf("lease context changed: %#v %s %#v", caller, operation, ContextMetadata(ctx))
					}
					var err error
					if acquireFailure {
						err = failure
					}
					return Resources{}, func() { released++ }, err
				},
				Observe: func(ctx context.Context, _ identity.HTTPIdentity, _ string) (context.Context, func(any, error)) {
					return ctx, func(result any, err error) {
						observed++
						if result != nil || !errors.Is(err, failure) {
							t.Fatalf("observer replaced actual failure: %#v %v", result, err)
						}
					}
				},
			})
			request := httptest.NewRequest(http.MethodGet, "/probe", nil)
			request.Header = http.Header{"X-Kc-As": {"alice"}, "X-Kc-Request-Id": {"request"}, "X-Kc-Trace-Id": {"trace"}, "X-Kc-Span-Id": {"span"}, "X-Kc-Parent-Span-Id": {"parent"}, "Cookie": {"credential-must-not-enter-metadata"}}
			response := httptest.NewRecorder()
			handler.withResources(response, request, "probe", func(context.Context, identity.HTTPIdentity, Resources) (any, error) {
				if acquireFailure {
					t.Fatal("operation ran after a failed acquire")
				}
				return nil, failure
			})
			if response.Code != http.StatusServiceUnavailable || acquired != 1 || released != 1 || observed != 1 {
				t.Fatalf("lease/observation failure: %d acquired=%d released=%d observed=%d", response.Code, acquired, released, observed)
			}
		})
	}
}

func TestSnapshotTransportPreservesDomainAndAuthenticationStatusDistinction(t *testing.T) {
	for _, check := range []struct {
		code   kernel.ErrorCode
		status int
	}{
		{kernel.ErrForbidden, http.StatusForbidden}, {kernel.ErrUnauthenticated, http.StatusUnauthorized},
		{kernel.ErrVersionUnresolved, http.StatusNotFound}, {kernel.ErrPreconditionFailed, http.StatusConflict},
		{kernel.ErrTemporaryUnavailable, http.StatusServiceUnavailable},
	} {
		w := httptest.NewRecorder()
		writeError(w, kernel.Fail(check.code, "failed"))
		if w.Code != check.status || !strings.Contains(w.Body.String(), string(check.code)) {
			t.Fatalf("domain error representation: %s %d %s", check.code, w.Code, w.Body.String())
		}
	}
	verifier := &transportVerifier{mode: "verified", err: kernel.Fail(kernel.ErrPreconditionFailed, "binding unavailable")}
	handler := newHandler(Config{Identity: IdentityOptions{Authenticator: verifier}})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/identity/v1/whoami", nil)
	r.Header.Set("Authorization", "Bearer proof")
	handler.identityWhoAmI(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable binding became a domain conflict: %d %s", w.Code, w.Body.String())
	}
}

func TestSnapshotAroundCannotRepeatReplaceOrEraseTheDomainOperation(t *testing.T) {
	ctx := context.Background()
	caller := identity.HTTPIdentity{Principal: "alice"}
	wanted := map[string]any{"published": "fixed-commit"}
	failure := kernel.Fail(kernel.ErrForbidden, "source scope denied")
	for _, domainErr := range []error{nil, failure} {
		calls := 0
		resources := Resources{around: func(_ context.Context, _ identity.HTTPIdentity, _ string, run func() (any, error)) (any, error) {
			_, _ = run()
			_, _ = run()
			return map[string]any{"published": "invented-commit"}, nil
		}}
		result, err := resources.runOperation(ctx, caller, "dataset-define", func() (any, error) {
			calls++
			return wanted, domainErr
		})
		if calls != 1 || !reflect.DeepEqual(result, wanted) || !errors.Is(err, domainErr) {
			t.Fatalf("hook changed the actual operation: calls=%d result=%#v err=%v", calls, result, err)
		}
	}
	denied := kernel.Fail(kernel.ErrHookDenied, "publication pre-hook rejected")
	resources := Resources{around: func(context.Context, identity.HTTPIdentity, string, func() (any, error)) (any, error) {
		return wanted, denied
	}}
	if result, err := resources.runOperation(ctx, caller, "dataset-define", func() (any, error) {
		t.Fatal("pre-hook refusal ran the operation")
		return nil, nil
	}); result != nil || !errors.Is(err, denied) {
		t.Fatalf("pre-hook failure did not remain authoritative: %#v %v", result, err)
	}
	resources.around = func(context.Context, identity.HTTPIdentity, string, func() (any, error)) (any, error) {
		return wanted, nil
	}
	if _, err := resources.runOperation(ctx, caller, "dataset-define", func() (any, error) {
		t.Fatal("omitted operation unexpectedly ran")
		return nil, nil
	}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("hook fabricated success without an operation: %v", err)
	}
}

func TestSnapshotLeaseAndObserverFinishWhenTheDomainPanics(t *testing.T) {
	released, observed := 0, 0
	handler := newHandler(Config{
		Identity: IdentityOptions{Local: true},
		Acquire: func(context.Context, identity.HTTPIdentity, string) (Resources, func(), error) {
			return Resources{}, func() { released++ }, nil
		},
		Observe: func(ctx context.Context, _ identity.HTTPIdentity, _ string) (context.Context, func(any, error)) {
			return ctx, func(result any, err error) {
				observed++
				if result != nil || err == nil {
					t.Fatalf("panicked operation was observed as success: %#v %v", result, err)
				}
			}
		},
	})
	r := httptest.NewRequest(http.MethodGet, "/probe", nil)
	r.Header.Set("X-Kc-As", "alice")
	defer func() {
		if cause := recover(); cause != "original-panic" || released != 1 || observed != 1 {
			t.Fatalf("panic cleanup changed: cause=%v released=%d observed=%d", cause, released, observed)
		}
	}()
	handler.withResources(httptest.NewRecorder(), r, "probe", func(context.Context, identity.HTTPIdentity, Resources) (any, error) { panic("original-panic") })
}
