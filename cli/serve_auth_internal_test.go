package cli

import (
	"strings"
	"testing"
)

func TestHTTPServerOptionsFromFlags(t *testing.T) {
	t.Setenv("KC_RESOURCE_ACCESS_URL", "")
	t.Setenv("KC_RERANK_MODEL", "")
	t.Setenv("KC_TAIHU_HMAC_SECRET", "")
	t.Setenv("KC_SERVICE_CLIENT_SECRET", "")
	if _, err := httpServerOptionsFromFlags(map[string]FlagValue{}); err == nil {
		t.Fatal("kc serve without --auth must fail")
	}
	if _, err := httpServerOptionsFromFlags(map[string]FlagValue{"auth-url": "https://git.example"}); err == nil {
		t.Fatal("auth-url without auth mode must fail")
	}
	if _, err := httpServerOptionsFromFlags(map[string]FlagValue{"auth": "oidc"}); err == nil {
		t.Fatal("unknown auth mode must fail")
	}
	if _, err := httpServerOptionsFromFlags(map[string]FlagValue{"auth": "gitea"}); err == nil {
		t.Fatal("gitea mode without auth-url must fail")
	}
	if _, err := httpServerOptionsFromFlags(map[string]FlagValue{"auth": "local", "auth-url": "https://git.example"}); err == nil {
		t.Fatal("--auth local with --auth-url must fail")
	}
	local, err := httpServerOptionsFromFlags(map[string]FlagValue{"auth": "local"})
	if err != nil || local.Authenticator != nil || local.authMode() != "local" || !local.localAssertion() {
		t.Fatalf("local options: %#v %v", local, err)
	}
	options, err := httpServerOptionsFromFlags(map[string]FlagValue{
		"auth":       "gitea",
		"auth-url":   "https://git.example/prefix",
		"auth-admin": []string{"gitea:1", "gitea:2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.Authenticator == nil || options.Authenticator.Name() != "gitea" || len(options.AdminPrincipals) != 2 {
		t.Fatalf("unexpected options: %#v", options)
	}
	// Taihu can work without --auth-url (x-tai-identity header mode).
	taihuOpts, err := httpServerOptionsFromFlags(map[string]FlagValue{"auth": "taihu"})
	if err != nil {
		t.Fatalf("taihu mode without auth-url: %v", err)
	}
	if taihuOpts.Authenticator == nil || taihuOpts.Authenticator.Name() != "taihu" {
		t.Fatal("taihu authenticator must be created")
	}
	// Taihu with --auth-url (token introspection mode).
	taihuOpts, err = httpServerOptionsFromFlags(map[string]FlagValue{"auth": "taihu", "auth-url": "https://taihu.example"})
	if err != nil {
		t.Fatalf("taihu mode with auth-url: %v", err)
	}
	if taihuOpts.Authenticator == nil || taihuOpts.Authenticator.Name() != "taihu" {
		t.Fatal("taihu authenticator must be created")
	}
}

func TestHTTPServerOptionsConfigureExplicitLLMReranker(t *testing.T) {
	t.Setenv("KC_RESOURCE_ACCESS_URL", "")
	t.Setenv("OPENAI_BASE_URL", "https://llm.example/v1")
	t.Setenv("OPENAI_API_KEY", "secret")
	t.Setenv("KC_RERANK_MODEL", "")
	options, err := httpServerOptionsFromFlags(map[string]FlagValue{"auth": "local", "rerank-model": "gpt-5.6-luna", "rerank-timeout": "12s"})
	if err != nil || options.Reranker == nil {
		t.Fatalf("reranker options: %#v %v", options, err)
	}
	if _, err := httpServerOptionsFromFlags(map[string]FlagValue{"auth": "local", "rerank-model": "gpt-5.6-luna", "rerank-timeout": "never"}); err == nil {
		t.Fatal("invalid rerank timeout was accepted")
	}
	t.Setenv("OPENAI_API_KEY", "")
	if _, err := httpServerOptionsFromFlags(map[string]FlagValue{"auth": "local", "rerank-model": "gpt-5.6-luna"}); err == nil {
		t.Fatal("enabled reranker without credentials was accepted")
	}
}

func TestHTTPServerOptionsInstallStateLookupWithoutServeURL(t *testing.T) {
	t.Setenv("KC_RESOURCE_ACCESS_URL", "")
	t.Setenv("KC_RERANK_MODEL", "")
	options, err := httpServerOptionsFromFlags(map[string]FlagValue{"auth": "local"})
	if err != nil || options.StateLookup == nil {
		t.Fatalf("serve must install a Schema-origin StateLookup: %#v %v", options, err)
	}
}

func TestServeRequiresExplicitDeploymentConfiguration(t *testing.T) {
	for _, args := range [][]string{{"serve"}, {"serve", "--auth", "local"}} {
		result := Run(args)
		if result.Status == 0 || !strings.Contains(result.Stdout, "--config") {
			t.Fatalf("kc serve without declared configuration: %#v", result)
		}
	}
}
