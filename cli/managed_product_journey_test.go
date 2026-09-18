package cli_test

import (
	"context"
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
	kcclient "kc/client"
	apphome "kc/home"
	"kc/internal/testkit"
	"kc/kernel"
)

// This starts with no user account or per-repository binding. Deployment
// policy is the only precondition; every user action crosses CLI -> Server.
func TestManagedProductHumanSelfServiceOnLiveGitea(t *testing.T) {
	managedHumanJourney(t, "gitea")
}

func TestManagedProductHumanSelfServiceOnDolt(t *testing.T) {
	managedHumanJourney(t, "dolt")
}

func managedHumanJourney(t *testing.T, driver string) {
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
	pool := apphome.ManagedRepositoryConfig{Driver: driver, PublicURL: server.URL, CreatorActions: []string{"writer.preview", "writer.commit", "writer.receipt.read", "knowledge.read", "knowledge.provenance", "knowledge.schema.read", "dataset.resolve", "file.read", "repository.metadata.read", "repository.shares.manage"}, ShareActions: []string{"knowledge.read", "knowledge.provenance", "dataset.resolve", "file.read"}}
	if driver == "gitea" {
		pool.DSN = base + "/kc"
	} else {
		pool.Root = filepath.Join(filepath.Dir(cfg.StateDir), "hosted-dolt")
	}
	cfg.ManagedStores = map[string]apphome.ManagedRepositoryConfig{driver: pool}
	cfg.Admission = &apphome.AdmissionConfig{RequestURL: "https://itsm.example/kc-access"}
	writeDeployment(t, path, cfg)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	grants, err := cli.ReadAllow(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	grants.Rules = append(grants.Rules, cli.AllowRule{
		ID: "external-approval", Principal: "kaiqidong", Catalog: cfg.Catalogs[0].ID,
		Actions: []string{"catalog.read", "catalog.repositories.create"},
	})
	if err := cli.WriteAllow(cfg.StateDir, grants); err != nil {
		t.Fatal(err)
	}
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
	shareActions := func(raw string) []string {
		out := []string{}
		for _, action := range strings.Split(raw, ",") {
			if action = strings.TrimSpace(action); action != "" {
				out = append(out, action)
			}
		}
		return out
	}
	repositoryShareAdd := func(client *kcclient.Client, repository, principal, actions string) kcclient.RepositoryShare {
		t.Helper()
		var share kcclient.RepositoryShare
		if err := client.CatalogService().ShareRepository(context.Background(), repository, kcclient.RepositoryShareRequest{
			Principal: principal, Actions: shareActions(actions),
		}, kcclient.RequestOptions{}, &share); err != nil {
			t.Fatal(err)
		}
		return share
	}
	repositoryShareList := func(client *kcclient.Client, repository string) kcclient.RepositoryShares {
		t.Helper()
		var listed kcclient.RepositoryShares
		if err := client.CatalogService().RepositoryShares(context.Background(), repository, kcclient.RequestOptions{}, &listed); err != nil {
			t.Fatal(err)
		}
		return listed
	}
	repositoryShareRemove := func(client *kcclient.Client, repository, shareID string) {
		t.Helper()
		if err := client.CatalogService().RevokeRepositoryShare(context.Background(), repository, shareID, kcclient.RequestOptions{}, &struct{}{}); err != nil {
			t.Fatal(err)
		}
	}
	expectRepositoryShareCode := func(client *kcclient.Client, repository, principal, actions, code string) {
		t.Helper()
		var share kcclient.RepositoryShare
		err := client.CatalogService().ShareRepository(context.Background(), repository, kcclient.RepositoryShareRequest{
			Principal: principal, Actions: shareActions(actions),
		}, kcclient.RequestOptions{}, &share)
		if string(kernel.CodeOf(err)) != code {
			t.Fatalf("want error code %s, got %v", code, err)
		}
	}
	expectRepositorySharesCode := func(client *kcclient.Client, repository, code string) {
		t.Helper()
		var listed kcclient.RepositoryShares
		err := client.CatalogService().RepositoryShares(context.Background(), repository, kcclient.RequestOptions{}, &listed)
		if string(kernel.CodeOf(err)) != code {
			t.Fatalf("want error code %s, got %v", code, err)
		}
	}
	expectRepositoryShareRevokeCode := func(client *kcclient.Client, repository, shareID, code string) {
		t.Helper()
		err := client.CatalogService().RevokeRepositoryShare(context.Background(), repository, shareID, kcclient.RequestOptions{}, &struct{}{})
		if string(kernel.CodeOf(err)) != code {
			t.Fatalf("want error code %s, got %v", code, err)
		}
	}
	providerConfig := t.TempDir()
	consumerConfig := t.TempDir()
	t.Setenv("KC_CONFIG_DIR", providerConfig)
	login := asMap(t, body(t, run("login", "--mode", "local", "--server", server.URL, "--as", "kaiqidong")))
	if login["principal"] != "kaiqidong" {
		t.Fatalf("public username changed: %#v", login)
	}
	body(t, run("admission", "show"))
	body(t, run("show"))
	created := asMap(t, body(t, run("create", "--name", "团队知识")))
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
	providerClient := productClient(t, server.URL, "kaiqidong")
	mine := myRepositories(t, providerClient)
	items, _ := mine["repositories"].([]any)
	if len(items) != 1 || asMap(t, items[0])["managementURL"] != management {
		t.Fatalf("own inventory lost management URL: %#v", mine)
	}
	put := []string{"writer", "put", "--repo", repository, "--command-id", "human-first-publication", "--object", "note/one", "--if-absent", "--value", `{"text":"published by kaiqidong"}`}
	commit := publishedCommit(t, asMap(t, body(t, run(put...))))
	detail := ownedRepository(t, providerClient, repository)
	readiness := asMap(t, detail["readiness"])
	if readiness["publication"] != "PUBLISHED" || readiness["publishedCommit"] != commit || readiness["search"] != "NOT_AUTHORIZED" {
		t.Fatalf("management page misreports current publication or search access: %#v", detail)
	}
	if _, ok := readiness["profile"]; ok {
		t.Fatalf("readiness must not keep a profile status word: %#v", readiness)
	}
	if _, ok := readiness["title"]; ok {
		t.Fatalf("readiness must not flatten README into title: %#v", readiness)
	}
	if _, ok := readiness["summary"]; ok {
		t.Fatalf("readiness must not flatten README into summary: %#v", readiness)
	}
	receipt := asMap(t, body(t, run("writer", "receipt", "--command-id", "human-first-publication")))
	if receipt["commandId"] != "human-first-publication" || receipt["status"] != "APPLIED" {
		t.Fatalf("author cannot recover own publication: %#v", receipt)
	}
	share := repositoryShareAdd(providerClient, repository, "consumer", "knowledge.read,knowledge.provenance,dataset.resolve,file.read")
	listed := repositoryShareList(providerClient, repository)
	if len(listed.Shares) != 1 || listed.Shares[0].ID != share.ID || listed.Shares[0].Principal != "consumer" {
		t.Fatalf("provider cannot recover the issued share: %#v", listed)
	}
	expectRepositoryShareCode(providerClient, repository, "consumer", "repository.shares.manage", "FORBIDDEN")
	shareID := share.ID
	if shareID == "" {
		t.Fatalf("share has no reversible identity: %#v", share)
	}
	t.Setenv("KC_CONFIG_DIR", consumerConfig)
	body(t, run("login", "--mode", "local", "--server", server.URL, "--as", "consumer"))
	consumerClient := productClient(t, server.URL, "consumer")
	// A consumer cannot re-share, enumerate grants, or revoke the owner's
	// share. The succeeding read and pin prove denial kept legitimate rights.
	expectRepositoryShareCode(consumerClient, repository, "another-reader", "knowledge.read", "FORBIDDEN")
	expectRepositorySharesCode(consumerClient, repository, "FORBIDDEN")
	expectRepositoryShareRevokeCode(consumerClient, repository, shareID, "FORBIDDEN")
	read := asMap(t, body(t, run("read", "--repo", repository, "--commit", commit, "--object", "note/one")))
	if read["commit"] != commit {
		t.Fatal("consumer did not read the published basis")
	}
	body(t, run("read", "--repo", repository, "--object", "note/one"))
	expectCode(t, run("writer", "receipt", "--command-id", "human-first-publication"), "FORBIDDEN")
	t.Setenv("KC_CONFIG_DIR", providerConfig)
	expectRepositorySharesCode(providerClient, "kr://another-owner/unrelated", "FORBIDDEN")
	expectRepositoryShareRevokeCode(providerClient, repository, "unknown-share", "USAGE_INVALID")
	remaining := repositoryShareList(providerClient, repository)
	if len(remaining.Shares) != 1 || remaining.Shares[0].ID != shareID {
		t.Fatalf("rejected revoke changed the valid share: %#v", remaining)
	}
	repositoryShareRemove(providerClient, repository, shareID)
	guard.Lock()
	_ = current.(interface{ Close() error }).Close()
	open()
	guard.Unlock()
	replayed := asMap(t, body(t, run("create", "--name", "团队知识")))
	if replayed["repositoryId"] != repository || replayed["managementURL"] != management || replayed["status"] != "REPLAYED" {
		t.Fatalf("restart created a different resource: %#v", replayed)
	}
	body(t, run("writer", "receipt", "--command-id", "human-first-publication"))
	t.Setenv("KC_CONFIG_DIR", consumerConfig)
	expectCode(t, run("read", "--repo", repository, "--object", "note/one"), "FORBIDDEN")
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
