package cli

import "testing"

func TestHTTPServerOptionsFromFlags(t *testing.T) {
	t.Setenv("KC_RESOURCE_ACCESS_URL", "")
	if _, err := httpServerOptionsFromFlags(map[string]FlagValue{"auth-url": "https://git.example"}); err == nil {
		t.Fatal("auth-url without auth mode must fail")
	}
	if _, err := httpServerOptionsFromFlags(map[string]FlagValue{"auth": "oidc"}); err == nil {
		t.Fatal("unknown auth mode must fail")
	}
	if _, err := httpServerOptionsFromFlags(map[string]FlagValue{"auth": "gitea"}); err == nil {
		t.Fatal("gitea mode without auth-url must fail")
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
}

func TestHTTPServerOptionsConfigureRemoteStateRuntime(t *testing.T) {
	t.Setenv("KC_RESOURCE_ACCESS_URL", "https://runtime.internal/prefix")
	options, err := httpServerOptionsFromFlags(map[string]FlagValue{})
	if err != nil || options.StateLookup == nil {
		t.Fatalf("environment runtime: %#v %v", options, err)
	}
	options, err = httpServerOptionsFromFlags(map[string]FlagValue{"resource-access-url": "http://resource-runtime:8090"})
	if err != nil || options.StateLookup == nil {
		t.Fatalf("flag runtime: %#v %v", options, err)
	}
	if _, err := httpServerOptionsFromFlags(map[string]FlagValue{"resource-access-url": "file:///runtime"}); err == nil {
		t.Fatal("serve accepted a non-HTTP State runtime")
	}
}
