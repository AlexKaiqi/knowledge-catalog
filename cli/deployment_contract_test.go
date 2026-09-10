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
	for _, path := range []string{"deployment init", "deployment status", "deployment system publish", "attach", "create", "workspace overlay"} {
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
	t.Setenv("KC_SERVER_URL", "")
	home := t.TempDir()
	for _, args := range [][]string{
		{"--home", home, "show", "--config", "deployment.yaml"},
		{"--home", home, "show", "--listen", "127.0.0.1:0"},
		{"--home", home, "show", "--typo", "value"},
		{"--home", home, "attach", "--repo", "kr://acme/source", "--dir", "/tmp/source"},
		{"--home", home, "attach", "--repo", "kr://acme/source", "--driver", "dolt"},
		{"--home", home, "attach", "--repo", "kr://acme/source", "--dsn", "http://source"},
		{"--home", home, "pack", "--repo", "kr://acme/source", "--dir", t.TempDir(), "--config", "deployment.yaml"},
	} {
		result := Run(args)
		if result.Status == 0 || !strings.Contains(result.Stdout, "USAGE_INVALID") {
			t.Errorf("misplaced flags reached connection or identity %v: %s", args, result.Stdout)
		}
	}
}
