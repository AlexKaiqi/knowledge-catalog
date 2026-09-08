package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kc/cli"
	"kc/identity"
	"kc/kernel"
)

func TestDeploymentIdentityMigrationPreservesRevocationAndRefusesReassignment(t *testing.T) {
	cfg, config := declaredDeployment(t, false)
	cfg.BootstrapPrincipal = "gitea:42"
	writeDeployment(t, config, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", config))
	policy, err := cli.ReadAllow(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	policy.Rules = append(policy.Rules, cli.AllowRule{ID: "other-account", Principal: "gitea:43", Repo: "kr://other", Actions: []string{"knowledge.read"}})
	policy.InitialGrants = map[string]kernel.Digest{"revoked-creation": kernel.CanonicalDigest("original receipt")}
	if err := cli.WriteAllow(cfg.StateDir, policy); err != nil {
		t.Fatal(err)
	}
	originalReceipts := policy.InitialGrants
	request := cli.IdentityMigrationRequest{LegacyPrincipal: "gitea:42", User: identity.VerifiedUser{Username: "kaiqidong", Provider: "gitea", Issuer: "https://id.example", Subject: "42"}}
	requestPath := filepath.Join(t.TempDir(), "migration.json")
	writeRequest := func(request cli.IdentityMigrationRequest) {
		t.Helper()
		raw, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(requestPath, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeRequest(request)
	run := func() kcRunResult {
		return kcRunResultFrom(cli.Run([]string{"deployment", "identity", "migrate", "--config", config, "--file", requestPath}), "deployment identity migrate")
	}
	result := asMap(t, body(t, run()))
	if result["status"] != "APPLIED" || result["principal"] != "kaiqidong" || result["migratedRules"] != float64(1) {
		t.Fatalf("migration result: %#v", result)
	}
	var bindings struct {
		LegacyAliases map[string]identity.VerifiedUser `json:"legacyAliases"`
	}
	rawBindings, err := os.ReadFile(filepath.Join(cfg.StateDir, identity.Filename))
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(rawBindings, &bindings) != nil || bindings.LegacyAliases["gitea:42"] != request.User {
		t.Fatalf("explicit migration did not preserve control-plane owner alias: %s", rawBindings)
	}
	policy, err = cli.ReadAllow(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.Rules) != 2 || policy.Rules[0].Principal != "kaiqidong" || policy.Rules[1].Principal != "gitea:43" {
		t.Fatalf("migration changed unrelated grants or lost rule identity: %#v", policy.Rules)
	}
	if !reflect.DeepEqual(policy.InitialGrants, originalReceipts) {
		t.Fatal("migration changed initial policy receipts")
	}
	// Revoke the migrated grant, then replay. Migration must not use an old
	// snapshot or initial creation receipt to restore it.
	policy.Rules = policy.Rules[1:]
	if err := cli.WriteAllow(cfg.StateDir, policy); err != nil {
		t.Fatal(err)
	}
	result = asMap(t, body(t, run()))
	if result["status"] != "REPLAYED" {
		t.Fatalf("retry: %#v", result)
	}
	after, err := cli.ReadAllow(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, policy) {
		t.Fatal("migration replay changed authorization after revoke")
	}
	changed := request
	changed.User.Username = "other-owner"
	writeRequest(changed)
	expectCode(t, run(), "IDEMPOTENCY_CONFLICT")
	changed = request
	changed.LegacyPrincipal = "gitea:43"
	writeRequest(changed)
	expectCode(t, run(), "USAGE_INVALID")
	changed = request
	changed.LegacyPrincipal = "gitea:43"
	changed.User.Subject = "43"
	writeRequest(changed)
	expectCode(t, run(), "FORBIDDEN")
	after, err = cli.ReadAllow(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, policy) {
		t.Fatal("rejected identity migration changed policy")
	}
}

func TestDeploymentIdentityStateIsInitializedAndCannotBeRecreatedOnRecovery(t *testing.T) {
	cfg, config := declaredDeployment(t, false)
	body(t, deploymentCommand(t, "deployment", "init", "--config", config))
	user := identity.VerifiedUser{Username: "kaiqidong", Provider: "gitea", Issuer: "https://id.example", Subject: "42"}
	path := filepath.Join(cfg.StateDir, identity.Filename)
	if err := identity.Bind(path, user); err != nil {
		t.Fatal(err)
	}
	body(t, deploymentCommand(t, "deployment", "status", "--config", config))
	body(t, deploymentCommand(t, "deployment", "init", "--config", config))
	if err := identity.Bind(path, user); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	expectCode(t, deploymentCommand(t, "deployment", "status", "--config", config), "PRECONDITION_FAILED")
	expectCode(t, deploymentCommand(t, "deployment", "init", "--config", config), "PRECONDITION_FAILED")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lost identity state was recreated: %v", err)
	}
}
