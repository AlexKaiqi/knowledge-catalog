package plugin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
	kcclient "kc/client"
	"kc/identity"
	"kc/kernel"
	"kc/liveaccess"
	objectmodel "kc/object"
	"kc/server"
)

type acceptanceAuthenticator struct{}

func (acceptanceAuthenticator) Name() string { return "liveaccess-acceptance" }
func (acceptanceAuthenticator) Authenticate(_ context.Context, header http.Header) (identity.HTTPIdentity, error) {
	for _, principal := range []string{"operator", "reader", "auditor", "alice", "bob"} {
		if header.Get("Authorization") == "Bearer "+principal+"-proof" {
			return identity.HTTPIdentity{Principal: principal, OnBehalfOf: "verified-" + principal, Provider: "liveaccess-acceptance"}, nil
		}
	}
	return identity.HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "fixture authenticator rejected proof")
}

// U16: a real verifier proves identity from Authorization; interleaved SDK
// requests cross Server and the default Repo-selected HTTP entry unchanged.
func TestServerLiveAccessInterleavedAuthenticationUsesVerifiedCurrentCaller(t *testing.T) {
	type received struct {
		header http.Header
		body   struct {
			Binding struct {
				Address objectmodel.Address `json:"address"`
			} `json:"binding"`
			Identity liveaccess.StateRuntimeIdentity `json:"identity"`
		}
	}
	entered := make(chan received, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var seen sync.Map
	var mu sync.Mutex
	var requests []received
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request received
		request.header = r.Header.Clone()
		if err := json.NewDecoder(r.Body).Decode(&request.body); err != nil {
			t.Error(err)
		}
		mu.Lock()
		requests = append(requests, request)
		mu.Unlock()
		if _, loaded := seen.LoadOrStore(request.body.Identity.Principal, true); !loaded {
			entered <- request
			<-release
		}
		var value any = map[string]any{"value": 17}
		if request.body.Binding.Address.AspectName == "empty" {
			value = nil
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": value, "basis": objectmodel.ObservationBasis{BindingGeneration: "source", Consistency: objectmodel.ObservationRepeatable, ObservedAt: "2026-10-14T12:00:00Z"}})
	}))
	t.Cleanup(runtime.Close)
	f := newAcceptanceFixture(t, server.HTTPServerOptions{Authenticator: acceptanceAuthenticator{}})
	f.stop(t)
	f.config.LiveAccess.CredentialOrigins = []string{runtime.URL}
	f.writeConfig(t)
	f.start(t)
	f.publish(t, acceptanceR1, runtime.URL)
	results := make(chan error, 2)
	call := func(principal string, traceByte byte) {
		client, err := kcclient.New(kcclient.Config{BaseURL: f.server.URL, Authenticator: acceptanceClientAuthenticator{}})
		if err == nil {
			_, err = client.Login(context.Background(), kcclient.LoginRequest{Identity: kernel.IdentityContext{Principal: "spoofed-" + principal}, Authentication: kcclient.Authentication{Authorization: "Bearer " + principal + "-proof"}})
		}
		traceID, spanID := trace.TraceID{}, trace.SpanID{}
		traceID[15], spanID[7] = traceByte, traceByte
		ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled}))
		var result objectmodel.ObservedValue
		if err == nil {
			err = client.KnowledgeService().Read(ctx, kcclient.KnowledgeReadRequest{Reference: acceptanceReference(t, acceptanceR1, ""), Commit: string(f.commits[acceptanceR1])}, kcclient.RequestOptions{RequestID: principal + "-request"}, &result)
		}
		if err == nil && (len(result.Observations) != 4 || fmt.Sprint(result.Value.(map[string]any)["good"].(map[string]any)["value"]) != "17") {
			err = kernel.Fail(kernel.ErrCapabilityUnsatisfied, "actual authenticated value not adopted")
		}
		results <- err
	}
	go call("alice", 1)
	select {
	case first := <-entered:
		if first.body.Identity.Principal != "alice" {
			t.Fatal("first proof not Alice")
		}
	case <-time.After(5 * time.Second):
		releaseOnce.Do(func() { close(release) })
		t.Fatal("Alice did not reach actual source")
	}
	go call("bob", 2)
	select {
	case second := <-entered:
		if second.body.Identity.Principal != "bob" {
			t.Fatal("second proof not Bob")
		}
	case <-time.After(5 * time.Second):
		releaseOnce.Do(func() { close(release) })
		t.Fatal("Bob serialized behind Alice or lost identity")
	}
	releaseOnce.Do(func() { close(release) })
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 8 {
		t.Fatalf("unexpected source calls: %d", len(requests))
	}
	for _, request := range requests {
		principal := request.body.Identity.Principal
		byteID := byte(1)
		if principal == "bob" {
			byteID = 2
		} else if principal != "alice" {
			t.Fatalf("Server trusted client assertion: %#v", request.body.Identity)
		}
		traceID := trace.TraceID{}
		traceID[15] = byteID
		if request.header.Get("Authorization") != "Bearer "+principal+"-proof" || request.header.Get("X-Resource-Principal") != principal || request.header.Get("X-Resource-On-Behalf-Of") != "verified-"+principal || request.header.Get("X-Resource-Request-Id") != principal+"-request" || request.body.Identity.OnBehalfOf != "verified-"+principal || request.body.Identity.RequestID != principal+"-request" || request.body.Identity.TraceID != traceID.String() {
			t.Fatalf("current proof/verified caller/request trace crossed callers: %#v %#v", request.header, request.body.Identity)
		}
		raw, _ := json.Marshal(request.body)
		if strings.Contains(string(raw), "-proof") {
			t.Fatal("source authentication entered protocol body")
		}
	}
	logs, err := f.audit(t, "auditor", "alice-request")
	if err != nil || len(logs) != 1 {
		t.Fatalf("authenticated request evidence unavailable: %#v %v", logs, err)
	}
	raw, _ := json.Marshal(logs)
	if strings.Contains(string(raw), "-proof") {
		t.Fatal("current proof entered independent audit output")
	}
}
