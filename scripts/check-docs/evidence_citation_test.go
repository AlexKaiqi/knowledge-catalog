package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A design document that cites a pruned run id points at evidence nobody can
// open. The guard must reject it, and must not reject ordinary prose or a
// task record, where the id is a dated historical annotation.
func TestEvidenceCitationRejectsRunIDsInDesignDocuments(t *testing.T) {
	root := documentationFixture(t)
	writeMarkdownFixture(t, root, "doc/design/object.md",
		"# 对象\n\n读取按坐标规划，见运行 `20261020T150232Z-7f4893af014e`。\n")
	_, err := checkDocumentation(root)
	if err == nil || !strings.Contains(err.Error(), "20261020T150232Z-7f4893af014e") {
		t.Fatalf("cited run id in a design document was accepted: %v", err)
	}
}

// Task records keep their ids: they sit next to the date and scope that make a
// dead id readable as history, and tasks/done/ is never rewritten.
func TestEvidenceCitationIgnoresTaskRecords(t *testing.T) {
	root := documentationFixture(t)
	writeMarkdownFixture(t, root, "tasks/README.md", "# 任务\n\n状态即目录。\n")
	for _, state := range []string{"todo", "doing", "done", "explore"} {
		if err := os.MkdirAll(filepath.Join(root, "tasks", state), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeMarkdownFixture(t, root, "tasks/done/OLD-01.md",
		"# OLD-01\n\n证据见 `20261020T150232Z-7f4893af014e`。\n")
	if _, err := checkDocumentation(root); err != nil {
		t.Fatalf("task record citation was rejected: %v", err)
	}
}

// Ordinary dates and digests are not run ids.
func TestEvidenceCitationIgnoresUnrelatedTokens(t *testing.T) {
	root := documentationFixture(t)
	writeMarkdownFixture(t, root, "doc/design/object.md", strings.Join([]string{
		"# 对象",
		"",
		"2026-10-20 的审查以 `07d3c41a9dff04eb6737a371f530642d4f7c99af541215d64b6229eee117eba3` 为基线。",
		"运行目录形如 `.validation/runs/<run-id>/`，不在此处枚举具体编号。",
		"",
	}, "\n"))
	if _, err := checkDocumentation(root); err != nil {
		t.Fatalf("unrelated tokens were treated as run ids: %v", err)
	}
}

func TestEvidenceCitationReadsCurrentDesignDocuments(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	docs, err := designDocuments(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateNoCitedRunIDs(root, docs); err != nil {
		t.Fatalf("maintained design documents cite unreadable evidence: %v", err)
	}
}
