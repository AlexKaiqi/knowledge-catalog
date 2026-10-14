package httpjson_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"kc/internal/httpjson"
	"kc/kernel"
)

type transportFunc func(context.Context, string, *http.Request) (*http.Response, error)

func (f transportFunc) Do(ctx context.Context, audience string, request *http.Request) (*http.Response, error) {
	return f(ctx, audience, request)
}

func TestSharedJSONTransportBoundsResponsesAndDoesNotRetry(t *testing.T) {
	calls := 0
	transport := transportFunc(func(context.Context, string, *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", httpjson.MaxResponseBytes+1)))}, nil
	})
	var result any
	err := httpjson.Do(context.Background(), transport, "https://service.test", "GET", "/snapshot/v1/datasets", nil, "", &result)
	if kernel.CodeOf(err) != kernel.ErrTemporaryUnavailable || !strings.Contains(err.Error(), "exceeds") || calls != 1 {
		t.Fatalf("response limit or retry changed: %v, %d calls", err, calls)
	}
	canceled := transportFunc(func(context.Context, string, *http.Request) (*http.Response, error) {
		calls++
		return nil, context.Canceled
	})
	err = httpjson.Do(context.Background(), canceled, "https://service.test", "POST", "/snapshot/v1/datasets", struct{}{}, "", nil)
	if !errors.Is(err, context.Canceled) || calls != 2 {
		t.Fatalf("transport cancellation changed or retried: %v, %d calls", err, calls)
	}
}
