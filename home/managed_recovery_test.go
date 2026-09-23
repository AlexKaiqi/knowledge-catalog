package home

import (
	"encoding/json"
	"kc/knowledge"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"kc/kernel"
	"kc/snapshot"
)

func TestManagedGiteaOfflineRecoveryPreservesControlPlaneAddresses(t *testing.T) {
	var offline atomic.Bool
	var remoteCalls atomic.Int64
	cfg, ws, creates := managedFixtureWithInterceptor(t, func(w http.ResponseWriter, r *http.Request) bool {
		remoteCalls.Add(1)
		if offline.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return true
		}
		return false
	})
	result, err := ws.CreateNamedManagedRepository(ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, Principal: "kaiqidong", Name: "团队规范"}, func(ManagedRepositoryGrant) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.ManagementURL == "" {
		t.Fatal("provisioning omitted management URL")
	}
	if err := ws.Close(); err != nil {
		t.Fatal(err)
	}
	offline.Store(true)
	before := remoteCalls.Load()
	restarted, err := OpenDeployment(cfg)
	if err != nil {
		t.Fatalf("offline managed Gitea prevented control-plane recovery: %v", err)
	}
	defer restarted.Close()
	rows, err := restarted.ListManagedRepositories("kaiqidong")
	if err != nil || len(rows) != 1 || rows[0].ManagementURL != result.ManagementURL {
		t.Fatalf("mine lost durable address: %#v %v", rows, err)
	}
	detail, err := restarted.GetOwnedManagedRepository("kaiqidong", result.RepositoryID)
	if err != nil || detail.ManagementURL != result.ManagementURL || detail.Head != result.Head {
		t.Fatalf("detail lost durable allocation: %#v %v", detail, err)
	}
	if remoteCalls.Load() != before {
		t.Fatal("recovery or control-plane metadata probed the offline source")
	}
	source, ok := restarted.Store.Get(kernel.RepositoryID(result.RepositoryID))
	if !ok {
		t.Fatal("managed binding was not restored")
	}
	if _, err := source.Head(snapshot.DefaultRef); err == nil {
		t.Fatal("offline source returned cached head as authority")
	}
	offline.Store(false)
	if head, err := source.Head(snapshot.DefaultRef); err != nil || head != result.Head {
		t.Fatalf("restored handle did not recover online: %s %v", head, err)
	}
	if *creates != 1 {
		t.Fatal("recovery provisioned another repository")
	}
}

func TestManagedGiteaRecoveredHandleRejectsAuthorityReplacementBeforeReadOrWrite(t *testing.T) {
	for _, problem := range []string{"backend", "allocation", "initial-commit"} {
		t.Run(problem, func(t *testing.T) {
			var tampered atomic.Bool
			var mutations atomic.Int64
			var record managedRecord
			cfg, ws, _ := managedFixtureWithInterceptor(t, func(w http.ResponseWriter, request *http.Request) bool {
				if request.Method != http.MethodGet {
					mutations.Add(1)
				}
				if !tampered.Load() {
					return false
				}
				endpoint, _ := url.Parse(record.Binding.DSN)
				repoPath := "/api/v1/repos" + endpoint.Path
				if request.URL.Path == repoPath && problem != "initial-commit" {
					backend, _ := strconv.ParseInt(record.BackendID, 10, 64)
					description := "kc-managed:" + record.AllocationID + ":" + record.Binding.ID
					if problem == "backend" {
						backend++
					} else {
						description += "-replacement"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"id": backend, "description": description, "default_branch": "main", "empty": false})
					return true
				}
				if problem == "initial-commit" {
					if request.URL.Path == repoPath+"/branches/main" {
						_ = json.NewEncoder(w).Encode(map[string]any{"name": "main", "commit": map[string]any{"id": "new-head"}})
						return true
					}
					if request.URL.Path == repoPath+"/commits/"+string(record.Head) || request.URL.Path == repoPath+"/git/commits/"+string(record.Head) {
						w.WriteHeader(http.StatusNotFound)
						return true
					}
				}
				return false
			})
			result, err := ws.CreateNamedManagedRepository(ManagedRepositoryRequest{CatalogID: cfg.Catalogs[0].ID, Principal: "kaiqidong", Name: "notes"}, func(ManagedRepositoryGrant) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			records, err := loadManagedRecords(cfg.StateDir)
			if err != nil || len(records) != 1 {
				t.Fatal("missing durable allocation")
			}
			record = records[0]
			if err := ws.Close(); err != nil {
				t.Fatal(err)
			}
			restarted, err := OpenDeployment(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer restarted.Close()
			source, _ := restarted.Store.Get(kernel.RepositoryID(result.RepositoryID))
			if _, err := source.Head(snapshot.DefaultRef); err != nil {
				t.Fatal(err)
			}
			tampered.Store(true)
			before := mutations.Load()
			if _, err := source.Head(snapshot.DefaultRef); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("replacement was readable: %v", err)
			}
			tree, ok := snapshot.TreeStoreOf(source)
			if !ok {
				t.Fatal("Gitea tree capability disappeared")
			}
			if _, err := tree.ReadFile("example.okf", result.Head); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("replacement file was readable: %v", err)
			}
			if err := source.CreateRef("refs/heads/proposal", result.Head); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("replacement accepted a ref mutation: %v", err)
			}
			if _, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{TargetRepository: source.ID(), BaseCommit: result.Head, ExpectedTargetCommit: result.Head, Changes: []snapshot.TreeChange{{Path: "file", Content: []byte("content")}}}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("replacement accepted a tree write: %v", err)
			}
			if mutations.Load() != before {
				t.Fatal("invalid authority received a mutation")
			}
			if source.HasCommit(result.Head) {
				t.Fatal("replacement validated a saved commit")
			}
			if _, err := restarted.GetOwnedManagedRepository("kaiqidong", result.RepositoryID); err != nil {
				t.Fatal("durable metadata depended on replacement authority")
			}
		})
	}
}

func TestManagedRestoreKeepsDriverCapabilities(t *testing.T) {
	original := authorityDrivers["lakefs"]
	native := knowledge.NewSystemRepository()
	head, err := native.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	opened := 0
	driver := original
	driver.managedOpen = func(RepositoryBinding, string, string) (snapshot.Store, error) { opened++; return native, nil }
	authorityDrivers["lakefs"] = driver
	defer func() { authorityDrivers["lakefs"] = original }()
	restored, err := restoreManagedSource(managedRecord{Binding: RepositoryBinding{Driver: "lakefs"}, Head: head})
	if err != nil {
		t.Fatal(err)
	}
	if restored != native || opened != 1 {
		t.Fatal("lakeFS restoration replaced its driver handle")
	}
	if _, ok := restored.(knowledge.NativeRepository); !ok {
		t.Fatal("native knowledge capability was erased")
	}
	if _, ok := restored.(snapshot.TreeStore); ok {
		t.Fatal("restoration invented a TreeStore capability")
	}
}
