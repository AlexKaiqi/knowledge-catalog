package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestResourceMetadataHelpNamesReachableManagementAndPublication(t *testing.T) {
	for _, leaf := range []string{"show", "create"} {
		text := formatLeafHelp(leaf)
		for _, boundary := range []string{
			"/console → Repositories → 目标仓 → Settings → General",
			"repository.metadata.read", "repository.metadata.write", "原仓浏览权限",
			"不随 commit 冻结", "登记账", "不自动继承",
			"dataset define", "--metadata", "revision", "dataset show --revision",
		} {
			if !strings.Contains(text, boundary) {
				t.Errorf("%s help does not explain metadata management/publication boundary %q", leaf, boundary)
			}
		}
	}
	if text := formatLeafHelp("show"); !strings.Contains(text, "不读取 Repository 自定义元数据表") {
		t.Fatal("inventory help must distinguish registered identities from native metadata reads")
	}
	if text := formatLeafHelp("attach"); !strings.Contains(text, "不读取、复制或编辑原仓自定义元数据") {
		t.Fatal("attachment help must not imply a second metadata authority")
	}
	for _, command := range CLICommandsForTest() {
		if strings.Contains(command, "metadata") {
			t.Fatalf("help must not introduce an unimplemented metadata command: %s", command)
		}
	}
}

func TestRepositoryMetadataFlagStaysUnavailableWithoutHTTPOrLocalWrites(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	dir := t.TempDir()
	t.Setenv("KC_SERVER_URL", server.URL)
	t.Setenv("KC_CONFIG_DIR", filepath.Join(dir, "missing-session"))
	t.Setenv("KC_HOME", filepath.Join(dir, "missing-home"))
	for _, argv := range [][]string{
		{"show", "--metadata", `{"description":"unused"}`},
		{"create", "--name", "table-meta", "--metadata", `{"description":"unused"}`},
		{"attach", "--repo", "table-meta", "--metadata", `{"description":"unused"}`},
	} {
		result := Run(argv)
		if result.Status == 0 || !strings.Contains(result.Stdout, "unknown flag --metadata") {
			t.Errorf("unimplemented Repository metadata flag should fail before effects: %v %#v", argv, result)
		}
	}
	for _, leaf := range []string{"show", "attach", "create"} {
		byTopic := Run([]string{"help", leaf})
		byFlag := Run([]string{leaf, "--help"})
		if byTopic.Status != 0 || byFlag.Status != 0 || byTopic.Stdout != byFlag.Stdout {
			t.Errorf("metadata guidance should use the static help entry: %s %#v %#v", leaf, byTopic, byFlag)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("metadata help/input rejection contacted HTTP %d times", requests.Load())
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("metadata help/input rejection wrote local state: %v %v", entries, err)
	}
}
