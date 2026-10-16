package writer_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

func TestIndexDeclarationWriterAcceptsMultipleInputsFromBatchSchemas(t *testing.T) {
	s := testkit.NewSetup(t, "")
	declaration := testkit.IndexMember("combined", "vector", []string{objectmodel.IndexUseSemantic},
		"schema/service.definition#/fields/name", "schema/service.definition#/fields/summary", "schema/oncall.guide#/fields/body")
	body := declaration.Value.(map[string]any)
	body["build"] = map[string]any{"kind": "vector", "template": "{name}\n{summary}\n{body}", "model": "embed/test@v1"}
	input := body["inputs"].([]any)[2].(map[string]any)
	input["missing"], input["maxTokens"] = objectmodel.IndexMissingSkip, 512
	// Declaration order must not require its Schema drafts to appear first.
	at := commitIndexWriterDrafts(t, s, s.Repo, "combined", declaration,
		indexWriterSchema("schema/service.definition", "definition", "name", "summary"),
		indexWriterSchema("schema/oncall.guide", "guide", "body"))
	assertIndexWriterStored(t, s, s.RepositoryID, at, declaration)
}

func TestIndexDeclarationWriterRejectsInvalidInputsAndShapeAtomically(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
		code   kernel.ErrorCode
	}{
		{"missing second input field", func(v map[string]any) {
			v["inputs"] = []any{map[string]any{"from": "schema/article#/fields/body"}, map[string]any{"from": "schema/article#/fields/missing"}}
		}, kernel.ErrObjectRefUnresolved},
		{"missing schema", func(v map[string]any) { v["inputs"] = []any{map[string]any{"from": "schema/absent#/fields/body"}} }, kernel.ErrObjectRefUnresolved},
		{"invalid locator", func(v map[string]any) { v["inputs"] = []any{map[string]any{"from": "schema/article#/body"}} }, kernel.ErrUsageInvalid},
		{"empty build kind", func(v map[string]any) { v["build"] = map[string]any{"kind": ""} }, kernel.ErrUsageInvalid},
		{"deployment configuration", func(v map[string]any) { v["instance"] = "production" }, kernel.ErrUsageInvalid},
		{"raw engine configuration", func(v map[string]any) { v["build"] = map[string]any{"kind": "text", "mapping": map[string]any{}} }, kernel.ErrUsageInvalid},
		{"engine script", func(v map[string]any) { v["script"] = "doc['body'].value" }, kernel.ErrUsageInvalid},
		{"shard settings", func(v map[string]any) { v["shards"] = 3 }, kernel.ErrUsageInvalid},
		{"identity substitute", func(v map[string]any) { v["documentId"] = "body" }, kernel.ErrUsageInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testkit.NewSetup(t, "")
			declaration := testkit.IndexText("schema/article", "body")
			tc.mutate(declaration.Value.(map[string]any))
			assertIndexWriterRejected(t, s, "invalid", tc.code,
				indexWriterSchema("schema/article", "definition", "body"), declaration)
			if _, err := s.Reader.ReadAddress(s.RepositoryID, declaration.Address, s.RootCommitID); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
				t.Fatalf("rejected declaration became readable: %v", err)
			}
			if _, err := s.Reader.ReadAddress(s.RepositoryID, objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, s.RootCommitID); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
				t.Fatalf("rejected batch published its Schema draft: %v", err)
			}
		})
	}
}

// Writer validates shape and field resolution; the interpreting plugin owns
// support for a build kind or option, so neither is a Writer allowlist.
func TestIndexDeclarationWriterAcceptsBuildsOutsidePluginCapabilities(t *testing.T) {
	for _, build := range []map[string]any{
		{"kind": "future-build", "model": "embed/future@v9"},
		{"kind": "text", "analyzer": "uninstalled-analyzer@v3"},
	} {
		t.Run(build["kind"].(string), func(t *testing.T) {
			s := testkit.NewSetup(t, "")
			declaration := testkit.IndexText("schema/article", "body")
			declaration.Value.(map[string]any)["build"] = build
			at := commitIndexWriterDrafts(t, s, s.Repo, "unsupported-by-plugin",
				indexWriterSchema("schema/article", "definition", "body"), declaration)
			assertIndexWriterStored(t, s, s.RepositoryID, at, declaration)
		})
	}
}

