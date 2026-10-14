package invocation

import (
	"context"
	"reflect"
	"testing"

	"kc/invoke"
	"kc/kernel"
)

func TestLegacyGrantAddCannotDelegateSourcePublicationFromDatasetManagement(t *testing.T) {
	for _, action := range []string{"repository.dataset.publish", "repository.*"} {
		t.Run(action, func(t *testing.T) {
			dir := t.TempDir()
			before := AllowFile{Rules: []AllowRule{{ID: "dataset-admin", Principal: "editor", Dataset: "files", Actions: []string{"admin.grants.manage"}}}}
			if err := WriteAllow(dir, before); err != nil {
				t.Fatal(err)
			}
			before, err := ReadAllow(dir)
			if err != nil {
				t.Fatal(err)
			}
			flags := map[string]invoke.FlagValue{"as": "editor", "principal": "editor", "action": action, "repo": "private", "dataset": "files", "ref": "refs/heads/main"}
			operation := Command{Stage: invoke.StageHome, Run: VerbAllow}
			_, err = ExecuteApplicationOperation(context.Background(), "grant-add", "admin.grants.manage", operation, flags, nil, nil, dir, nil)
			if kernel.CodeOf(err) != kernel.ErrForbidden {
				t.Errorf("legacy grant scope omitted source action %s: %v", action, err)
			}
			after, err := ReadAllow(dir)
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("source self-grant changed durable policy: before=%+v after=%+v error=%v", before, after, err)
			}
		})
	}
}

func TestLegacySourcePublicationQuestionUsesPublicationScope(t *testing.T) {
	query := AllowQuery{Principal: "publisher", Action: "repository.dataset.publish", Repo: "private", Dataset: "files", Ref: "refs/heads/main"}
	for _, rule := range []AllowRule{
		{Principal: "publisher", Dataset: "files", Actions: []string{"repository.dataset.publish"}},
		{Principal: "publisher", Repo: "private", Object: "*", Actions: []string{"repository.dataset.publish"}},
	} {
		if result, err := EvaluateAllow([]AllowRule{rule}, query); kernel.CodeOf(err) != kernel.ErrForbidden || result != nil {
			t.Fatalf("legacy question invented source authority: %+v, %v", result, err)
		}
	}
	valid := AllowRule{ID: "source-rule", Principal: "publisher", Repo: "private", Dataset: "files", Ref: "refs/heads/main", Actions: []string{"repository.dataset.publish"}}
	result, err := EvaluateAllow([]AllowRule{valid}, query)
	if err != nil || !reflect.DeepEqual(result, map[string]any{"allow": true, "ruleId": "source-rule"}) {
		t.Fatalf("legacy explicit source question changed: %+v, %v", result, err)
	}
}
