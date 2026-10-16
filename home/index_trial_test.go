package home_test

import (
	"path/filepath"
	"strings"
	"testing"

	declarative "kc/declarativeindex/plugin"
	retrieval "kc/discovery"
	consumerplugin "kc/discovery/plugin"
	index "kc/discovery/projection"
	apphome "kc/home"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
)

func trialSetup(t *testing.T) (testkit.Setup, *apphome.Home, kernel.CommitID, objectmodel.Operation) {
	t.Helper()
	s := testkit.NewSetup(t, "kr://trial/public")
	op := testkit.IndexMember("good", "text", []string{"text"}, "schema/article#/fields/body")
	r, err := s.Writer.Commit("seed", objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"},
				Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}}}}, op,
		}})
	if err != nil {
		t.Fatal(err)
	}
	entry := declarative.Entry(declarative.Options{Directory: filepath.Join(t.TempDir(), "projection"),
		Engine: func(string) index.EngineOpener {
			return func(string, kernel.RepositoryID) (index.Engine, error) {
				t.Error("trial opened an engine")
				return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "trial must not open an engine")
			}
		}, Lookup: func(id kernel.RepositoryID) (objectmodel.Repository, error) {
			return s.Reader.Require(id, kernel.ErrObjectRefUnresolved)
		},
	})
	plugins, err := consumerplugin.OpenIndexPlugins(consumerplugin.PluginsConfig{"declarative-index": {Instances: map[string]consumerplugin.PluginInstanceConfig{
		"configured": {Catalog: declarative.Catalog},
	}}}, consumerplugin.PluginScope{Repositories: []kernel.RepositoryID{s.RepositoryID}}, map[string]consumerplugin.IndexPluginEntry{"declarative-index": entry})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if plugins.Running() || testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != r.Result.CommitID {
			t.Error("trial started maintenance or changed HEAD")
		}
		if err := plugins.Close(); err != nil {
			t.Error(err)
		}
	})
	return s, &apphome.Home{Store: s.Store, Reader: s.Reader, Writer: s.Writer, Plugins: plugins}, r.Result.CommitID, op
}

func trialDocument(t *testing.T, path string, op objectmodel.Operation) retrieval.DeclarationDocument {
	t.Helper()
	content, err := repofile.Serialize(op.Address, "", op.SchemaRef, nil, op.Value)
	if err != nil {
		t.Fatal(err)
	}
	return retrieval.DeclarationDocument{Path: path, Content: content}
}

func allowAll(kernel.RepositoryID) error { return nil }

// Trial reports one verdict per member: a sibling's missing field must not
// become the verdict of a structurally valid member.
func TestTrialJudgesEachMemberIndependently(t *testing.T) {
	s, ws, at, good := trialSetup(t)
	bad := testkit.IndexMember("bad", "text", []string{"text"}, "schema/article#/fields/missing")
	result, err := ws.TrialIndexDeclarations("declarative-index", retrieval.DeclarationTrialRequest{
		Repository: s.RepositoryID, Commit: at, Documents: []retrieval.DeclarationDocument{
			trialDocument(t, "good.yaml", good), trialDocument(t, "bad.yaml", bad),
		}}, allowAll)
	if err != nil {
		t.Fatal(err)
	}
	members := map[string]retrieval.DeclarationTrialMember{}
	for _, member := range result.Members {
		members[member.Member.MemberKey] = member
	}
	if len(result.Members) != 2 || members["bad"].Status != retrieval.DeclarationInvalid || !strings.Contains(members["bad"].Reason, "missing") {
		t.Fatalf("missing field member was not invalid: %#v", result.Members)
	}
	if members["good"].Status != "supported" || members["good"].Reason != "" || members["good"].Code != "" || members["good"].Instance != "configured" {
		t.Fatalf("valid sibling was judged by another member: %#v", result.Members)
	}
}

// A file updating an existing member may omit schema_ref exactly as Writer
// allows; trial and Writer must agree on the same material.
func TestTrialInheritsStoredSchemaRefLikeWriter(t *testing.T) {
	s, ws, at, inherited := trialSetup(t)
	inherited.SchemaRef = ""
	cs := objectmodel.ChangeSet{TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: at, ExpectedTargetCommit: at, Operations: []objectmodel.Operation{inherited}}
	if _, _, err := s.Writer.CheckIndexDeclaration(cs, inherited.Address); err != nil {
		t.Fatalf("Writer refused inherited Schema: %v", err)
	}
	result, err := ws.TrialIndexDeclarations("declarative-index", retrieval.DeclarationTrialRequest{
		Repository: s.RepositoryID, Commit: at, Documents: []retrieval.DeclarationDocument{trialDocument(t, "good.yaml", inherited)},
	}, allowAll)
	if err != nil {
		t.Fatalf("trial rejected a Writer-valid inherited Schema: %v", err)
	}
	if len(result.Members) != 1 || result.Members[0].Status != "supported" || result.Members[0].Rebuild == nil || *result.Members[0].Rebuild {
		t.Fatalf("inherited member judged differently from Writer: %#v", result.Members)
	}
	// Without a stored declaration there is nothing to inherit.
	fresh := inherited
	fresh.Address.MemberKey = "fresh"
	if _, err := ws.TrialIndexDeclarations("declarative-index", retrieval.DeclarationTrialRequest{
		Repository: s.RepositoryID, Commit: at, Documents: []retrieval.DeclarationDocument{trialDocument(t, "fresh.yaml", fresh)},
	}, allowAll); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("member without written or stored schema_ref was accepted: %v", err)
	}
}
