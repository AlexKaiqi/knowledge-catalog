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
	for _, needle := range []string{"身份", "组合", "知识", "发布", "治理（进阶）", "运维", "部署", "kc help consume"} {
		if !strings.Contains(result.Stdout, needle) {
			t.Fatal(needle)
		}
	}
	for _, leak := range []string{"kc login", "kc show", "writer put", "--repo", "--server", "catalog audit", "resolve", "governance preview"} {
		if strings.Contains(result.Stdout, leak) {
			t.Fatalf("root help disclosed command detail %q", leak)
		}
	}
}