func TestIndexDeclarationWriterDoesNotClaimBusinessIndexNames(t *testing.T) {
	for _, kind := range []objectmodel.AddressKind{objectmodel.KindAspect, objectmodel.KindMember} {
		t.Run(string(kind), func(t *testing.T) {
			s := testkit.NewSetup(t, "")
			address := objectmodel.Address{Kind: kind, ObjectID: "index/business", AspectName: "index"}
			if kind == objectmodel.KindMember {
				address.MemberKey = "notes"
			}
			ordinary := objectmodel.Operation{Op: objectmodel.OpPut, Address: address,
				Value: map[string]any{"inputs": []any{map[string]any{"from": "schema/absent#/fields/missing"}}, "build": "business data"}}
			at := commitIndexWriterDrafts(t, s, s.Repo, "ordinary-index", ordinary)
			assertIndexWriterStored(t, s, s.RepositoryID, at, ordinary)
		})
	}
}

func TestIndexDeclarationWriterResolvesFixedSchemaVersions(t *testing.T) {
	s := testkit.NewSetup(t, "")
	old := commitIndexWriterDrafts(t, s, s.Repo, "old-schema", indexWriterSchema("schema/article", "definition", "body"))
	commitIndexWriterDrafts(t, s, s.Repo, "new-schema", indexWriterSchema("schema/article", "definition", "body", "summary"))
	for _, tc := range []struct {
		name, from string
		code       kernel.ErrorCode
	}{
		{"pinned existing field", "schema/article@" + string(old) + "#/fields/body", ""},
		{"unpinned new field", "schema/article#/fields/summary", ""},
		{"pinned missing field", "schema/article@" + string(old) + "#/fields/summary", kernel.ErrObjectRefUnresolved},
		{"missing fixed commit", "schema/article@missing-commit#/fields/body", kernel.ErrSchemaRevisionUnresolved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			declaration := testkit.IndexMember("versioned", "text", []string{objectmodel.IndexUseText}, tc.from)
			if tc.code != "" {
				assertIndexWriterRejected(t, s, tc.name, tc.code, declaration)
				return
			}
			at := commitIndexWriterDrafts(t, s, s.Repo, tc.name, declaration)
			assertIndexWriterStored(t, s, s.RepositoryID, at, declaration)
		})
	}
}

func TestIndexDeclarationWriterResolvesRemoteSchemaUniquelyOrExplicitly(t *testing.T) {
	s := testkit.NewSetup(t, "kr://index/consumer")
	first := testkit.MakeRepository(t, "kr://index/source-a")
	if err := s.Store.Add(first); err != nil {
		t.Fatal(err)
	}
	firstAt := commitIndexWriterDrafts(t, s, first, "first-schema", indexWriterSchema("schema/shared", "definition", "body"))
	bare := testkit.IndexMember("remote", "text", []string{objectmodel.IndexUseText}, "schema/shared#/fields/body")
	at := commitIndexWriterDrafts(t, s, s.Repo, "unique-remote", bare)
	assertIndexWriterStored(t, s, s.RepositoryID, at, bare)
	second := testkit.MakeRepository(t, "kr://index/source-b")
	if err := s.Store.Add(second); err != nil {
		t.Fatal(err)
	}
	// Equal Schema bodies still have ambiguous provenance when no source is named.
	secondAt := commitIndexWriterDrafts(t, s, second, "second-schema", indexWriterSchema("schema/shared", "definition", "body"))
	assertIndexWriterRejected(t, s, "ambiguous-remote", kernel.ErrObjectIDConflict, bare)
	for _, from := range []string{
		"kc://index/source-a/schema/shared#/fields/body",
		"kc://index/source-a@" + string(firstAt) + "/schema/shared#/fields/body",
	} {
		declaration := testkit.IndexMember("explicit", "text", []string{objectmodel.IndexUseText}, from)
		at = commitIndexWriterDrafts(t, s, s.Repo, from, declaration)
		assertIndexWriterStored(t, s, s.RepositoryID, at, declaration)
	}
	missing := testkit.IndexMember("missing-repository", "text", []string{objectmodel.IndexUseText}, "kc://index/absent/schema/shared#/fields/body")
	assertIndexWriterRejected(t, s, "missing-repository", kernel.ErrObjectRefUnresolved, missing)
	if testkit.MustHead(t, first, snapshot.DefaultRef) != firstAt || testkit.MustHead(t, second, snapshot.DefaultRef) != secondAt {
		t.Fatal("declaration publication changed a source Repository")
	}
}

