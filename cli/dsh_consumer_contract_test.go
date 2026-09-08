//go:build dsh_contract

package cli_test

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"kc/cli"
	"kc/internal/testkit"
	"kc/snapshot"
)

// Run with -tags=dsh_contract after npm --prefix dsh-plugin run build.
// This crosses the actual Go Server DTOs, the packaged host bridge, and the
// authenticated knowledge reader; it does not replace HTTP with JSON mocks.
func TestDSHConsumerUsesActualServerContract(t *testing.T) {
	isolateClientCredentials(t)
	t.Setenv("KC_REQUIRE_LIVE_ADAPTERS", "1")
	base, token, run := testkit.GiteaEndpoint(t)
	t.Setenv("KC_GITEA_TOKEN", token)
	home := testkit.TempDir(t)
	cat, repo, principal := "kr://probe/catalog", "kr://probe/docs", "consumer"
	body(t, kc(home, "local", "init", "--catalog", cat))
	seedRepo(t, home, repo, "--driver", "gitea", "--dsn", base+"/kc/consumer-"+run)
	body(t, kc(home, "local", "grant", "bootstrap", "--principal", principal))
	handler := cli.HTTPHandlerWithOptions(home, cli.HTTPServerOptions{})
	if closer, ok := handler.(interface{ Close() error }); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	invoke := func(args ...string) kcRunResult { return kcRemote(t, server.URL, principal, args...) }
	body(t, invoke("writer", "put", "--command-id", "schema-probe", "--repo", repo, "--object", "schema/note", "--value", `{"entity":"Note","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}`))
	body(t, invoke("writer", "put", "--command-id", "note-probe", "--repo", repo, "--object", "note/one", "--schema-ref", "schema/note", "--value", `{"body":"consumed through the actual Server"}`))
	body(t, invoke("workspace", "define", "--catalog", cat, "--workspace", "named", "--revision", "1", "--source", repo))
	node := os.Getenv("KC_NODE_BIN")
	if node == "" {
		node = "node"
	}
	script, err := filepath.Abs("../dsh-plugin/scripts/test-server-contract.mjs")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, script)
	cmd.Env = append(os.Environ(), "KC_DSH_CONTRACT_SERVER="+server.URL, "KC_DSH_DEFAULT_REF="+snapshot.DefaultRef)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("packaged consumer contract: %v\n%s", err, output)
	}
	t.Log(string(output))
}
