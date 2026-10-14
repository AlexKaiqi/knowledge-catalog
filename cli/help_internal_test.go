package cli

import (
	"strings"
	"testing"
)

func TestHelp(t *testing.T) {
	result := Run([]string{"help"})
	if result.Status != 0 {
		t.Fatal(result)
	}
	want := rootHelp
	if !strings.HasSuffix(want, "\n") {
		want += "\n"
	}
	if result.Stdout != want {
		t.Fatalf("help mismatch")
	}
	for _, needle := range []string{"object_id", "Aspect", "Member", "Address", "Relation", "JSON Pointer", "#/columns/order_id/constraints/nullable", "writer put/remove", "身份", "组合", "对象", "发布", "分支", "运维", "部署", "kc help consume"} {
		if !strings.Contains(result.Stdout, needle) {
			t.Fatal(needle)
		}
	}
	for _, leak := range []string{"kc login", "kc show", "kc writer put --command-id", "--value", "--file", "--kind", "--base", "--repo", "--server", "registry audit", "resolve", "writer merge --command-id"} {
		if strings.Contains(result.Stdout, leak) {
			t.Fatalf("root help disclosed command detail %q", leak)
		}
	}
}
