package cli_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"kc/cli"
	apphome "kc/home"
	"kc/internal/testkit"
)

// This starts with no user account or per-repository binding. Deployment
// policy is the only precondition; every user action crosses CLI -> Server.
func TestManagedProductHumanSelfServiceOnLiveGitea(t *testing.T) {
	managedHumanJourney(t, "gitea", nil)
}

func TestManagedProductHumanSelfServiceOnDolt(t *testing.T) {
	// Literal public argv belongs to this named formal journey. These
	// callbacks use saved logins and never reset client configuration.
	managedHumanJourney(t, "dolt", &managedShareCommands{
		add: func(repository, principal, actions string) kcRunResult {
			return kcRunResultFrom(cli.Run([]string{"catalog", "repo", "share", "add", "--repo", repository, "--principal", principal, "--action", actions}), "catalog repo share add")
		},
		list: func(repository string) kcRunResult {
			return kcRunResultFrom(cli.Run([]string{"catalog", "repo", "share", "list", "--repo", repository}), "catalog repo share list")
		},
		remove: func(repository, shareID string) kcRunResult {
			return kcRunResultFrom(cli.Run([]string{"catalog", "repo", "share", "remove", "--repo", repository, "--id", shareID}), "catalog repo share remove")
		},
	})
}

type managedShareCommands struct {
	add    func(repository, principal, actions string) kcRunResult
	list   func(repository string) kcRunResult
	remove func(repository, shareID string) kcRunResult
}