func TestIndexDeclarationWriterValidatesInheritedSchemaRef(t *testing.T) {
	s := testkit.NewSetup(t, "")
	declaration := testkit.IndexText("schema/article", "body")
	at := commitIndexWriterDrafts(t, s, s.Repo, "initial-declaration",
		indexWriterSchema("schema/article", "definition", "body", "summary"), declaration)
	for _, tc := range []struct {
		name string
		from string
		code kernel.ErrorCode
	}{
		{"missing field", "schema/article#/fields/absent", kernel.ErrObjectRefUnresolved},
		{"malformed locator", "schema/article#/body", kernel.ErrUsageInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			replacement := testkit.IndexMember(declaration.Address.MemberKey, "text", []string{objectmodel.IndexUseText}, tc.from)
			replacement.SchemaRef = ""
			assertIndexWriterRejected(t, s, tc.name, tc.code, replacement)
			assertIndexWriterStored(t, s, s.RepositoryID, at, declaration)
		})
	}
	replacement := testkit.IndexMember(declaration.Address.MemberKey, "text", []string{objectmodel.IndexUseText}, "schema/article#/fields/summary")
	replacement.SchemaRef = ""
	at = commitIndexWriterDrafts(t, s, s.Repo, "valid-inherited", replacement)
	replacement.SchemaRef = declaration.SchemaRef
	assertIndexWriterStored(t, s, s.RepositoryID, at, replacement)
}

func indexWriterSchema(id objectmodel.ObjectID, aspect string, names ...string) objectmodel.Operation {
	fields := map[string]any{}
	for _, name := range names {
		fields[name] = map[string]any{"type": "string"}
	}
	return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: id},
		Value: map[string]any{"entity": "Article", "aspect": aspect, "fields": fields}}
}

func commitIndexWriterDrafts(t *testing.T, s testkit.Setup, target snapshot.Store, command string, operations ...objectmodel.Operation) kernel.CommitID {
	t.Helper()
	before := testkit.MustHead(t, target, snapshot.DefaultRef)
	receipt, err := s.Writer.Commit(command, objectmodel.ChangeSet{TargetRepository: target.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: before, ExpectedTargetCommit: before, Operations: operations})
	if err != nil {
		t.Fatal(err)
	}
	if head := testkit.MustHead(t, target, snapshot.DefaultRef); head != receipt.Result.CommitID || head == before {
		t.Fatalf("accepted declaration did not publish its new version: head=%s receipt=%s before=%s", head, receipt.Result.CommitID, before)
	}
	return receipt.Result.CommitID
}

func assertIndexWriterRejected(t *testing.T, s testkit.Setup, command string, code kernel.ErrorCode, operations ...objectmodel.Operation) {
	t.Helper()
	before := testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
	_, err := s.Writer.Commit(command, objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: before, ExpectedTargetCommit: before, Operations: operations})
	testkit.ExpectCode(t, err, code)
	if head := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); head != before {
		t.Fatalf("rejected declaration changed HEAD: got %s, want %s", head, before)
	}
}

func assertIndexWriterStored(t *testing.T, s testkit.Setup, repo kernel.RepositoryID, at kernel.CommitID, want objectmodel.Operation) {
	t.Helper()
	got, err := s.Reader.ReadAddress(repo, want.Address, at)
	if err != nil {
		t.Fatal(err)
	}
	if got.Address != want.Address || got.Commit != at || kernel.CanonicalDigest(got.Value) != kernel.CanonicalDigest(want.Value) {
		t.Fatalf("published declaration did not retain its identity, version and body: %#v", got)
	}
	if len(got.Declarations) != 1 || got.Declarations[0].SchemaRef != want.SchemaRef {
		t.Fatalf("published declaration lost its adopted Schema: %#v", got.Declarations)
	}
}
