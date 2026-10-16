package invocation

import (
	"kc/invoke"
	"os"
	"testing"
)

func TestDatasetFilePolicyHonorsFixedSourceRef(t *testing.T) {
	home := t.TempDir()
	if err := WriteAllow(home, AllowFile{Rules: []AllowRule{{Principal: "reader", Actions: []string{"file.read"}, Repo: "A", Dataset: "delivery", Ref: "c1"}}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		ref     string
		allowed bool
	}{{"c1", true}, {"c2", false}, {"", false}} {
		t.Run(tc.ref, func(t *testing.T) {
			flags := map[string]invoke.FlagValue{"as": "reader", "dataset": "delivery", "ref": tc.ref}
			allowed, err := DatasetFSMayReadRepository(home, flags, "A")
			if err != nil || allowed != tc.allowed {
				t.Fatalf("source grant discarded the fixed version: allowed=%v want=%v error=%v", allowed, tc.allowed, err)
			}
		})
	}
}

func TestDatasetFilePolicyPropagatesGrantStoreFailure(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(AllowPath(home), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	_, fault := ReadAllow(home)
	if fault == nil {
		t.Fatal("fault fixture did not fail the grant store")
	}
	flags := map[string]invoke.FlagValue{"as": "reader", "dataset": "delivery", "ref": "c1"}
	allowed, err := DatasetFSMayReadRepository(home, flags, "A")
	if allowed || err == nil || err.Error() != fault.Error() {
		t.Fatalf("grant store failure changed: allowed=%v error=%v want=%v", allowed, err, fault)
	}
	// The trusted local direct path keeps its existing owner semantics.
	if allowed, err := DatasetFSMayReadRepository(home, nil, "A"); err != nil || !allowed {
		t.Fatalf("local direct owner semantics changed: allowed=%v error=%v", allowed, err)
	}
}

func TestDatasetFilePolicyDoesNotUseApplicationQualifiers(t *testing.T) {
	for _, qualifier := range []string{"object", "aspect"} {
		t.Run(qualifier, func(t *testing.T) {
			home := t.TempDir()
			rule := AllowRule{Principal: "reader", Actions: []string{"file.read"}, Repo: "A", Dataset: "delivery"}
			if qualifier == "object" {
				rule.Object = "private"
			} else {
				rule.Aspect = "private"
			}
			if err := WriteAllow(home, AllowFile{Rules: []AllowRule{rule}}); err != nil {
				t.Fatal(err)
			}
			flags := map[string]invoke.FlagValue{"as": "reader", "dataset": "delivery", qualifier: "private", "repo": "untrusted"}
			if allowed, err := DatasetFSMayReadRepository(home, flags, "A"); err != nil || allowed {
				t.Fatalf("application qualifier authorized the file source: allowed=%v error=%v", allowed, err)
			}
		})
	}
}
