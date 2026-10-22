package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Object versioning rests on three separate bases: a range-limited local state
// digest, a fixed source/dependency coordinate for exact read-back, and a
// revision history that tells two edits apart. Collapsing them into one
// "version" is the confusion this guard exists to prevent: a digest is not a
// read coordinate, a local commit is not a full dependency vector, and equal
// digests do not prove equal revisions (A -> B -> A restores the digest, not the
// revision). Documentation that reintroduces such a claim fails `make check-docs`
// instead of relying on reviewers to remember the split.
var collapsedVersionClaims = []struct {
	name    string
	pattern *regexp.Regexp
	because string
}{
	{
		name:    "global object digest",
		pattern: regexp.MustCompile(`全对象摘要|全局(?:完整)?(?:对象)?摘要|全局唯一版本`),
		because: "跨仓对象没有全局摘要：状态摘要只在明确范围内有效，且不能作读取坐标",
	},
	{
		name:    "digest as revision identity",
		pattern: regexp.MustCompile(`摘要(?:即|就是|等同于)修订|(?:修订|版本)(?:即|就是|等同于)摘要`),
		because: "摘要相同不代表同一修订：A→B→A 会恢复摘要但不是同一修订",
	},
	{
		name:    "local commit as full pin",
		pattern: regexp.MustCompile(`(?:单仓)?(?:本地)?提交(?:即|就是|等同于|足以)(?:固定|确定)(?:完整|全部|所有)`),
		because: "本地提交只固定本地保存的内容；精确回读需覆盖全部解释依赖坐标",
	},
}

// validateVersionSemantics scans the maintained design documents for claims that
// collapse the three bases above.
func validateVersionSemantics(root string, docs []string) error {
	var violations []string
	for _, doc := range docs {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(doc)))
		if err != nil {
			return err
		}
		for _, claim := range collapsedVersionClaims {
			for _, line := range strings.Split(string(raw), "\n") {
				if !claim.pattern.MatchString(line) {
					continue
				}
				violations = append(violations, fmt.Sprintf("%s: %s: %s（%s）",
					doc, claim.name, strings.TrimSpace(line), claim.because))
			}
		}
	}
	if len(violations) > 0 {
		return fmt.Errorf("object version wording collapses distinct bases:\n  %s", strings.Join(violations, "\n  "))
	}
	return nil
}
