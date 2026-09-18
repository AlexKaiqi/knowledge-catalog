package cli_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"kc/cli"
	apphome "kc/home"
)

func TestAdmissionCLIReportsCurrentGrantsAndExternalRequestRoute(t *testing.T) {
	cfg, config := declaredDeployment(t, false)
	cfg.Admission = &apphome.AdmissionConfig{RequestURL: "https://itsm.example/kc-access"}
	writeDeployment(t, config, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", config))

	policy, err := cli.ReadAllow(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	policy.Rules = append(policy.Rules,
		cli.AllowRule{ID: "reader", Principal: "kaiqidong", Repo: "kr://acme/knowledge", Actions: []string{"knowledge.read"}},
		cli.AllowRule{ID: "shared", Principal: "kaiqidong", Repo: "kr://acme/shared", Actions: []string{"knowledge.search"}, SharedBy: "alice", ShareID: "share-1"},
	)
	if err := cli.WriteAllow(cfg.StateDir, policy); err != nil {
		t.Fatal(err)
	}

	handler, err := cli.HTTPHandlerFromConfig(config, cli.HTTPServerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer handler.(interface{ Close() error }).Close()
	server := httptest.NewServer(handler)
	defer server.Close()

	result := asMap(t, body(t, kcRemote(t, server.URL, "kaiqidong", "admission", "show")))
	if result["principal"] != "kaiqidong" || result["status"] != nil || result["eligible"] != nil || result["currentActions"] != nil {
		t.Fatalf("admission response kept retired fields: %#v", result)
	}
	request := asMap(t, result["request"])
	if request["url"] != "https://itsm.example/kc-access" {
		t.Fatalf("missing request route: %#v", request)
	}
	admins, _ := request["administrators"].([]any)
	if len(admins) != 1 || admins[0] != cfg.BootstrapPrincipal {
		t.Fatalf("initialized deployment must list the bootstrap grant manager: %#v", request)
	}
	if grants, ok := result["grants"].([]any); !ok || len(grants) != 2 {
		t.Fatalf("current grants are not scoped to the caller: %#v", result)
	}
	// Admission is caller-scoped; a Catalog operand is not part of its surface.
	expectCode(t, kcRemote(t, server.URL, "kaiqidong", "admission", "show", "--catalog", "kr://acme/catalog"), "USAGE_INVALID")

	req, err := http.NewRequest(http.MethodPost, server.URL+"/identity/v1/admission", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Kc-As", "kaiqidong")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST admission still exists: %d", resp.StatusCode)
	}
}
