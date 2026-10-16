package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"kc/identity"
	"kc/identity/identitytest"
	"kc/kernel"
	"kc/snapshot/access"
	snapshotclient "kc/snapshot/client"
	"kc/snapshot/service"
)

type testTransport struct {
	http *http.Client
}

func (t testTransport) Do(ctx context.Context, _ string, request *http.Request) (*http.Response, error) {
	return t.http.Do(request.WithContext(ctx))
}

type currentProofTransport struct {
	sync.Mutex
	http  *http.Client
	proof string
	calls int
}

func (t *currentProofTransport) Do(ctx context.Context, audience string, request *http.Request) (*http.Response, error) {
	t.Lock()
	proof := t.proof
	t.calls++
	t.Unlock()
	prepared := request.Clone(ctx)
	prepared.Header = request.Header.Clone()
	prepared.Header.Set("Authorization", proof)
	return t.http.Do(prepared)
}

var testIdentity = identitytest.Bearer("test", map[string]identity.HTTPIdentity{"Bearer alice": {Principal: "alice"}})

type testRules struct {
	sync.Mutex
	rules []access.Rule
}

func (s *testRules) ReadRules(context.Context) ([]access.Rule, error) {
	s.Lock()
	defer s.Unlock()
	return append([]access.Rule(nil), s.rules...), nil
}
func (s *testRules) UpdateRules(_ context.Context, update func([]access.Rule) ([]access.Rule, error)) error {
	s.Lock()
	defer s.Unlock()
	rules, err := update(s.rules)
	if err == nil {
		s.rules = rules
	}
	return err
}

func TestStandaloneSDKUsesCurrentProofAndRevocationAgainstSnapshotHandler(t *testing.T) {
	rules := &testRules{rules: []access.Rule{{Principal: "alice", Actions: []string{"admin.grants.read"}}}}
	handler := service.NewHandler(service.Config{Identity: service.IdentityOptions{Authenticator: testIdentity}, Acquire: func(context.Context, identity.HTTPIdentity, string) (service.Resources, func(), error) {
		return service.Resources{Access: access.NewService(rules)}, func() {}, nil
	}})
	server := httptest.NewServer(handler)
	defer server.Close()
	transport := &currentProofTransport{http: server.Client(), proof: "Bearer alice"}
	sdk, err := snapshotclient.New(snapshotclient.Config{BaseURL: server.URL, Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	if err := sdk.AdminService().Grants(context.Background(), snapshotclient.RequestOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := rules.UpdateRules(context.Background(), func([]access.Rule) ([]access.Rule, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if err := sdk.AdminService().Grants(context.Background(), snapshotclient.RequestOptions{}, nil); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("revoked grant still read: %v", err)
	}
	transport.Lock()
	transport.proof = "Bearer revoked"
	transport.Unlock()
	if err := sdk.AdminService().Admission(context.Background(), snapshotclient.RequestOptions{}, nil); kernel.CodeOf(err) != kernel.ErrUnauthenticated {
		t.Fatalf("old authentication reused: %v", err)
	}
	if transport.calls != 3 {
		t.Fatalf("request was cached or retried: %d", transport.calls)
	}
}

func TestStandaloneSDKRequiresExplicitConfiguredTransport(t *testing.T) {
	for _, config := range []snapshotclient.Config{
		{BaseURL: "https://service.test"},
		{BaseURL: "file:///tmp/server", Transport: testTransport{http: http.DefaultClient}},
		{BaseURL: "https://user:secret@service.test", Transport: testTransport{http: http.DefaultClient}},
		{BaseURL: "https://service.test?route=other", Transport: testTransport{http: http.DefaultClient}},
	} {
		if _, err := snapshotclient.New(config); err == nil {
			t.Fatal("SDK accepted invalid transport configuration")
		}
	}
}
