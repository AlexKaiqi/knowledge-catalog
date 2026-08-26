package cli_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
)

type fixedAuthenticator struct {
	identity cli.HTTPIdentity
}

func (a fixedAuthenticator) Name() string { return "fixed" }

func (a fixedAuthenticator) Authenticate(context.Context, string) (cli.HTTPIdentity, error) {
	return a.identity, nil
}

func authenticatedRequest(t *testing.T, srv *httptest.Server, method, path string, body any, authorization, as string) (int, map[string]any) {
	return authenticatedRequestWithHeaders(t, srv, method, path, body, map[string]string{
		"Authorization": authorization,
		"X-Kc-As":       as,
	})
}

func authenticatedRequestWithHeaders(t *testing.T, srv *httptest.Server, method, path string, body any, headers map[string]string) (int, map[string]any) {
	t.Helper()
	if verb, ok := strings.CutPrefix(path, "/v1/"); ok {
		recordOperation(verb)
	}
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, srv.URL+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		if value != "" {
			req.Header.Set(name, value)
		}
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("decode %s: %s", err, raw)
		}
	}
	return res.StatusCode, payload
}

func errorCode(payload map[string]any) string {
	errObj, _ := payload["error"].(map[string]any)
	code, _ := errObj["code"].(string)
	return code
}

func TestHTTPFacadeAuthenticatesWithGitea(t *testing.T) {
	giteaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/user" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Header.Get("Authorization") {
		case "token root-token":
			_, _ = io.WriteString(w, `{"id":1,"login":"root","active":true,"is_admin":true}`)
		case "Bearer alice-token":
			_, _ = io.WriteString(w, `{"id":42,"login":"alice","active":true,"is_admin":false}`)
		case "token inactive-token":
			_, _ = io.WriteString(w, `{"id":9,"login":"former","active":false,"is_admin":false}`)
		default:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"message":"unauthorized"}`)
		}
	}))
	t.Cleanup(giteaServer.Close)

	authenticator, err := cli.NewGiteaAuthenticator(giteaServer.URL, giteaServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	srv := httptest.NewServer(cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{Authenticator: authenticator}))
	t.Cleanup(srv.Close)

	code, health := authenticatedRequest(t, srv, http.MethodGet, "/health", nil, "", "")
	if code != http.StatusOK || health["auth"] != "gitea" || health["home"] != nil {
		t.Fatalf("health %d %#v", code, health)
	}

	code, missing := authenticatedRequest(t, srv, http.MethodPost, "/v1/whoami", map[string]any{}, "", "")
	if code != http.StatusUnauthorized || errorCode(missing) != "UNAUTHENTICATED" {
		t.Fatalf("missing credentials %d %#v", code, missing)
	}
	code, invalid := authenticatedRequest(t, srv, http.MethodPost, "/v1/whoami", map[string]any{}, "token wrong", "")
	if code != http.StatusUnauthorized || errorCode(invalid) != "UNAUTHENTICATED" {
		t.Fatalf("invalid credentials %d %#v", code, invalid)
	}
	code, inactive := authenticatedRequest(t, srv, http.MethodPost, "/v1/whoami", map[string]any{}, "token inactive-token", "")
	if code != http.StatusUnauthorized || errorCode(inactive) != "UNAUTHENTICATED" {
		t.Fatalf("inactive credentials %d %#v", code, inactive)
	}

	code, who := authenticatedRequest(t, srv, http.MethodPost, "/v1/whoami", map[string]any{"as": "owner"}, "Bearer alice-token", "")
	if code != http.StatusOK || who["principal"] != "gitea:42" || who["provider"] != "gitea" || who["login"] != "alice" {
		t.Fatalf("whoami %d %#v", code, who)
	}
	code, spoof := authenticatedRequest(t, srv, http.MethodPost, "/v1/whoami", map[string]any{}, "Bearer alice-token", "owner")
	if code != http.StatusForbidden || errorCode(spoof) != "FORBIDDEN" {
		t.Fatalf("spoofed X-Kc-As %d %#v", code, spoof)
	}
	code, delegatedBody := authenticatedRequest(t, srv, http.MethodPost, "/v1/whoami", map[string]any{"on-behalf-of": "user:victim"}, "Bearer alice-token", "")
	if code != http.StatusForbidden || errorCode(delegatedBody) != "FORBIDDEN" {
		t.Fatalf("unverified JSON delegation %d %#v", code, delegatedBody)
	}
	code, delegatedHeader := authenticatedRequestWithHeaders(t, srv, http.MethodPost, "/v1/whoami", map[string]any{}, map[string]string{
		"Authorization":     "Bearer alice-token",
		"X-Kc-On-Behalf-Of": "user:victim",
	})
	if code != http.StatusForbidden || errorCode(delegatedHeader) != "FORBIDDEN" {
		t.Fatalf("unverified header delegation %d %#v", code, delegatedHeader)
	}

	code, deniedAdmin := authenticatedRequest(t, srv, http.MethodPost, "/v1/init", map[string]any{"catalog": "kr://acme/catalog"}, "Bearer alice-token", "")
	if code != http.StatusForbidden || errorCode(deniedAdmin) != "FORBIDDEN" {
		t.Fatalf("non-admin init %d %#v", code, deniedAdmin)
	}
	code, deniedOverlay := authenticatedRequest(t, srv, http.MethodPost, "/v1/overlay", map[string]any{}, "Bearer alice-token", "")
	if code != http.StatusForbidden || errorCode(deniedOverlay) != "FORBIDDEN" {
		t.Fatalf("non-admin overlay %d %#v", code, deniedOverlay)
	}
	code, initialized := authenticatedRequest(t, srv, http.MethodPost, "/v1/init", map[string]any{"catalog": "kr://acme/catalog"}, "token root-token", "")
	if code != http.StatusOK || initialized["catalog"] != "kr://acme/catalog" {
		t.Fatalf("admin init %d %#v", code, initialized)
	}
	repo := "kr://acme/public/reference"
	code, added := authenticatedRequest(t, srv, http.MethodPost, "/v1/repo-add", map[string]any{"repo": repo}, "token root-token", "")
	if code != http.StatusOK || added["repositoryId"] != repo {
		t.Fatalf("admin repo-add %d %#v", code, added)
	}

	putBody := map[string]any{
		"as":         "owner",
		"command-id": "auth-put-1",
		"repo":       repo,
		"object":     "Document:authenticated",
		"value":      map[string]any{"text": "verified"},
	}
	code, deniedPut := authenticatedRequest(t, srv, http.MethodPost, "/v1/put", putBody, "Bearer alice-token", "")
	if code != http.StatusForbidden || errorCode(deniedPut) != "FORBIDDEN" {
		t.Fatalf("JSON as must not bypass policy %d %#v", code, deniedPut)
	}
	code, granted := authenticatedRequest(t, srv, http.MethodPost, "/v1/allow", map[string]any{
		"principal": "gitea:42",
		"cmd":       "put",
		"repo":      repo,
	}, "token root-token", "")
	if code != http.StatusOK || granted["principal"] != "gitea:42" {
		t.Fatalf("admin allow %d %#v", code, granted)
	}
	code, put := authenticatedRequest(t, srv, http.MethodPost, "/v1/put", putBody, "Bearer alice-token", "")
	if code != http.StatusOK || put["result"] == nil {
		t.Fatalf("authenticated put %d %#v", code, put)
	}
	code, allowed := authenticatedRequest(t, srv, http.MethodPost, "/v1/allowed", map[string]any{
		"cmd":  "put",
		"repo": repo,
	}, "Bearer alice-token", "")
	if code != http.StatusOK || allowed["allow"] != true {
		t.Fatalf("self allowed %d %#v", code, allowed)
	}
	code, dumped := authenticatedRequest(t, srv, http.MethodPost, "/v1/allowed", map[string]any{}, "Bearer alice-token", "")
	if code != http.StatusForbidden || errorCode(dumped) != "FORBIDDEN" {
		t.Fatalf("rule dump must be admin-only %d %#v", code, dumped)
	}
	code, blob := authenticatedRequest(t, srv, http.MethodGet, "/v1/_blob?path=allow.json", nil, "Bearer alice-token", "")
	if code != http.StatusForbidden || errorCode(blob) != "FORBIDDEN" {
		t.Fatalf("server files must be admin-only %d %#v", code, blob)
	}
}

func TestHTTPFacadeAcceptsConfiguredGiteaAdmin(t *testing.T) {
	giteaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":42,"login":"alice","active":true,"is_admin":false}`)
	}))
	t.Cleanup(giteaServer.Close)
	authenticator, err := cli.NewGiteaAuthenticator(giteaServer.URL, giteaServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(cli.HTTPHandlerWithOptions(t.TempDir(), cli.HTTPServerOptions{
		Authenticator:   authenticator,
		AdminPrincipals: []string{"gitea:42"},
	}))
	t.Cleanup(srv.Close)
	code, body := authenticatedRequest(t, srv, http.MethodPost, "/v1/init", map[string]any{"catalog": "kr://acme/catalog"}, "Bearer anything", "")
	if code != http.StatusOK || body["catalog"] != "kr://acme/catalog" {
		t.Fatalf("configured admin %d %#v", code, body)
	}
}

