package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The guard has to fail on a collapsed claim and pass on the wording the design
// actually uses. A guard that only ever passes protects nothing.
func TestVersionSemanticsRejectsCollapsedClaims(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "global digest",
			body: "# 对象\n\n跨仓对象只有一个全对象摘要。\n",
			want: "global object digest",
		},
		{
			name: "global unique version",
			body: "# 对象\n\n对象版本就是该对象的全局唯一版本。\n",
			want: "global object digest",
		},
		{
			name: "digest equals revision",
			body: "# 对象\n\n摘要即修订，可以直接用它辨认两次修改。\n",
			want: "digest as revision identity",
		},
		{
			name: "local commit pins everything",
			body: "# 读取\n\n单仓提交足以固定完整解释依赖。\n",
			want: "local commit as full pin",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := documentationFixture(t)
			writeMarkdownFixture(t, root, "doc/design/object.md", tc.body)
			_, err := checkDocumentation(root)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

// The adopted wording separates the three bases explicitly; it must keep passing.
func TestVersionSemanticsAcceptsSeparatedBases(t *testing.T) {
	root := documentationFixture(t)
	writeMarkdownFixture(t, root, "doc/design/object.md", strings.Join([]string{
		"# 对象",
		"",
		"明确读取范围内，由固定的来源坐标及所需解释依赖坐标确定的内容。跨仓是一组坐标，不代表全局时刻。",
		"状态摘要用于比较本地状态，不作为读取坐标或修订身份。",
		"不同修订可能具有相同状态摘要：A 改为 B 再恢复 A，恢复后的修订不同于最初的 A。",
		"本地提交只固定本地保存内容；精确回读的固定依据须覆盖全部所需解释依赖。",
		"",
	}, "\n"))
	if _, err := checkDocumentation(root); err != nil {
		t.Fatalf("adopted wording was rejected: %v", err)
	}
}

// Every maintained document is scanned, not only the object design.
func TestVersionSemanticsScansEveryDesignDocument(t *testing.T) {
	root := documentationFixture(t)
	writeMarkdownFixture(t, root, "doc/design/versioning.md", "# 版本\n\n跨仓对象只有一个全对象摘要。\n")
	writeMarkdownFixture(t, root, "doc/README.md",
		"# 文档\n\n[愿景](vision.md)、[对象](design/object.md)、[版本](design/versioning.md)\n")
	_, err := checkDocumentation(root)
	if err == nil || !strings.Contains(err.Error(), "doc/design/versioning.md") {
		t.Fatalf("claim outside the object design was not reported: %v", err)
	}
}

func TestVersionSemanticsReadsCurrentDesignDocuments(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	docs, err := designDocuments(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateVersionSemantics(root, docs); err != nil {
		t.Fatalf("maintained design documents violate the adopted split: %v", err)
	}
	for _, doc := range docs {
		if _, err := os.Stat(filepath.Join(root, doc)); err != nil {
			t.Fatalf("%s: %v", doc, err)
		}
	}
}
