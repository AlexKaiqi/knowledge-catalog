package invocation

import (
	"testing"

	"kc/invoke"
)

func TestDatasetFilePolicyKeepsRepositoryAndDatasetIntersection(t *testing.T) {
	home := t.TempDir()
	rule := AllowRule{Principal: "reader", Actions: []string{"file.read"}, Repo: "A", Dataset: "delivery"}
	if err := WriteAllow(home, AllowFile{Rules: []AllowRule{rule}}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, principal, repository, dataset string
		allowed                              bool
	}{
		{"both coordinates", "reader", "A", "delivery", true},
		{"other source", "reader", "B", "delivery", false},
		{"other dataset", "reader", "A", "other", false},
		{"other principal", "other", "A", "delivery", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			flags := map[string]invoke.FlagValue{"as": test.principal, "dataset": test.dataset}
			allowed, err := DatasetFSMayReadRepository(home, flags, test.repository)
			if err != nil || allowed != test.allowed {
				t.Fatalf("source policy discarded an authorization coordinate: allowed=%v want=%v error=%v", allowed, test.allowed, err)
			}
		})
	}
	flags := map[string]invoke.FlagValue{"as": "reader", "dataset": "delivery"}
	if AllowedDatasetFileRead(home, flags) {
		t.Fatal("one authorized source became whole-Dataset authority")
	}
	for _, qualifier := range []string{"ref", "object", "aspect"} {
		t.Run("unmatched "+qualifier, func(t *testing.T) {
			qualified := rule
			switch qualifier {
			case "ref":
				qualified.Ref = "main"
			case "object":
				qualified.Object = "concept/table"
			case "aspect":
				qualified.Aspect = "canonical"
			}
			if err := WriteAllow(home, AllowFile{Rules: []AllowRule{qualified}}); err != nil {
				t.Fatal(err)
			}
			if allowed, err := DatasetFSMayReadRepository(home, flags, "A"); err != nil || allowed {
				t.Fatalf("source file coordinates supplied an application qualifier: allowed=%v error=%v", allowed, err)
			}
		})
	}
}
