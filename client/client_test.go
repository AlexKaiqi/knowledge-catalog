package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"kc/cli"
	"kc/client"
	"kc/kernel"
)

func TestPassThroughLoginCarriesIdentityAuthenticationAndTrace(t *testing.T) {
	seen := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		seen <- request.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"principal":"agent:catalog","onBehalfOf":"user:kai"}`)
	}))
	t.Cleanup(server.Close)

	kc, err := client.New(client.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := kc.Login(context.Background(), client.LoginRequest{
		Identity:       client.Identity{Principal: "agent:catalog", OnBehalfOf: "user:kai"},
		Authentication: client.Authentication{Authorization: "Bearer opaque-token"},
	})
	if err != nil || identity.Principal != "agent:catalog" {
		t.Fatalf("login: identity=%#v err=%v", identity, err)
	}

	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
		Remote:     false,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)
	var who map[string]any
	if err := kc.Invoke(ctx, client.Invocation{Verb: "whoami", RequestID: "request-1"}, &who); err != nil {
		t.Fatal(err)
	}
	header := <-seen
	if header.Get("Authorization") != "Bearer opaque-token" || header.Get("X-Kc-As") != "agent:catalog" || header.Get("X-Kc-On-Behalf-Of") != "user:kai" {
		t.Fatalf("identity/authentication not propagated: %#v", header)
	}
	if header.Get("X-Kc-Request-Id") != "request-1" || !strings.HasPrefix(header.Get("Traceparent"), "00-") {
		t.Fatalf("request/trace context not propagated: %#v", header)
	}
	if header.Get("Baggage") != "" {
		t.Fatalf("identity or credential must not enter baggage: %q", header.Get("Baggage"))
	}
}

func TestLogoutClearsCredentialsBeforeNextCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(server.Close)
	kc, err := client.New(client.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "user:alice"}}); err != nil {
		t.Fatal(err)
	}
	if err := kc.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := kc.Identity(context.Background()); err != nil || ok {
		t.Fatalf("identity remains after logout: ok=%v err=%v", ok, err)
	}
	request, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kc.Do(context.Background(), "external-system", request); kernel.CodeOf(err) != kernel.ErrUnauthenticated {
		t.Fatalf("logged-out call: %v", err)
	}
}

func TestAuthenticationIsNotJSONSerialized(t *testing.T) {
	raw, err := json.Marshal(client.Session{
		Identity:       client.Identity{Principal: "user:alice"},
		Authentication: client.Authentication{Authorization: "Bearer secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "Authorization") {
		t.Fatalf("credential escaped through JSON: %s", raw)
	}
}

func TestClientWorksWithLocalKCPassThroughFacade(t *testing.T) {
	home := t.TempDir()
	handler := cli.HTTPHandler(home)
	seenAuthorization := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		seenAuthorization <- request.Header.Get("Authorization")
		handler.ServeHTTP(w, request)
	}))
	t.Cleanup(server.Close)
	kc, err := client.New(client.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = kc.Login(context.Background(), client.LoginRequest{
		Identity:       client.Identity{Principal: "agent:test", OnBehalfOf: "user:test"},
		Authentication: client.Authentication{Authorization: "Opaque direct"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var who map[string]any
	if err := kc.Invoke(context.Background(), client.Invocation{Verb: "whoami"}, &who); err != nil {
		t.Fatal(err)
	}
	if authorization := <-seenAuthorization; authorization != "Opaque direct" {
		t.Fatalf("authorization was not carried to kc server: %q", authorization)
	}
	if who["principal"] != "agent:test" || who["onBehalfOf"] != "user:test" {
		t.Fatalf("whoami: %#v", who)
	}
}