func TestHTTPFacadeOnlyAcceptsVerifiedDelegation(t *testing.T) {
	srv := httptest.NewServer(cli.HTTPHandlerWithOptions(t.TempDir(), cli.HTTPServerOptions{
		Authenticator: fixedAuthenticator{identity: cli.HTTPIdentity{
			Principal: "agent:finance", OnBehalfOf: "user:kai", Provider: "oidc", Subject: "agent-7",
		}},
	}))
	t.Cleanup(srv.Close)
	code, who := authenticatedRequest(t, srv, http.MethodPost, "/v1/whoami", map[string]any{}, "Bearer verified", "")
	if code != http.StatusOK || who["principal"] != "agent:finance" || who["onBehalfOf"] != "user:kai" {
		t.Fatalf("verified delegation was not injected: %d %#v", code, who)
	}
}

func TestGiteaAuthenticatorAgainstLiveGitea(t *testing.T) {
	base, token, _ := testkit.GiteaEndpoint(t)
	authenticator, err := cli.NewGiteaAuthenticator(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	id, err := authenticator.Authenticate(context.Background(), "token "+token)
	if err != nil {
		t.Fatal(err)
	}
	if id.Provider != "gitea" || id.Subject == "" || id.Principal != "gitea:"+id.Subject || id.Login != "kc" || !id.Admin {
		t.Fatalf("unexpected live Gitea identity: %#v", id)
	}
	if os.Getenv("KC_GITEA_URL") == "" {
		basic := base64.StdEncoding.EncodeToString([]byte("kc:kcpass123"))
		viaPassword, err := authenticator.Authenticate(context.Background(), "Basic "+basic)
		if err != nil {
			t.Fatal(err)
		}
		if viaPassword.Principal != id.Principal {
			t.Fatalf("PAT and Basic login resolved different principals: %#v %#v", id, viaPassword)
		}
	}
}
