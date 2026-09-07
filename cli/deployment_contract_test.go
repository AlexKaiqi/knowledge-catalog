package cli

import (
	"strings"
	"testing"
)

func TestDeploymentContractRetiresLocalAndManualRegistration(t *testing.T) {
	for path := range cliSurface {
		if strings.HasPrefix(path, "local ") || path == "catalog repo register" {
			t.Errorf("retired public operation remains: %s", path)
		}
	}
	for _, path := range []string{"deployment init", "deployment status", "deployment system publish", "catalog repo attach", "catalog repo create", "workspace overlay"} {
		if _, ok := cliSurface[path]; !ok {
			t.Errorf("missing operation: %s", path)
		}
	}
}

func TestDeploymentContractHelpWorksBeforeConnectionAndIdentity(t *testing.T) {
	for _, args := range [][]string{{"knowledge", "--help"}, {"knowledge", "read", "--help"}, {"help", "knowledge"}, {"deployment", "--help"}} {
		result := Run(args)
		if result.Status != 0 || !strings.Contains(result.Stdout, "kc") {
			t.Errorf("offline help %v: %s", args, result.Stdout)
		}
	}
}

func TestDeploymentContractRejectsMisplacedFlagsBeforeConnection(t *testing.T) {
	t.Setenv("KC_SERVER_URL", "http://127.0.0.1:1")
	for _, args := range [][]string{
		{"catalog", "show", "--config", "deployment.yaml"},
		{"catalog", "show", "--listen", "127.0.0.1:0"},
		{"catalog", "show", "--typo", "value"},
		{"catalog", "repo", "attach", "--repo", "kr://acme/source", "--dir", "/tmp/source"},
		{"catalog", "repo", "attach", "--repo", "kr://acme/source", "--driver", "dolt"},
		{"catalog", "repo", "attach", "--repo", "kr://acme/source", "--dsn", "http://source"},
		{"pack", "--repo", "kr://acme/source", "--dir", t.TempDir(), "--config", "deployment.yaml"},
	} {
		result := Run(args)
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
			t.Errorf("misplaced flags reached connection or identity %v: %s", args, result.Stdout)
		}
	}
}
