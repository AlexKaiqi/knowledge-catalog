package invocation

import (
	retrieval "kc/discovery"
	apphome "kc/home"
	"kc/invoke"
	"kc/kernel"
	objectmodel "kc/object"
	"os"
	"path/filepath"
	"testing"
)

func TestAllowedRepoReadFailsClosedWhenPolicyCannotBeRead(t *testing.T) {
	homeDir := t.TempDir()
	// ReadAllow expects a JSON file here. A directory produces a stable read
	// error even when the test process has elevated filesystem permissions.
	if err := os.Mkdir(filepath.Join(homeDir, "allow.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	flags := map[string]invoke.FlagValue{"as": "alice"}
	if AllowedRepoRead(homeDir, flags, "kr://acme/private/core", "") {
		t.Fatal("an unreadable authorization policy must fail closed")
	}
}

func TestDatasetFileReadDoesNotImplyRepoObject(t *testing.T) {
	rules := []AllowRule{{
		ID: "ws", Principal: "bot", Actions: []string{"file.read"},
		Dataset: "agent",
	}}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "object.read", Repo: "kr://acme/public/core",
	}); ok {
		t.Fatal("dataset file.read must not grant repository object.read")
	}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "object.search", Repo: "kr://acme/public/core",
	}); ok {
		t.Fatal("dataset file.read must not grant repository object.search")
	}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "dataset.resolve",
		Dataset: "agent",
	}); ok {
		t.Fatal("dataset file.read must not imply dataset.resolve")
	}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "file.read",
		Dataset: "agent",
	}); !ok {
		t.Fatal("file.read must still match itself on the dataset")
	}
}

func TestDatasetConsumeDoesNotImplyObjectActions(t *testing.T) {
	rules := []AllowRule{{
		ID: "ws", Principal: "bot", Actions: []string{"file.read"},
		Dataset: "agent",
	}}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "object.read", Repo: "kr://acme/public/core",
	}); ok {
		t.Fatal("file.read must not grant object.read")
	}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "object.search",
		Dataset: "agent",
	}); ok {
		t.Fatal("file.read must not grant object.search")
	}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "dataset.consume",
		Dataset: "agent",
	}); ok {
		t.Fatal("dataset.consume is not a live alias")
	}
	if _, ok := MatchAllow(rules, AllowQuery{
		Principal: "bot", Action: "file.read",
		Dataset: "agent",
	}); !ok {
		t.Fatal("file.read must still match itself")
	}
}

