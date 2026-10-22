package cli

import (
	"bytes"

	"strings"
	"testing"
)

func TestSnapshotHelpRootNavigation(t *testing.T) {
	root := Run([]string{"help"})
	if root.Status != 0 {
		t.Fatal(root)
	}
	groups := map[string][]string{
		"composition": {"show", "create", "attach", "detach", "registry audit", "grant add", "grant list", "grant remove", "dataset define", "dataset retire", "dataset overlay"},
		"dataset":     {"show", "grant add", "grant list", "grant remove", "dataset define", "dataset retire", "dataset overlay"},
		"files":       {"files list", "files read"},
		"grant":       {"grant add", "grant list", "grant remove"},
		"registry":    {"registry audit"},
	}
	for topic, commands := range groups {
		t.Run(topic, func(t *testing.T) {
			if !strings.Contains(root.Stdout, "kc help "+topic) {
				t.Errorf("root help has no executable link to %s", topic)
			}
			group := Run([]string{"help", topic})
			if group.Status != 0 {
				t.Fatal(group)
			}
			for _, command := range commands {
				if !strings.Contains(group.Stdout, "  kc "+command+" ") {
					t.Errorf("%s help does not disclose kc %s", topic, command)
				}
				leaf := Run(append([]string{"help"}, strings.Fields(command)...))
				if leaf.Status != 0 || !strings.Contains(leaf.Stdout, "kc "+command) {
					t.Errorf("disclosed command has no usable leaf help: %s %#v", command, leaf)
				}
			}
		})
	}
	if !strings.Contains(root.Stdout, "kcfs help") || !strings.Contains(Run([]string{"help", "dataset"}).Stdout, "kcfs help") {
		t.Error("Snapshot root and Dataset help must both link to the file-view tool")
	}
}

func TestConsumeHelpConnectsObjectFileAddressesToDownload(t *testing.T) {
	consume := Run([]string{"help", "consume"})
	for _, fact := range []string{"definition", "资源自己的仓", "commit", "路径", "kc files read", "kc help files", "file.read"} {
		if consume.Status != 0 || !strings.Contains(consume.Stdout, fact) {
			t.Errorf("object consumption help omits file download step %q: %#v", fact, consume)
		}
	}
	files := Run([]string{"help", "files"})
	leaf := Run([]string{"help", "files", "read"})
	for _, help := range []RunResult{files, leaf} {
		if help.Status != 0 || !strings.Contains(help.Stdout, "--repo <id> --commit <commit>") {
			t.Errorf("file help cannot carry an object reader to a repository download: %#v", help)
		}
	}
}

func TestSnapshotHelpDescribesResourceEffects(t *testing.T) {
	for _, test := range []struct {
		command string
		claims  []string
	}{
		{"create", []string{"不自动登记"}},
		{"attach", []string{"不创建"}},
		{"detach", []string{"保留", "文件", "历史"}},
		{"dataset define", []string{"发布", "固定版本", "单个", "必须递增", "同版发布不可覆盖", "目录来源", "非根交付目录", "根映射", "回退", "可在发布前", "跳过写回", "recipeLocation", "recipeSkipped"}},
		{"dataset retire", []string{"停止消费", "保留", "发布记录"}},
		{"dataset overlay", []string{"stdout", "--out", "配方文件"}},
	} {
		t.Run(test.command, func(t *testing.T) {
			result := Run(append([]string{"help"}, strings.Fields(test.command)...))
			if result.Status != 0 {
				t.Fatal(result)
			}
			for _, claim := range test.claims {
				if !strings.Contains(result.Stdout, claim) {
					t.Errorf("%s help omits the resource effect %q", test.command, claim)
				}
			}
			if test.command == "dataset define" && strings.Contains(result.Stdout, "所有 --source 都要写 @path") {
				t.Error("the path rule must distinguish directory mappings from per-file sources and unmapped sources")
			}
		})
	}
	compose := Run([]string{"help", "compose"})
	if compose.Status != 0 || !strings.Contains(compose.Stdout, "--action dataset.resolve,file.read") {
		t.Errorf("compose help does not grant both Dataset resolution and file consumption: %#v", compose)
	}
}

func TestSnapshotFileViewHelpDistinguishesPlanAndMount(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if status := RunDatasetFS([]string{"help"}, &stdout, &stderr); status != 0 {
		t.Fatalf("file-view help failed: %d %s", status, stderr.String())
	}
	for _, claim := range []string{"plan", "exits", "mount", "SIGINT", "per-file", "file gateway", "repository", "original bytes"} {
		if !strings.Contains(stdout.String(), claim) {
			t.Errorf("file-view help omits %q", claim)
		}
	}
}

func TestSnapshotHelpMergeRejectsDuplicateLeaves(t *testing.T) {
	defer func() {
		recovered := recover()
		message, ok := recovered.(string)
		if !ok || !strings.Contains(message, "show") {
			t.Fatalf("duplicate leaf was not identified: %v", recovered)
		}
	}()
	mergeHelpEntries(snapshotHelpEntries(), map[string]helpEntry{"show": {Description: "conflicting inventory description"}})
}
