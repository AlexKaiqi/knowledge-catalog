package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	discoverydatasets "kc/discovery/datasets"
	apphome "kc/home"
	"kc/internal/testkit"
	inv "kc/invocation"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
	snapshotservice "kc/snapshot/service"
)

func TestNonDeploymentSnapshotConsumptionCannotAdoptPortableRecipe(t *testing.T) {
	for _, operation := range []string{"resolve", "file-read"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			source := testkit.MakeTreeStore(t, "kr://fixture/private-source")
			tree, _ := snapshot.TreeStoreOf(source)
			root := testkit.MustHead(t, source, snapshot.DefaultRef)
			commit, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{TargetRepository: source.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: root, ExpectedTargetCommit: root, Changes: []snapshot.TreeChange{{Path: "private.txt", Content: []byte("private source content")}}})
			if err != nil {
				t.Fatal(err)
			}
			stores := snapshot.NewRegistry()
			if err := stores.Add(source); err != nil {
				t.Fatal(err)
			}
			authority := testkit.MakeTreeStore(t, "kr://fixture/registry")
			registry, err := dataset.CreateSnapshotRegistry(authority, "")
			if err != nil {
				t.Fatal(err)
			}
			cat, err := dataset.NewService(stores, registry)
			if err != nil {
				t.Fatal(err)
			}
			if err := cat.RegisterRepository(source.ID()); err != nil {
				t.Fatal(err)
			}
			recipe := dataset.DatasetRecipe{Name: "files", Files: []dataset.DatasetFileMount{{Repository: string(source.ID()), Selector: snapshot.DefaultRef, File: "private.txt", Target: "docs/private.txt"}}}
			raw, err := dataset.FormatDatasetRecipe(recipe)
			if err != nil {
				t.Fatal(err)
			}
			file := inv.LocalWorkspaceFile(dir, "files")
			if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			policy := inv.AllowFile{Rules: []access.Rule{
				{ID: "consumer", Principal: "consumer", Dataset: "files", Actions: []string{"dataset.resolve", "file.read"}},
				{ID: "target-manager", Principal: "manager", Dataset: "files", Actions: []string{"dataset.manage"}},
			}}
			if err := inv.WriteAllow(dir, policy); err != nil {
				t.Fatal(err)
			}
			// The hand-built Home has a single file-only source and no Reader, so
			// its publication can reference no object outside the selection.
			// home.Open wires the real Serving.FreezeDependencies in production.
			preparation := discoverydatasets.Preparation{Dependencies: func(dataset.ResolvedDataset) (map[kernel.RepositoryID]kernel.CommitID, error) { return nil, nil }}
			opened := &Home{Dir: dir, Store: stores, Datasets: cat, Registry: registry, DatasetPreparation: preparation, File: apphome.HomeFile{Registry: apphome.HomeRegistry{ID: string(authority.ID())}}}
			facade := &httpFacade{home: dir, readHome: opened, options: HTTPServerOptions{AuthMode: "local"}}
			handler := facade.snapshotStoreService()
			before := dataset.NormalizeState(cat.DumpState())
			head := testkit.MustHead(t, authority, snapshot.DefaultRef)
			pin := dataset.ResolvedDataset{SetID: "files", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{source.ID(): commit}}
			target, payload := "/snapshot/v1/datasets/files/resolve", any(map[string]any{})
			if operation == "file-read" {
				target, payload = "/snapshot/v1/files/file:read", map[string]any{"dataset": "files", "pin": pin, "path": "docs/private.txt"}
			}
			body, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			failure := grantRequestForTest(t, handler, "consumer", http.MethodPost, target, string(body), http.StatusBadRequest)
			if failure["error"].(map[string]any)["code"] != string(kernel.ErrDatasetInvalid) {
				t.Fatalf("missing accepted Dataset changed error: %+v", failure)
			}
			if !reflect.DeepEqual(dataset.NormalizeState(cat.DumpState()), before) || testkit.MustHead(t, authority, snapshot.DefaultRef) != head {
				t.Fatal("consumer adopted an unaccepted portable recipe")
			}
			define, err := json.Marshal(map[string]any{"dataset": "files", "revision": 1, "sources": recipe.Sources()})
			if err != nil {
				t.Fatal(err)
			}
			grantRequestForTest(t, handler, "manager", http.MethodPost, "/snapshot/v1/datasets", string(define), http.StatusForbidden)
			if testkit.MustHead(t, authority, snapshot.DefaultRef) != head {
				t.Fatal("target-only manager adopted a private source")
			}
			policy.Rules = append(policy.Rules, access.Rule{ID: "source-permit", Principal: "manager", Repo: string(source.ID()), Dataset: "files", Ref: snapshot.DefaultRef, Actions: []string{access.RepositoryDatasetPublish}})
			if err := inv.WriteAllow(dir, policy); err != nil {
				t.Fatal(err)
			}
			grantRequestForTest(t, handler, "manager", http.MethodPost, "/snapshot/v1/datasets", string(define), http.StatusOK)
			policy.Rules = policy.Rules[:2]
			if err := inv.WriteAllow(dir, policy); err != nil {
				t.Fatal(err)
			}
			acceptedHead := testkit.MustHead(t, authority, snapshot.DefaultRef)
			resolved := grantRequestForTest(t, handler, "consumer", http.MethodPost, "/snapshot/v1/datasets/files/resolve", `{}`, http.StatusOK)
			readBody, err := json.Marshal(map[string]any{"dataset": "files", "pin": resolved, "path": "docs/private.txt"})
			if err != nil {
				t.Fatal(err)
			}
			read := grantRequestForTest(t, handler, "consumer", http.MethodPost, "/snapshot/v1/files/file:read", string(readBody), http.StatusOK)
			rawRead, err := json.Marshal(read)
			if err != nil {
				t.Fatal(err)
			}
			var delivered snapshotservice.FileReadResponse
			if err := json.Unmarshal(rawRead, &delivered); err != nil || string(delivered.Content) != "private source content" || testkit.MustHead(t, authority, snapshot.DefaultRef) != acceptedHead {
				t.Fatalf("accepted Dataset consumption changed or republished: %+v, %v", delivered, err)
			}
		})
	}
}
