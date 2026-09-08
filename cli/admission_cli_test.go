package cli_test

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"kc/cli"
	apphome "kc/home"
)

func TestAdmissionCLIExplicitPolicyAndDurableRevocation(t *testing.T) {
	cfg, config := declaredDeployment(t, false)
	cfg.Admission = &apphome.AdmissionConfig{Enabled: true, Catalog: cfg.Catalogs[0].ID, Principals: []string{"kaiqidong"}, Actions: []string{"catalog.read", "catalog.repositories.create"}}
	writeDeployment(t, config, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", config))
	handler, err := cli.HTTPHandlerFromConfig(config, cli.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer handler.(interface{ Close() error }).Close()
	server := httptest.NewServer(handler)
	defer server.Close()
	call := func(principal string, args ...string) kcRunResult { return kcRemote(t, server.URL, principal, args...) }
	status := asMap(t, body(t, kcRemote(t, server.URL, "kaiqidong", "admission", "show")))
	if status["status"] != "AVAILABLE" || status["eligible"] != true {
		t.Fatalf("first-use admission: %#v", status)
	}
	expectCode(t, call("kaiqidong", "catalog", "show", cfg.Catalogs[0].ID), "FORBIDDEN")
	result := asMap(t, body(t, kcRemote(t, server.URL, "kaiqidong", "admission", "request")))
	if result["status"] != "APPLIED" {
		t.Fatalf("admission did not apply: %#v", result)
	}
	body(t, call("kaiqidong", "catalog", "show", cfg.Catalogs[0].ID))
	policy, err := cli.ReadAllow(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range policy.Rules {
		if rule.Principal == "kaiqidong" {
			body(t, call("agent:operator", "admin", "grant", "remove", "--id", rule.ID))
		}
	}
	result = asMap(t, body(t, call("kaiqidong", "admission", "request")))
	if result["status"] != "REPLAYED" || len(result["currentActions"].([]any)) != 0 {
		t.Fatalf("replay recreated revoked policy: %#v", result)
	}
	policyPath := filepath.Join(cfg.StateDir, "allow.json")
	original, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(policyPath); err != nil {
		t.Fatal(err)
	}
	expectCode(t, call("kaiqidong", "admission", "show"), "PRECONDITION_FAILED")
	if err := os.WriteFile(policyPath, original, 0600); err != nil {
		t.Fatal(err)
	}
	t.Run("unlisted user cannot self admit", func(t *testing.T) {
		expectCode(t, kcRemote(t, server.URL, "someone", "admission", "request"), "FORBIDDEN")
	})
	t.Run("machine cannot use human policy", func(t *testing.T) {
		expectCode(t, kcRemote(t, server.URL, "agent:operator", "admission", "request"), "FORBIDDEN")
	})
}