func TestReadAllowRejectsRetiredKsetFieldAndActions(t *testing.T) {
	homeDir := t.TempDir()
	path := filepath.Join(homeDir, "allow.json")
	for _, body := range []string{
		`{"version":2,"rules":[{"id":"old","principal":"bot","actions":["file.read"],"kset":"agent"}]}`,
		`{"version":2,"rules":[{"id":"old","principal":"bot","actions":["kset.consume"],"dataset":"agent"}]}`,
	} {
		if err := os.WriteFile(path, []byte(body+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadAllow(homeDir); err == nil {
			t.Fatalf("allow.json must reject retired kset aliases: %s", body)
		}
	}
}

func TestValidateActionsRejectsRetiredDatasetNames(t *testing.T) {
	for _, action := range []string{"kset.consume", "kset.manage", "kset.resolve", "dataset.consume"} {
		if err := ValidateActions([]string{action}); err == nil {
			t.Fatalf("%s must not be grantable", action)
		}
	}
	if err := ValidateActions([]string{"file.read", "dataset.resolve", "dataset.manage"}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizeDatasetObjectSeparatesConsumeFromSearch(t *testing.T) {
	workspace := "agent"
	repo := "kr://acme/public/core"
	consume := AllowRule{
		ID: "ws", Principal: "bot", Actions: []string{"file.read"},
		Dataset: workspace,
	}
	searchRepo := AllowRule{
		ID: "search", Principal: "bot", Actions: []string{"object.search"}, Repo: repo,
	}
	catalogRead := AllowRule{
		ID: "cat", Principal: "bot", Actions: []string{"inventory.read"}}
	searchQ := AllowQuery{
		Principal: "bot", Action: "object.search", Dataset: workspace,
	}
	if err := AuthorizeWorkspaceObject([]AllowRule{consume}, searchQ); err != nil {
		t.Fatalf("dataset file.read must admit pin object.search: %v", err)
	}
	if err := AuthorizeWorkspaceObject([]AllowRule{catalogRead, searchRepo}, searchQ); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("inventory.read must not skip named-dataset file.read: %v", err)
	}
	readQ := searchQ
	readQ.Action = "object.read"
	if err := AuthorizeWorkspaceObject([]AllowRule{consume}, readQ); err != nil {
		t.Fatalf("dataset file.read admits pin object.read: %v", err)
	}
	if err := AuthorizeWorkspaceObject([]AllowRule{consume}, AllowQuery{
		Principal: "bot", Action: "object.search", Repo: repo,
	}); err != ErrNotWorkspaceObject {
		t.Fatalf("--repo SEARCH is not a named-dataset gate: %v", err)
	}
}

func TestDeliverSearchHitStripsSearchOnlyGrant(t *testing.T) {
	homeDir := t.TempDir()
	if err := WriteAllow(homeDir, AllowFile{Rules: []AllowRule{
		{ID: "s", Principal: "taihu:alice", Actions: []string{"object.search"}, Repo: "kr://scene/object"},
	}}); err != nil {
		t.Fatal(err)
	}
	flags := map[string]invoke.FlagValue{"as": "taihu:alice", "dataset": "scene-set"}
	hit := retrieval.ObjectHit{Object: objectmodel.ObjectValue{
		ObjectRef:  objectmodel.ObjectRef{Repository: "kr://scene/object", Object: "metric/gmv"},
		Repository: "kr://scene/object",
		Address:    objectmodel.Address{ObjectID: "metric/gmv"},
		Value:      map[string]any{"definition": map[string]any{"name": "Gross merchandise value"}},
	}}
	out, err := DeliverSearchHit(homeDir, flags, hit)
	if err != nil {
		t.Fatal(err)
	}
	if out.Object.Value != nil {
		t.Fatalf("search-only must strip Canonical: %#v", out.Object)
	}
}

func TestDeliverSearchHitKeepsDatasetFileReadBody(t *testing.T) {
	homeDir := t.TempDir()
	if err := WriteAllow(homeDir, AllowFile{Rules: []AllowRule{
		{ID: "c", Principal: "taihu:alice", Actions: []string{"file.read"}, Dataset: "scene-set"},
	}}); err != nil {
		t.Fatal(err)
	}
	flags := map[string]invoke.FlagValue{"as": "taihu:alice", "dataset": "scene-set"}
	hit := retrieval.ObjectHit{Object: objectmodel.ObjectValue{
		ObjectRef:  objectmodel.ObjectRef{Repository: "kr://scene/object", Object: "metric/gmv"},
		Repository: "kr://scene/object",
		Address:    objectmodel.Address{ObjectID: "metric/gmv"},
		Value:      map[string]any{"definition": map[string]any{"name": "Gross merchandise value"}},
	}}
	out, err := DeliverSearchHit(homeDir, flags, hit)
	if err != nil {
		t.Fatal(err)
	}
	if out.Object.Value == nil {
		t.Fatal("dataset file.read must deliver listed-file bodies without repository object.read")
	}
}

func TestDeliverSearchHitStripsInstanceMisTaggedAsSystemRepository(t *testing.T) {
	homeDir := t.TempDir()
	if err := WriteAllow(homeDir, AllowFile{Rules: []AllowRule{
		{ID: "s", Principal: "taihu:alice", Actions: []string{"object.search"}, Repo: "kr://scene/object"},
	}}); err != nil {
		t.Fatal(err)
	}
	flags := map[string]invoke.FlagValue{"as": "taihu:alice"}
	hit := retrieval.ObjectHit{Object: objectmodel.ObjectValue{
		ObjectRef:  objectmodel.ObjectRef{Repository: objectmodel.SystemRepositoryID, Object: "metric/gmv"},
		Repository: objectmodel.SystemRepositoryID,
		Address:    objectmodel.Address{ObjectID: "metric/gmv"},
		Value:      map[string]any{"definition": map[string]any{"name": "Gross merchandise value"}},
	}}
	out, err := DeliverSearchHit(homeDir, flags, hit)
	if err != nil {
		t.Fatal(err)
	}
	if out.Object.Value != nil {
		t.Fatalf("undeclared repository must strip Canonical like any other repo: %#v", out.Object)
	}
}

func TestDeliverSearchHitKeepsDeclaredRepositoryBody(t *testing.T) {
	homeDir := t.TempDir()
	if err := apphome.WriteRepositoryAccess(homeDir, apphome.RepositoryAccessFile{Repositories: []apphome.RepositoryAccess{apphome.SystemRepositoryAccess()}}); err != nil {
		t.Fatal(err)
	}
	flags := map[string]invoke.FlagValue{"as": "taihu:alice"}
	hit := retrieval.ObjectHit{Object: objectmodel.ObjectValue{
		ObjectRef:  objectmodel.ObjectRef{Repository: objectmodel.SystemRepositoryID, Object: "schema/meta/schema-definition/v1"},
		Repository: objectmodel.SystemRepositoryID,
		Address:    objectmodel.Address{ObjectID: "schema/meta/schema-definition/v1"},
		Value:      map[string]any{"title": "schema definition"},
	}}
	out, err := DeliverSearchHit(homeDir, flags, hit)
	if err != nil {
		t.Fatal(err)
	}
	if out.Object.Value == nil {
		t.Fatal("authenticated default must deliver the declared repository body")
	}
}
