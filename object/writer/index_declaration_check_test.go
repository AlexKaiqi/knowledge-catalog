package writer_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/writer"
	"kc/snapshot"
)

func TestIndexDeclarationCheckSharesWriterValidationWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name, from string
		want       kernel.ErrorCode
	}{
		{name: "same batch Schema", from: "schema/article#/fields/body"},
		{name: "missing field", from: "schema/article#/fields/absent", want: kernel.ErrObjectRefUnresolved},
		{name: "missing Schema", from: "schema/missing#/fields/body", want: kernel.ErrObjectRefUnresolved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testkit.NewSetup(t, "")
			ledger, err := snapshot.NewCommandLedger(nil)
			if err != nil {
				t.Fatal(err)
			}
			w, err := writer.NewWriter(s.Store, ledger)
			if err != nil {
				t.Fatal(err)
			}
			declaration := testkit.IndexMember("body", "future-build", []string{objectmodel.IndexUseText}, tc.from)
			cs := objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
				BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
				Operations: append(testkit.PutEntity("schema/article", articleSchema(), ""), declaration)}
			digest := kernel.CanonicalDigest(cs)
			checked, fields, checkErr := w.CheckIndexDeclaration(cs, declaration.Address)
			if kernel.CodeOf(checkErr) != tc.want {
				t.Fatalf("check: %v, want %s", checkErr, tc.want)
			}
			if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != s.RootCommitID || len(ledger.Entries()) != 0 || kernel.CanonicalDigest(cs) != digest {
				t.Fatalf("check mutated authority, ledger or input: head=%s entries=%v", got, ledger.Entries())
			}
			if checkErr == nil && (checked.Member != "body" || checked.Build.Kind != "future-build" || len(fields) != 1 || fields[0].Path != "body" || fields[0].Type != "string") {
				t.Fatalf("check lost the resolved declaration: %#v %#v", checked, fields)
			}
			_, commitErr := w.Commit("checked-declaration", cs)
			if kernel.CodeOf(commitErr) != kernel.CodeOf(checkErr) {
				t.Fatalf("check and Writer disagree: check=%v commit=%v", checkErr, commitErr)
			}
		})
	}
}

func TestIndexDeclarationWriterResolvesEncodedSchemaFieldNames(t *testing.T) {
	s := testkit.NewSetup(t, "")
	fields := map[string]any{
		"统计 名称": map[string]any{"type": "string"},
		"a/b":   map[string]any{"type": "string"},
		"t~n":   map[string]any{"type": "string"},
	}
	declaration := testkit.IndexMember("encoded", "future-build", []string{objectmodel.IndexUseText},
		"schema/article#/fields/%E7%BB%9F%E8%AE%A1%20%E5%90%8D%E7%A7%B0", "schema/article#/fields/a~1b", "schema/article#/fields/t~0n")
	cs := objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: append(testkit.PutEntity("schema/article", map[string]any{"entity": "Article", "fields": fields}, ""), declaration)}
	_, resolved, err := s.Writer.CheckIndexDeclaration(cs, declaration.Address)
	if err != nil || len(resolved) != 3 {
		t.Fatalf("resolve ordinary Schema field names: %#v %v", resolved, err)
	}
	for i, want := range []string{"统计 名称", "a/b", "t~n"} {
		if resolved[i].Path != want {
			t.Fatalf("input %d resolved %q, want %q", i, resolved[i].Path, want)
		}
	}
	if _, err := s.Writer.Commit("encoded-fields", cs); err != nil {
		t.Fatalf("valid field pointer could not be published: %v", err)
	}
}
