package server

import (
	"testing"

	"kc/home"
	"kc/identity"
	inv "kc/invocation"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	store "kc/snapshot/service"
)

func TestNativeRepositoryBindingsAreActualUniqueAndSameInstance(t *testing.T) {
	bindings := []home.HomeRepo{{ID: "kr://a/logical", Driver: "lakefs", DSN: "https://native.test/physical"}, {ID: "kr://a/second", Driver: "lakefs", DSN: "https://native.test/other"}, {ID: "kr://a/foreign", Driver: "lakefs", DSN: "https://elsewhere.test/foreign"}, {ID: "kr://a/alias", Driver: "lakefs", DSN: "https://native.test/physical"}}
	got := snapshotNativeRepositories(bindings, "https://native.test")
	if len(got) != 1 || got["other"] != "kr://a/second" {
		t.Fatalf("ambiguous/foreign binding survived: %#v", got)
	}
}

type nativeReadOnlySource struct{ snapshot.Store }

func (nativeReadOnlySource) ID() kernel.RepositoryID { return "kr://test/readonly" }

func TestNativeMutationCannotBypassMountedReadOnlyCapability(t *testing.T) {
	dir := t.TempDir()
	opened := &Home{Dir: dir, Store: snapshot.NewRegistry()}
	if err := opened.Store.Add(nativeReadOnlySource{}); err != nil {
		t.Fatal(err)
	}
	if err := inv.WriteAllow(dir, inv.AllowFile{Rules: []access.Rule{{Principal: "alice", Actions: []string{"*"}}}}); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{access.RepositoryMetadataWrite, access.RepositoryFilesWrite, access.RepositoryCommit, access.RepositoryRefsManage, access.RepositoryMerge, access.RepositorySettingsManage, access.RepositoryImport, access.RepositoryDelete} {
		err := authorizeNativeSnapshot(opened, store.Authorization{Caller: identity.HTTPIdentity{Principal: "alice", Admin: true}, NativeRepository: true, Query: access.Query{Repo: "kr://test/readonly", Action: action}})
		if kernel.CodeOf(err) != kernel.ErrForbidden {
			t.Fatalf("read-only source accepted %s: %v", action, err)
		}
	}
}

func TestNativeRepositoryAuthorizationUsesExistingLedgerWithoutInventoryFallback(t *testing.T) {
	dir := t.TempDir()
	opened := &Home{Dir: dir}
	caller := identity.HTTPIdentity{Principal: "alice", Admin: true}
	request := store.Authorization{Caller: caller, NativeRepository: true, Query: access.Query{Repo: "kr://a/repo", Action: access.RepositoryFileRead}}
	for _, rule := range []access.Rule{{Principal: "alice", Dataset: "files", Actions: []string{"*"}}, {Principal: "alice", Repo: "kr://a/repo", Object: "*", Actions: []string{"*"}}, {Principal: "bob", Repo: "kr://a/repo", Actions: []string{"*"}}} {
		if err := inv.WriteAllow(dir, inv.AllowFile{Rules: []access.Rule{rule}}); err != nil {
			t.Fatal(err)
		}
		if err := authorizeNativeSnapshot(opened, request); kernel.CodeOf(err) != kernel.ErrForbidden {
			t.Fatalf("native admin/qualified grant became access: %v", err)
		}
	}
	if err := inv.WriteAllow(dir, inv.AllowFile{Rules: []access.Rule{{Principal: "alice", Repo: "kr://a/repo", Actions: []string{access.RepositoryFileRead}}}}); err != nil {
		t.Fatal(err)
	}
	if err := authorizeNativeSnapshot(opened, request); err != nil {
		t.Fatal(err)
	}
	if err := inv.WriteAllow(dir, inv.AllowFile{}); err != nil {
		t.Fatal(err)
	}
	if err := authorizeNativeSnapshot(opened, request); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatal("revocation did not affect next request")
	}
}
