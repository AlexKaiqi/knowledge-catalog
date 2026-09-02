package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kc/kernel"
)

type pairingAuthenticator struct {
	principal string
	onBehalf  string
}

func (a pairingAuthenticator) Name() string { return "taihu" }

func (a pairingAuthenticator) Authenticate(_ context.Context, headers http.Header) (HTTPIdentity, error) {
	if strings.TrimSpace(headers.Get("Authorization")) == "" {
		return HTTPIdentity{}, kernel.Fail(kernel.ErrUnauthenticated, "missing Authorization")
	}
	return HTTPIdentity{Principal: a.principal, OnBehalfOf: a.onBehalf, Provider: "taihu", Subject: a.principal}, nil
}

func TestIdentityAuthDiscovery(t *testing.T) {
	local := httptest.NewServer(HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{}))
	t.Cleanup(local.Close)
	status, body := pairingGET(t, local, "/identity/v1/auth", nil)
	if status != http.StatusOK || body["mode"] != "local" || body["localAssertion"] != true {
		t.Fatalf("local discovery: %d %#v", status, body)
	}

	product := httptest.NewServer(HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{
		Authenticator: pairingAuthenticator{principal: "taihu:1"},
	}))
	t.Cleanup(product.Close)
	status, body = pairingGET(t, product, "/identity/v1/auth", nil)
	if status != http.StatusOK || body["mode"] != "taihu" || body["localAssertion"] != false {
		t.Fatalf("taihu discovery: %d %#v", status, body)
	}
}

func TestLocalPairingRejectsAuthorizationAndOnBehalfOf(t *testing.T) {
	server := httptest.NewServer(HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{AuthMode: "local"}))
	t.Cleanup(server.Close)

	status, body := pairingGET(t, server, "/identity/v1/whoami", http.Header{"X-Kc-As": []string{"agent:dsh"}})
	if status != http.StatusOK || body["principal"] != "agent:dsh" {
		t.Fatalf("local whoami: %d %#v", status, body)
	}
	status, body = pairingGET(t, server, "/identity/v1/whoami", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("empty local identity: %d %#v", status, body)
	}
	status, body = pairingGET(t, server, "/identity/v1/whoami", http.Header{"Authorization": []string{"Bearer t"}})
	if status != http.StatusUnauthorized || !strings.Contains(fmtErr(body), "pairing mismatch") {
		t.Fatalf("local Authorization: %d %#v", status, body)
	}
	status, body = pairingGET(t, server, "/identity/v1/whoami", http.Header{
		"X-Kc-As":           []string{"agent:dsh"},
		"X-Kc-On-Behalf-Of": []string{"taihu:1"},
	})
	if status != http.StatusForbidden {
		t.Fatalf("local onBehalfOf: %d %#v", status, body)
	}
}

func TestProductPairingRejectsLocalAssertionAndMixedHeaders(t *testing.T) {
	server := httptest.NewServer(HTTPHandlerWithOptions(t.TempDir(), HTTPServerOptions{
		Authenticator: pairingAuthenticator{principal: "taihu:1"},
	}))
	t.Cleanup(server.Close)

	status, body := pairingGET(t, server, "/identity/v1/whoami", http.Header{"X-Kc-As": []string{"agent:dsh"}})
	if status != http.StatusUnauthorized || !strings.Contains(fmtErr(body), "pairing mismatch") {
		t.Fatalf("taihu X-Kc-As only: %d %#v", status, body)
	}
	status, body = pairingGET(t, server, "/identity/v1/whoami", http.Header{"Authorization": []string{"Bearer t"}})
	if status != http.StatusOK || body["principal"] != "taihu:1" {
		t.Fatalf("taihu Bearer: %d %#v", status, body)
	}
	status, body = pairingGET(t, server, "/identity/v1/whoami", http.Header{
		"Authorization": []string{"Bearer t"},
		"X-Kc-As":       []string{"agent:forged"},
	})
	if status != http.StatusForbidden {
		t.Fatalf("taihu mixed headers: %d %#v", status, body)
	}
}

func pairingGET(t *testing.T, server *httptest.Server, path string, headers http.Header) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	return resp.StatusCode, body
}

func fmtErr(body map[string]any) string {
	raw, _ := json.Marshal(body)
	return string(raw)
}