func managedHumanJourney(t *testing.T, driver string, sharing *managedShareCommands) {
	isolateClientCredentials(t)
	t.Setenv("KC_REQUIRE_LIVE_ADAPTERS", "1")
	t.Setenv("KC_SERVER_URL", "")
	base, token := "", ""
	if driver == "gitea" {
		base, token, _ = testkit.GiteaEndpoint(t)
		t.Setenv("KC_GITEA_TOKEN", token)
	}
	cfg, path := declaredDeployment(t, false)
	var guard sync.RWMutex
	var current http.Handler
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		guard.RLock()
		defer guard.RUnlock()
		current.ServeHTTP(w, r)
	}))
	defer server.Close()
	pool := apphome.ManagedRepositoryConfig{Driver: driver, PublicURL: server.URL, CreatorActions: []string{"writer.preview", "writer.commit", "writer.receipt.read", "knowledge.read", "knowledge.provenance", "knowledge.schema.read", "workspace.resolve", "workspace.consume", "file.read", "repository.metadata.read", "repository.shares.manage"}, ShareActions: []string{"knowledge.read", "knowledge.provenance", "workspace.resolve", "workspace.consume", "file.read"}}
	if driver == "gitea" {
		pool.DSN = base + "/kc"
	} else {
		pool.Root = filepath.Join(filepath.Dir(cfg.StateDir), "hosted-dolt")
	}
	cfg.ManagedStores = map[string]apphome.ManagedRepositoryConfig{driver: pool}
	cfg.Admission = &apphome.AdmissionConfig{Enabled: true, Catalog: cfg.Catalogs[0].ID, Principals: []string{"kaiqidong", "consumer"}, Actions: []string{"catalog.repositories.create"}}
	writeDeployment(t, path, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	open := func() {
		var err error
		current, err = cli.HTTPHandlerFromConfig(path, cli.HTTPServerOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}
	open()
	defer func() { _ = current.(interface{ Close() error }).Close() }()
	run := func(args ...string) kcRunResult { return kcRunResultFrom(cli.Run(args), publicCommandPath(args)) }
	if sharing == nil {
		sharing = &managedShareCommands{
			add: func(repository, principal, actions string) kcRunResult {
				return run("catalog", "repo", "share", "add", "--repo", repository, "--principal", principal, "--action", actions)
			},
			list: func(repository string) kcRunResult {
				return run("catalog", "repo", "share", "list", "--repo", repository)
			},
			remove: func(repository, shareID string) kcRunResult {
				return run("catalog", "repo", "share", "remove", "--repo", repository, "--id", shareID)
			},
		}
	}
	providerConfig := t.TempDir()
	consumerConfig := t.TempDir()
	t.Setenv("KC_CONFIG_DIR", providerConfig)
	login := asMap(t, body(t, run("login", "--mode", "local", "--server", server.URL, "--as", "kaiqidong")))
	if login["principal"] != "kaiqidong" {
		t.Fatalf("public username changed: %#v", login)
	}
	body(t, run("admission", "request"))
	expectCode(t, run("catalog", "show", "--catalog", cfg.Catalogs[0].ID), "FORBIDDEN")
	created := asMap(t, body(t, run("catalog", "repo", "create", "--name", "团队知识")))
	if _, leaked := created["commandId"]; leaked {
		t.Fatalf("ordinary creation exposes its internal allocation command: %#v", created)
	}
	repository, _ := created["repositoryId"].(string)
	if repository == "" || created["owner"] != "kaiqidong" || created["name"] != "团队知识" || created["status"] != "APPLIED" {
		t.Fatalf("create did not allocate a human-owned repository: %#v", created)
	}
	management, _ := created["managementURL"].(string)
	if !strings.HasPrefix(management, server.URL+"/repositories/") {
		t.Fatalf("management address is not delivered by this service: %s", management)
	}
	parsed, err := url.Parse(management)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/repositories/")); err != nil || got != repository {
		t.Fatalf("management address lost repository identity: %s %v", got, err)
	}
	resp, err := http.Get(management)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("management page unavailable: %d", resp.StatusCode)
	}
	if driver == "gitea" {
		request, _ := http.NewRequest(http.MethodGet, base+"/api/v1/users/kaiqidong", nil)
		request.Header.Set("Authorization", "token "+token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var account struct {
			Login string `json:"login"`
		}
		_ = json.NewDecoder(response.Body).Decode(&account)
		response.Body.Close()
		if response.StatusCode != 200 || account.Login != "kaiqidong" {
			t.Fatalf("Snapshot account is not the KC username: %d %s", response.StatusCode, account.Login)
		}
	}
	mine := asMap(t, body(t, run("catalog", "repo", "list", "--mine")))
	items, _ := mine["repositories"].([]any)
	if len(items) != 1 || asMap(t, items[0])["managementURL"] != management {
		t.Fatalf("own inventory lost management URL: %#v", mine)
	}
	put := []string{"writer", "put", "--repo", repository, "--command-id", "human-first-publication", "--object", "note/one", "--if-absent", "--value", `{"text":"published by kaiqidong"}`}
	commit := publishedCommit(t, asMap(t, body(t, run(put...))))
	detail := asMap(t, body(t, run("catalog", "repo", "list", "--mine", "--repo", repository)))
	readiness := asMap(t, detail["readiness"])
	if readiness["publication"] != "PUBLISHED" || readiness["publishedCommit"] != commit || readiness["profile"] != "MISSING" || readiness["search"] != "NOT_AUTHORIZED" {
		t.Fatalf("management page misreports current publication or search access: %#v", detail)
	}
	receipt := asMap(t, body(t, run("writer", "receipt", "--command-id", "human-first-publication")))
	if receipt["commandId"] != "human-first-publication" || receipt["status"] != "APPLIED" {
		t.Fatalf("author cannot recover own publication: %#v", receipt)
	}
	share := asMap(t, body(t, sharing.add(repository, "consumer", "knowledge.read,knowledge.provenance,workspace.resolve,workspace.consume,file.read")))
	listed := asMap(t, body(t, sharing.list(repository)))["shares"].([]any)
	if len(listed) != 1 || asMap(t, listed[0])["id"] != share["id"] || asMap(t, listed[0])["principal"] != "consumer" {
		t.Fatalf("provider cannot recover the issued share: %#v", listed)
	}
	expectCode(t, sharing.add(repository, "consumer", "repository.shares.manage"), "FORBIDDEN")
	shareID, _ := share["id"].(string)
	if shareID == "" {
		t.Fatalf("share has no reversible identity: %#v", share)
	}
	t.Setenv("KC_CONFIG_DIR", consumerConfig)
	body(t, run("login", "--mode", "local", "--server", server.URL, "--as", "consumer"))
	// A consumer cannot re-share, enumerate grants, or revoke the owner's
	// share. The succeeding read and pin prove denial kept legitimate rights.
	expectCode(t, sharing.add(repository, "another-reader", "knowledge.read"), "FORBIDDEN")
	expectCode(t, sharing.list(repository), "FORBIDDEN")
	expectCode(t, sharing.remove(repository, shareID), "FORBIDDEN")
	read := asMap(t, body(t, run("knowledge", "read", "--repo", repository, "--commit", commit, "--object", "note/one")))
	if read["commit"] != commit {
		t.Fatal("consumer did not read the published basis")
	}
	pinFile := filepath.Join(t.TempDir(), "pin.json")
	body(t, run("workspace", "pin", "--catalog", cfg.Catalogs[0].ID, "--source", repository, "--out", pinFile))
	body(t, run("knowledge", "read", "--pin", pinFile, "--object", "note/one"))
	expectCode(t, run("writer", "receipt", "--command-id", "human-first-publication"), "FORBIDDEN")
	t.Setenv("KC_CONFIG_DIR", providerConfig)
	expectCode(t, sharing.list("kr://another-owner/unrelated"), "FORBIDDEN")
	expectCode(t, sharing.remove(repository, "unknown-share"), "USAGE_INVALID")
	remaining := asMap(t, body(t, sharing.list(repository)))["shares"].([]any)
	if len(remaining) != 1 || asMap(t, remaining[0])["id"] != shareID {
		t.Fatalf("rejected revoke changed the valid share: %#v", remaining)
	}
	body(t, sharing.remove(repository, shareID))
	guard.Lock()
	_ = current.(interface{ Close() error }).Close()
	open()
	guard.Unlock()
	replayed := asMap(t, body(t, run("catalog", "repo", "create", "--name", "团队知识")))
	if replayed["repositoryId"] != repository || replayed["managementURL"] != management || replayed["status"] != "REPLAYED" {
		t.Fatalf("restart created a different resource: %#v", replayed)
	}
	body(t, run("writer", "receipt", "--command-id", "human-first-publication"))
	t.Setenv("KC_CONFIG_DIR", consumerConfig)
	expectCode(t, run("knowledge", "read", "--pin", pinFile, "--object", "note/one"), "FORBIDDEN")
	// Optional local visual inspection keeps this exact verified service alive.
	if marker := os.Getenv("KC_PRODUCT_UI_MARKER"); marker != "" {
		raw, _ := json.Marshal(map[string]string{"url": management, "server": server.URL, "repository": repository, "username": "kaiqidong"})
		if err := os.WriteFile(marker, raw, 0600); err != nil {
			t.Fatal(err)
		}
		t.Logf("management UI ready: %s", management)
		until := time.Now().Add(5 * time.Minute)
		for time.Now().Before(until) {
			if _, err := os.Stat(marker + ".done"); err == nil {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
	}
}
