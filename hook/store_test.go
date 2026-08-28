package hook_test

import (
	"testing"

	"kc/hook"
	"kc/internal/testkit"
	"kc/kernel"
)

func TestReadMissingHooksIsEmpty(t *testing.T) {
	home := testkit.TempDir(t)
	file, err := hook.Read(home)
	if err != nil || len(file.Bindings) != 0 {
		t.Fatal(file, err)
	}
}

func TestCanHook(t *testing.T) {
	if !hook.CanHook("writer.commit") || !hook.CanHook("workspace.manage") || hook.CanHook("append") || hook.CanHook("read") {
		t.Fatal("CanHook set")
	}
	testkit.ExpectCode(t, hook.ValidateOn("read"), kernel.ErrUsageInvalid)
	testkit.ExpectCode(t, hook.ValidateBinding(hook.Binding{On: "writer.commit", Phase: hook.PhasePost}), kernel.ErrUsageInvalid)
	testkit.ExpectCode(t, hook.ValidateBinding(hook.Binding{On: "writer.commit", Phase: hook.PhasePost, Run: "a.sh", URL: "http://x"}), kernel.ErrUsageInvalid)
}

func TestMatchRepoAndCatalog(t *testing.T) {
	file := hook.File{Bindings: []hook.Binding{
		{ID: "a", On: "writer.commit", Phase: hook.PhasePre, Repo: "kr://acme/physical", Run: "a.sh"},
		{ID: "b", On: "writer.commit", Phase: hook.PhasePre, Run: "all.sh"},
		{ID: "c", On: "workspace.manage", Phase: hook.PhasePost, Catalog: "kr://acme/catalog", URL: "http://x"},
	}}
	put := file.Match("writer.commit", hook.PhasePre, "kr://acme/physical", "")
	if len(put) != 2 {
		t.Fatal(put)
	}
	other := file.Match("writer.commit", hook.PhasePre, "kr://acme/semantic", "")
	if len(other) != 1 || other[0].ID != "b" {
		t.Fatal(other)
	}
	if got := file.Match("writer.commit", hook.PhasePost, "kr://acme/physical", ""); len(got) != 0 {
		t.Fatal(got)
	}
	prom := file.Match("workspace.manage", hook.PhasePost, "", "kr://acme/catalog")
	if len(prom) != 1 || prom[0].ID != "c" {
		t.Fatal(prom)
	}
	if got := file.Match("workspace.manage", hook.PhasePost, "", "kr://other/catalog"); len(got) != 0 {
		t.Fatal(got)
	}
}
