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

func TestEveryPublicHelpExplainsUseEffectsAndRecoveryWithoutExecuting(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	dir := t.TempDir()
	t.Setenv("KC_SERVER_URL", server.URL)
	t.Setenv("KC_HOME", filepath.Join(dir, "missing-home"))
	t.Setenv("KC_CONFIG_DIR", filepath.Join(dir, "missing-session"))
	paths := CLICommandsForTest()
	paths = append(paths, "serve")
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			entry, ok := helpEntries[path]
			if !ok {
				t.Fatalf("public entry has no help: %s", path)
			}
			for _, section := range []string{"用途", "用法与参数", "条件", "结果", "后果", "风险与成本", "失败恢复", "示例"} {
				_, rest, found := strings.Cut(entry.Usage, section+"：\n")
				if !found || strings.TrimSpace(strings.SplitN(rest, "\n", 2)[0]) == "" {
					t.Errorf("%s lacks a concrete %s explanation", path, section)
				}
			}
			if !strings.HasPrefix(entry.Usage, "kc "+path) || !strings.Contains(entry.Usage, "kc "+path) {
				t.Errorf("help must start with an executable leaf call: %q", entry.Usage)
			}
			byTopic := Run(append([]string{"help"}, strings.Fields(path)...))
			argv := append(strings.Fields(path), "--help")
			switch path {
			case "deployment init", "deployment status", "deployment system publish", "serve":
				argv = append(argv, "--config", filepath.Join(dir, "missing-deployment"))
			case "writer put", "writer commit", "governance proposal create", "dataset define", "dataset overlay", "grant update":
				argv = append(argv, "--file", filepath.Join(dir, "missing-input"))
			case "files read":
				argv = append(argv, "--out", filepath.Join(dir, "download"))
			}
			byFlag := Run(argv)
			want := formatLeafHelp(path) + "\n"
			if byTopic.Status != 0 || byFlag.Status != 0 || byTopic.Stdout != want || byFlag.Stdout != want {
				t.Fatalf("topic/flag help must display complete leaf without execution: topic=%#v flag=%#v", byTopic, byFlag)
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatalf("help contacted the server %d times", requests.Load())
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("help created Home, session or command outputs: %v %v", entries, err)
	}
}

func TestHelpPreservesDistinctWriteAndEvidenceBoundaries(t *testing.T) {
	for path, claims := range map[string][]string{
		"provenance":                   {"单仓需 object.provenance", "命名 Dataset", "file.read", "完整读取检查"},
		"access":                       {"resource.access", "file.read 不蕴含此访问权"},
		"writer commit":                {"目录外已有对象保留", "不自动拆批", "只轮询原回执"},
		"writer remove":                {"全部存储单元", "不擦除历史版本"},
		"invoke":                       {"外部状态", "幂等", "响应丢失", "盲目重试"},
		"deployment status":            {"追加", "审计", "不初始化"},
		"governance proposal merge":    {"--preview", "CAS", "不自动重新发布"},
		"governance validation record": {"不保存 --message", "不在这里跑套件", "替换当前保存结果"},
		"operations hook list":         {"全服务名单", "未传作查询过滤"},
		"operations gate list":         {"全服务名单", "未传作查询过滤"},
		"operations audit hitmap":      {"不返回下一页游标", "截断", "保留窗口"},
	} {
		t.Run(path, func(t *testing.T) {
			for _, claim := range claims {
				if !strings.Contains(formatLeafHelp(path), claim) {
					t.Errorf("%s help omitted its concrete guarantee or limitation %q", path, claim)
				}
			}
		})
	}
}
