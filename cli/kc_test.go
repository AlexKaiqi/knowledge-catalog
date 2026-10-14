package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"kc/invoke"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"kc/cli"
	kcclient "kc/client"
	"kc/internal/testkit"
	"kc/kernel"
)

func readCredentialFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func expectClientCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want error code %s, got success", code)
	}
	if string(kernel.CodeOf(err)) != code {
		t.Fatalf("want error code %s, got %v", code, err)
	}
}

func repositoryConnect(t *testing.T, serverURL, principal, catalog, repo, url, credentialFile string) map[string]any {
	t.Helper()
	client := productClient(t, serverURL, principal)
	var out map[string]any
	err := client.RepositoryManagementService().ConnectRepository(context.Background(), kcclient.ConnectionRequest{
		Repository: repo, Driver: "lakefs", URL: url, Credential: readCredentialFile(t, credentialFile),
	}, kcclient.RequestOptions{}, &out)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func repositoryConnectExpect(t *testing.T, serverURL, principal, catalog, repo, url, credentialFile, code string) {
	t.Helper()
	client := productClient(t, serverURL, principal)
	var out map[string]any
	err := client.RepositoryManagementService().ConnectRepository(context.Background(), kcclient.ConnectionRequest{
		Repository: repo, Driver: "lakefs", URL: url, Credential: readCredentialFile(t, credentialFile),
	}, kcclient.RequestOptions{}, &out)
	expectClientCode(t, err, code)
}

func repositoryConnectionShow(t *testing.T, serverURL, principal, repo string) map[string]any {
	t.Helper()
	client := productClient(t, serverURL, principal)
	var out map[string]any
	if err := client.RepositoryManagementService().RepositoryConnection(context.Background(), repo, kcclient.RequestOptions{}, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func repositoryConnectionShowExpect(t *testing.T, serverURL, principal, repo, code string) {
	t.Helper()
	client := productClient(t, serverURL, principal)
	var out map[string]any
	expectClientCode(t, client.RepositoryManagementService().RepositoryConnection(context.Background(), repo, kcclient.RequestOptions{}, &out), code)
}

func repositoryConnectionCheck(t *testing.T, serverURL, principal, repo string) map[string]any {
	t.Helper()
	client := productClient(t, serverURL, principal)
	var out map[string]any
	if err := client.RepositoryManagementService().CheckRepositoryConnection(context.Background(), repo, kcclient.RequestOptions{}, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func repositoryConnectionCheckExpect(t *testing.T, serverURL, principal, repo, code string) {
	t.Helper()
	client := productClient(t, serverURL, principal)
	var out map[string]any
	expectClientCode(t, client.RepositoryManagementService().CheckRepositoryConnection(context.Background(), repo, kcclient.RequestOptions{}, &out), code)
}

func repositoryConnectionRotate(t *testing.T, serverURL, principal, repo, credentialFile string) map[string]any {
	t.Helper()
	client := productClient(t, serverURL, principal)
	var out map[string]any
	err := client.RepositoryManagementService().RotateRepositoryConnection(context.Background(), repo, kcclient.ConnectionRotationRequest{
		Credential: readCredentialFile(t, credentialFile),
	}, kcclient.RequestOptions{}, &out)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func repositoryConnectionRotateExpect(t *testing.T, serverURL, principal, repo, credentialFile, code string) {
	t.Helper()
	client := productClient(t, serverURL, principal)
	var out map[string]any
	err := client.RepositoryManagementService().RotateRepositoryConnection(context.Background(), repo, kcclient.ConnectionRotationRequest{
		Credential: readCredentialFile(t, credentialFile),
	}, kcclient.RequestOptions{}, &out)
	expectClientCode(t, err, code)
}

func kc(home string, args ...string) kcRunResult {
	args = groupedTestArgs(args)
	all := append([]string{"--home", home}, args...)
	return kcRunResultFrom(cli.RunEmbeddedForTest(all, nil), publicCommandPath(all))
}

func TestHelperRejectsRetiredCatalogShow(t *testing.T) {
	home := testkit.TempDir(t)
	body(t, kc(home, "local", "init"))
	expectCode(t, kc(home, "catalog-show"), "USAGE_INVALID")
}

// seedRepo prepares the fixture's selected index capability on an object
// source, then attaches and registers its Snapshot. With no index backend the
// source stays unconfigured. Credentials remain in the process environment.
func seedRepo(t *testing.T, home, repo string, extra ...string) {
	t.Helper()
	seedSourceRepository(t, home, repo, true, extra...)
}

func seedUnindexedRepo(t *testing.T, home, repo string, extra ...string) {
	t.Helper()
	seedSourceRepository(t, home, repo, false, extra...)
}

func seedSourceRepository(t *testing.T, home, repo string, indexed bool, extra ...string) {
	t.Helper()
	if len(extra) == 0 {
		fake := testkit.NewLakeFSFake(t)
		t.Setenv("KC_LAKEFS_CREDENTIAL", testkit.LakeFSFakeCredential)
		extra = []string{"--driver", "lakefs", "--dsn", fake.DSN(fake.NewRepo())}
	}
	args := append([]string{"local", "repository", "attach", "--repo", repo}, extra...)
	if indexed {
		parsed, err := cli.ParseArgs(args)
		if err != nil {
			t.Fatal(err)
		}
		dsn := invoke.FlagString(parsed.Flags, "dsn")
		if dsn == "" {
			dsn = invoke.FlagString(parsed.Flags, "link")
		}
		if err := cli.ConfigureFixtureIndexSource(home, kernel.RepositoryID(repo), dsn); err != nil {
			t.Fatal(err)
		}
	}
	body(t, kc(home, args...))
	body(t, kc(home, "attach", "--repo", repo))
}

// lakeFSRepoDSN provisions one protocol-faithful lakeFS fake repository for
// this test and returns its DSN; the credential stays in the environment.
func lakeFSRepoDSN(t *testing.T) string {
	t.Helper()
	fake := testkit.NewLakeFSFake(t)
	t.Setenv("KC_LAKEFS_CREDENTIAL", testkit.LakeFSFakeCredential)
	return fake.DSN(fake.NewRepo())
}

func isolateClientCredentials(t *testing.T) {
	t.Helper()
	// Login tests persist Taihu/local sessions under KC_CONFIG_DIR. Product
	// remote CLI with --as must not pick up a leftover Authorization pairing.
	t.Setenv("KC_AUTH_TOKEN", "")
	t.Setenv("KC_AS", "")
	ensureRemoteClientConfigDir(t)
}

func ensureRemoteClientConfigDir(t *testing.T) {
	t.Helper()
	if os.Getenv("KC_TEST_CLIENT_CONFIG") == "1" {
		return
	}
	dir := strings.TrimSpace(os.Getenv("KC_CONFIG_DIR"))
	if dir == "" || strings.Contains(dir, "kc-cli-config-") {
		t.Setenv("KC_CONFIG_DIR", t.TempDir())
	}
	t.Setenv("KC_TEST_CLIENT_CONFIG", "1")
}

func productClient(t *testing.T, serverURL, principal string) *kcclient.Client {
	t.Helper()
	client, err := kcclient.New(kcclient.Config{BaseURL: serverURL, HTTPClient: http.DefaultClient})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Login(context.Background(), kcclient.LoginRequest{Identity: kcclient.Identity{Principal: principal}}); err != nil {
		t.Fatal(err)
	}
	return client
}

func myRepositories(t *testing.T, client *kcclient.Client) map[string]any {
	t.Helper()
	var out map[string]any
	if err := client.RepositoryManagementService().MyRepositories(context.Background(), kcclient.RequestOptions{}, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func ownedRepository(t *testing.T, client *kcclient.Client, repository string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := client.RepositoryManagementService().ManagedRepository(context.Background(), repository, kcclient.RequestOptions{}, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func remoteCatalogShow(t *testing.T, serverURL, principal, catalogID string) map[string]any {
	t.Helper()
	return asMap(t, body(t, kcRemote(t, serverURL, principal, "show")))
}

func kcRemote(t *testing.T, serverURL, principal string, args ...string) kcRunResult {
	t.Helper()
	isolateClientCredentials(t)
	args = groupedTestArgs(args)
	all := append([]string{"--server", serverURL, "--as", principal}, args...)
	return kcRunResultFrom(cli.Run(all), publicCommandPath(all))
}

func publicCommandPath(all []string) string {
	parsed, err := cli.ParseArgs(all)
	if err != nil {
		return ""
	}
	for n := len(parsed.Args); n >= 0; n-- {
		path := strings.Join(append([]string{parsed.Command}, parsed.Args[:n]...), " ")
		if cli.CLICommandForTest(path) {
			return path
		}
	}
	return ""
}

func kcRunResultFrom(run cli.RunResult, operation string) kcRunResult {
	result := kcRunResult{RunResult: run, operation: operation, runID: runSequence.Add(1)}
	recordCommandRun(operation, result.Status == 0)
	return result
}

// kcClientLocal runs a public command that keeps state in the client credential
// store rather than a Home. `kc login`/`logout` reject --home together with a
// Server target, so they cannot go through kc(), which always injects --home.
// Coverage is recorded identically so the per-command gate still applies.
func kcClientLocal(args ...string) kcRunResult {
	args = groupedTestArgs(args)
	operation := ""
	if parsed, err := cli.ParseArgs(args); err == nil {
		for n := len(parsed.Args); n >= 0; n-- {
			path := strings.Join(append([]string{parsed.Command}, parsed.Args[:n]...), " ")
			if cli.CLICommandForTest(path) {
				operation = path
				break
			}
		}
	}
	result := kcRunResult{RunResult: cli.RunEmbeddedForTest(args, nil), operation: operation, runID: runSequence.Add(1)}
	recordCommandRun(operation, result.Status == 0)
	return result
}

// Retrieval is an explicitly maintained projection. Consumer operations never
// build it as a side effect, so journeys that expect SEARCH/RELATIONS first
// publish the required exact-basis projection.
func syncIndexes(t *testing.T, home string, repositories ...string) {
	t.Helper()
	for _, repository := range repositories {
		body(t, kc(home, "index-sync", "--repo", repository))
	}
}

// Existing package tests name the internal operation they exercise. This
// adapter is test-only: the process entry still rejects every flat command.
func groupedTestArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	args = append([]string{}, args...)
	if args[0] == "allow" || args[0] == "allowed" {
		for i := 1; i+1 < len(args); i++ {
			if args[i] == "--cmd" {
				args[i] = "--action"
				args[i+1] = legacyTestActions(args[i+1])
			}
		}
	}
	if args[0] == "hook-add" {
		for i := 1; i+1 < len(args); i++ {
			if args[i] == "--on" {
				args[i+1] = legacyTestActions(args[i+1])
			}
		}
	}
	paths := map[string][]string{
		"init": {"local", "init"}, "status": {"local", "status"},
		"catalog-add": {"local", "catalog", "attach"}, "repo-add": {"local", "repository", "attach"},
		"store-ls": {"local", "store", "show"}, "store-set": {"local", "store", "set"}, "overlay": {"local", "dataset", "overlay"},
		"whoami": {"whoami"}, "allow": {"grant", "add"}, "revoke": {"grant", "remove"}, "allowed": {"grant", "list"},
		"audit": {"registry", "audit"}, "register": {"attach"}, "archive-repo": {"detach"},
		"archive-catalog": {"catalog", "archive"},
		"describe-schema": {"schema", "describe"}, "resolve-binding": {"binding", "show"},
		"resolve-object": {"resolve"},
		"put":            {"writer", "put"}, "remove": {"writer", "remove"}, "commit": {"writer", "commit"}, "writer-head": {"writer", "head"}, "receipt": {"writer", "receipt"},
		"propose": {"governance", "proposal", "create"}, "merge": {"governance", "proposal", "merge"}, "preview": {"governance", "preview", "create"},
		"validate": {"governance", "preview", "validate"}, "record-validation": {"governance", "validation", "record"},
		"describe-index": {"operations", "projection", "describe"}, "index-sync": {"operations", "projection", "sync"}, "index-notify": {"operations", "projection", "notice"}, "describe-access": {"operations", "access-spec", "describe"},
		"hook-add": {"operations", "hook", "add"}, "hook-ls": {"operations", "hook", "list"}, "hook-rm": {"operations", "hook", "remove"},
		"gate-add": {"operations", "gate", "add"}, "gate-ls": {"operations", "gate", "list"}, "gate-rm": {"operations", "gate", "remove"},
		"access-log": {"operations", "audit", "access"}, "trace": {"operations", "audit", "trace"}, "hitmap": {"operations", "audit", "hitmap"}, "record-feedback": {"operations", "feedback", "record"},
	}
	if path := paths[args[0]]; len(path) > 0 {
		return append(append([]string{}, path...), args[1:]...)
	}
	return args
}

func legacyTestActions(raw string) string {
	legacy := map[string]string{
		"put": "writer.commit", "remove": "writer.commit", "commit": "writer.commit",
		"read": "object.read", "read-workspace": "file.read", "read-catalog": "inventory.read", "search": "object.search",
		"relations": "object.relations", "resolve": "dataset.resolve",
		"describe-access": "object.access.describe",
		"propose":         "governance.proposal.create", "preview": "governance.preview.create",
		"validate": "governance.validate", "record-validation": "governance.validation.record", "merge": "governance.merge",
		"register": "repositories.manage", "archive-repo": "repositories.manage", "archive-catalog": "catalog.manage",
	}
	parts := strings.Split(raw, ",")
	for i, part := range parts {
		if action := legacy[strings.TrimSpace(part)]; action != "" {
			parts[i] = action
		}
	}
	return strings.Join(parts, ",")
}

func evidenceBytes(t *testing.T, home, kind string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if b, err := os.ReadFile(filepath.Join(home, kind+".jsonl")); err == nil {
		buf.Write(b)
	}
	entries, err := os.ReadDir(filepath.Join(home, kind))
	if err != nil {
		return buf.Bytes()
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(home, kind, name))
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(b)
	}
	return buf.Bytes()
}

func poisonEvidenceStream(t *testing.T, home, kind string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, kind), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func body(t *testing.T, result kcRunResult) any {
	t.Helper()
	if result.Status != 0 {
		t.Fatalf("status %d stdout %s", result.Status, result.Stdout)
	}
	var value any
	if err := json.Unmarshal([]byte(result.Stdout), &value); err != nil {
		t.Fatal(err, result.Stdout)
	}
	recordAssertedSuccess(t, result)
	return value
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("not object: %#v", value)
	}
	return m
}

func cloneJSONMap(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func failError(t *testing.T, result kcRunResult) map[string]any {
	t.Helper()
	if result.Status != 1 {
		t.Fatalf("want status 1, got %d stdout %s", result.Status, result.Stdout)
	}
	if errObj, ok := parseCLIFault(result.Stdout); ok {
		return errObj
	}
	t.Fatalf("want a CLI fault, got %s", result.Stdout)
	return nil
}

func parseCLIFault(stdout string) (map[string]any, bool) {
	stdout = strings.TrimSpace(stdout)
	if strings.HasPrefix(stdout, "{") {
		var payload map[string]any
		if err := json.NewDecoder(strings.NewReader(stdout)).Decode(&payload); err != nil {
			return nil, false
		}
		errObj, _ := payload["error"].(map[string]any)
		if errObj == nil {
			return nil, false
		}
		return errObj, true
	}
	line, _, _ := strings.Cut(stdout, "\n")
	code, message, ok := strings.Cut(line, ": ")
	if !ok || code == "" || message == "" {
		return nil, false
	}
	return map[string]any{"code": code, "message": message}, true
}

func expectCode(t *testing.T, result kcRunResult, code string) {
	t.Helper()
	err := failError(t, result)
	if err["code"] != code {
		t.Fatalf("want error code %s, got %#v", code, err)
	}
	recordAssertedFailure(t, result, code)
}

func expectMsg(t *testing.T, result kcRunResult, substr string) {
	t.Helper()
	err := failError(t, result)
	msg, _ := err["message"].(string)
	if !strings.Contains(msg, substr) {
		t.Fatalf("want message containing %q, got %#v", substr, err)
	}
	code, _ := err["code"].(string)
	recordAssertedFailure(t, result, code)
}

func TestParseSkipsBareDashDash(t *testing.T) {
	parsed, err := cli.ParseArgs([]string{"--", "serve", "--config", "/tmp/kc-demo/deployment.json", "--listen", "127.0.0.1:7380"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Command != "serve" {
		t.Fatal(parsed)
	}
	if invoke.FlagString(parsed.Flags, "config") != "/tmp/kc-demo/deployment.json" {
		t.Fatal(parsed.Flags)
	}
}

func TestShortHelpIsNotAPositional(t *testing.T) {
	parsed, err := cli.ParseArgs([]string{"search", "-h"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Command != "search" || !invoke.FlagBool(parsed.Flags, "help") || len(parsed.Args) != 0 {
		t.Fatalf("-h must be help, not a search operand: %#v", parsed)
	}

	parsed, err = cli.ParseArgs([]string{"search", "--query", "x", "-h"})
	if err != nil {
		t.Fatal(err)
	}
	if !invoke.FlagBool(parsed.Flags, "help") || invoke.FlagString(parsed.Flags, "query") != "x" {
		t.Fatalf("-h after flags must still be help: %#v", parsed)
	}

	result := cli.Run([]string{"search", "-h"})
	if result.Status != 0 || !strings.Contains(result.Stdout, "kc search (") {
		t.Fatalf("kc search -h must print search usage: %#v", result)
	}
	if strings.Contains(result.Stdout, "DATASET_INVALID") {
		t.Fatalf("kc search -h must not treat -h as a dataset id: %#v", result)
	}
}

func TestUsageInvalidShowsReasonAndLeafUsage(t *testing.T) {
	home := testkit.TempDir(t)
	cases := []struct {
		args  []string
		usage string
	}{
		{[]string{"grant", "add"}, "kc grant add --principal"},
		{[]string{"grant", "remove"}, "kc grant remove --id"},
		{[]string{"detach"}, "kc detach --repo"},
		{[]string{"search"}, "kc search ("},
		{[]string{"read"}, "kc read <object-urn"},
		{[]string{"schema", "list"}, "kc schema list --repo"},
		{[]string{"writer", "put"}, "kc writer put <object-urn"},
		{[]string{"writer", "head"}, "kc writer head --repo"},
		{[]string{"diff"}, "kc diff --repo"},
		{[]string{"operations", "hook", "remove"}, "kc operations hook remove --id"},
		{[]string{"deployment", "init"}, "kc deployment init --config"},
	}
	for _, tc := range cases {
		result := kc(home, tc.args...)
		expectCode(t, result, "USAGE_INVALID")
		if strings.Contains(result.Stdout, `"error"`) {
			t.Fatalf("%s USAGE_INVALID must not be JSON-only: %s", strings.Join(tc.args, " "), result.Stdout)
		}
		if !strings.Contains(result.Stdout, tc.usage) {
			t.Fatalf("%s missing usage %q: %s", strings.Join(tc.args, " "), tc.usage, result.Stdout)
		}
	}
}

func TestRoleHelp(t *testing.T) {
	for topic, needles := range map[string][]string{
		"consume": {"kc login", "kc show", "schema list --repo", "search --repo", "search --dataset", "read --dataset", "回执"},
		"write":   {"kc create --name", "kc create --url", "writer commit", "kc attach", "kc grant add"},
		"compose": {"kc show", "kc attach", "dataset define", "grant add"},
	} {
		result := cli.Run([]string{"help", topic})
		if result.Status != 0 {
			t.Fatalf("help %s: %#v", topic, result)
		}
		for _, needle := range needles {
			if !strings.Contains(result.Stdout, needle) {
				t.Fatalf("help %s missing %q: %s", topic, needle, result.Stdout)
			}
		}
		for _, leak := range []string{"catalog show", "admin grant", "admission request", "search --catalog"} {
			if strings.Contains(result.Stdout, leak) {
				t.Fatalf("help %s leaked %q: %s", topic, leak, result.Stdout)
			}
		}
	}
	unknown := cli.Run([]string{"help", "governor"})
	if unknown.Status == 0 || !strings.Contains(unknown.Stdout, "unknown help topic") {
		t.Fatalf("unknown role help must fail clearly: %#v", unknown)
	}
	extra := cli.Run([]string{"help", "consume", "extra"})
	if extra.Status == 0 || !strings.Contains(extra.Stdout, "consume extra") {
		t.Fatalf("role help must reject extra positionals: %#v", extra)
	}
}

func TestProtocolErrorJSON(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	seedRepo(t, h, "kr://acme/public/core")
	expectCode(t, kc(h, "read", testObjectReference(t, "kr://acme/public/core", "missing", ""), "--ref", "refs/heads/main"), "OBJECT_REF_UNRESOLVED")
}

func TestProposeMergeIsVisibleOnView(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	seedRepo(t, h, "kr://acme/public/core")
	body(t, kc(h, "put", testObjectReference(t, "kr://acme/public/core", "policy/P-103", ""), "--command-id", "seed", "--value", `{"v":1}`))
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "1", "--source", "kr://acme/public/core=refs/heads/main"))
	proposal := asMap(t, body(t, kc(h,
		"propose", "--proposal-id", "PR-1", "--repo", "kr://acme/public/core",
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/PR-1",
		"--object", "policy/P-103", "--value", `{"v":2}`,
	)))
	preview := asMap(t, body(t, kc(h, "preview", "--proposal", "PR-1", "--dataset", "agent")))
	structural := asMap(t, body(t, kc(h, "validate", "--preview", preview["previewId"].(string))))
	if structural["outcome"] != "PASSED" {
		t.Fatal(structural)
	}
	validation := asMap(t, body(t, kc(h, "record-validation", "--preview", preview["previewId"].(string), "--suite", "S7", "--outcome", "PASSED")))
	merged := asMap(t, body(t, kc(h, "merge", "--proposal", proposal["proposalId"].(string), "--preview", preview["previewId"].(string), "--validation", validation["reportId"].(string))))
	if merged["commitId"] != proposal["candidateCommit"] {
		t.Fatal(merged, proposal)
	}
	body(t, kc(h, "dataset", "define", "--dataset", "agent", "--revision", "2", "--source", "kr://acme/public/core=refs/heads/main"))
	serving := asMap(t, body(t, kc(h, "read", testObjectReference(t, "kr://acme/public/core", "policy/P-103", ""), "--dataset", "agent")))
	if asMap(t, serving)["value"].(map[string]any)["v"] != float64(2) {
		t.Fatal(serving)
	}
}

func TestSingleRegistrationSpacePublishesIndependentDatasets(t *testing.T) {
	h := testkit.TempDir(t)
	body(t, kc(h, "init"))
	const repo = "kr://acme/public/core"
	seedRepo(t, h, repo)
	body(t, kc(h, "put", testObjectReference(t, repo, "policy/P-1", ""), "--command-id", "seed", "--value", `{ "v": 1 }`))
	for _, name := range []string{"ops", "docs"} {
		body(t, kc(h, "dataset", "define", "--dataset", name, "--revision", "1", "--source", repo))
	}
	state := asMap(t, body(t, kc(h, "show")))
	if len(state["datasets"].([]any)) != 2 {
		t.Fatalf("one registry lost a Dataset: %#v", state)
	}
	if _, found := state["catalogId"]; found {
		t.Fatalf("inventory exposes retired Catalog identity: %#v", state)
	}
	expectCode(t, kc(h, "catalog", "list"), "USAGE_INVALID")
	expectCode(t, kc(h, "catalog", "use", "old"), "USAGE_INVALID")
	for _, name := range []string{"ops", "docs"} {
		got := asMap(t, body(t, kc(h, "read", testObjectReference(t, repo, "policy/P-1", ""), "--dataset", name)))
		if got["reference"] != testObjectReference(t, repo, "policy/P-1", "") {
			t.Fatalf("Dataset %s lost its accepted source: %#v", name, got)
		}
	}
	history := asMap(t, body(t, kc(h, "registry", "audit")))
	if len(history["entries"].([]any)) == 0 {
		t.Fatal(history)
	}
}

func TestWorkspaceAndCatalogLifecycle(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	seedRepo(t, h, "kr://acme/public/core")
	body(t, kc(h, "put", testObjectReference(t, "kr://acme/public/core", "policy/P-1", ""), "--command-id", "seed", "--value", `{"v":1}`))
	body(t, kc(h, "dataset", "define", "--dataset", "ops", "--revision", "1", "--source", "kr://acme/public/core=refs/heads/main"))
	got := asMap(t, body(t, kc(h, "read", testObjectReference(t, "kr://acme/public/core", "policy/P-1", ""), "--dataset", "ops")))
	if got["reference"] != testObjectReference(t, "kr://acme/public/core", "policy/P-1", "") {
		t.Fatal(got)
	}
	body(t, kc(h, "dataset", "retire", "--dataset", "ops"))
	expectCode(t, kc(h, "read", testObjectReference(t, "kr://acme/public/core", "policy/P-1", ""), "--dataset", "ops"), "DATASET_INVALID")
	expectMsg(t, kc(h, "pin-workspace", "--dataset", "ops"), "unknown command pin-workspace")
	expectCode(t, kc(h, "pin-workspace", "--dataset", "ops"), "USAGE_INVALID")
	expectCode(t, kc(h, "receipt", "--command-id", "missing"), "USAGE_INVALID")
	expectCode(t, kc(h, "archive-catalog"), "USAGE_INVALID")
	body(t, kc(h, "dataset", "define", "--dataset", "later", "--revision", "1", "--source", "kr://acme/public/core=refs/heads/main"))
}

func TestCatalogIsolationDoesNotShareAllow(t *testing.T) {
	h := testkit.TempDir(t)
	pub := "kr://acme/public/physical"
	secret := "kr://acme/restricted/classif"
	kc(h, "init")
	seedRepo(t, h, pub)
	seedRepo(t, h, secret)
	body(t, kc(h, "attach", "--repo", secret))
	body(t, kc(h, "put", testObjectReference(t, pub, "Table:orders", ""), "--command-id", "pub-1", "--value", `{"src":"public"}`))
	body(t, kc(h, "put", testObjectReference(t, secret, "Table:orders", ""), "--command-id", "sec-1", "--value", `{"src":"secret"}`))
	body(t, kc(h, "dataset", "define", "--dataset", "company", "--revision", "1", "--source", pub+"=refs/heads/main"))
	body(t, kc(h, "dataset", "define", "--dataset", "classif", "--revision", "1", "--source", secret+"=refs/heads/main"))

	body(t, kc(h, "allow", "--principal", "crew-bot", "--cmd", "read", "--repo", pub))
	body(t, kc(h, "allow", "--principal", "crew-bot", "--cmd", "read-workspace", "--dataset", "company"))
	body(t, kc(h, "allow", "--principal", "crew-bot", "--action", "file.read", "--dataset", "company"))
	body(t, kc(h, "allow", "--principal", "classif-bot", "--cmd", "read", "--repo", secret))
	body(t, kc(h, "allow", "--principal", "classif-bot", "--cmd", "read-workspace", "--dataset", "classif"))
	body(t, kc(h, "allow", "--principal", "classif-bot", "--action", "file.read", "--dataset", "classif"))
	body(t, kc(h, "allow", "--principal", "classif-bot", "--action", "inventory.read"))

	crew := asMap(t, body(t, kc(h, "read", testObjectReference(t, pub, "Table:orders", ""), "--as", "crew-bot")))
	if crew["repository"] != pub {
		t.Fatalf("%#v", crew)
	}
	expectCode(t, kc(h, "read", testObjectReference(t, secret, "Table:orders", ""), "--as", "crew-bot"), "FORBIDDEN")
	expectCode(t, kc(h, "read", testObjectReference(t, pub, "Table:orders", ""), "--as", "classif-bot"), "FORBIDDEN")
	classif := asMap(t, body(t, kc(h, "read", testObjectReference(t, secret, "Table:orders", ""), "--as", "classif-bot")))
	if classif["repository"] != secret {
		t.Fatalf("%#v", classif)
	}
}

func TestForkPublishDoesNotCopyPersonal(t *testing.T) {
	h := testkit.TempDir(t)
	pub := "kr://acme/public/semantic"
	alice := "kr://acme/personals/alice"
	kc(h, "init")
	seedRepo(t, h, pub)
	seedRepo(t, h, alice)
	draft := asMap(t, asMap(t, body(t, kc(h, "put", testObjectReference(t, alice, "drafts/metric-x", ""), "--command-id", "alice-draft", "--value", `{"text":"alice draft"}`)))["result"])
	source := alice + "@" + draft["newCommit"].(string) + "/drafts/metric-x"
	body(t, kc(h, "dataset", "define", "--dataset", "semantic", "--revision", "1", "--source", pub+"=refs/heads/main"))
	proposal := asMap(t, body(t, kc(h,
		"propose", "--proposal-id", "FORK-1", "--repo", pub,
		"--target", "refs/heads/main", "--candidate", "refs/heads/candidates/FORK-1",
		"--object", "metrics/x", "--value", `{"text":"published"}`,
		"--origin-kind", "ASSERTION",
		"--source-ref", "kc://"+strings.TrimPrefix(source, "kr://"),
	)))
	preview := asMap(t, body(t, kc(h, "preview", "--proposal", "FORK-1", "--dataset", "semantic")))
	structural := asMap(t, body(t, kc(h, "validate", "--preview", preview["previewId"].(string))))
	if structural["outcome"] != "PASSED" {
		t.Fatal(structural)
	}
	validation := asMap(t, body(t, kc(h, "record-validation", "--preview", preview["previewId"].(string), "--suite", "fork", "--outcome", "PASSED")))
	body(t, kc(h, "merge", "--proposal", proposal["proposalId"].(string), "--preview", preview["previewId"].(string), "--validation", validation["reportId"].(string)))
	body(t, kc(h, "dataset", "define", "--dataset", "semantic", "--revision", "2", "--source", pub+"=refs/heads/main"))

	live := asMap(t, body(t, kc(h, "read", testObjectReference(t, pub, "metrics/x", ""), "--ref", "refs/heads/main")))
	if asMap(t, live["value"])["text"] != "published" {
		t.Fatal(live)
	}
	personal := asMap(t, body(t, kc(h, "read", testObjectReference(t, alice, "drafts/metric-x", ""), "--ref", "refs/heads/main")))
	if asMap(t, personal["value"])["text"] != "alice draft" {
		t.Fatal(personal)
	}
	expectCode(t, kc(h, "read", testObjectReference(t, pub, "drafts/metric-x", ""), "--ref", "refs/heads/main"), "OBJECT_REF_UNRESOLVED")
	prov := asMap(t, body(t, kc(h, "provenance", "--repo", pub, "--object", "metrics/x", "--ref", "refs/heads/main")))
	chain := prov["chain"].([]any)
	refs := asMap(t, chain[0])["sourceRefs"].([]any)
	if len(refs) != 1 || refs[0] != "kc://acme/personals/alice@"+draft["newCommit"].(string)+"/drafts/metric-x" {
		t.Fatal(prov)
	}
	servingNew := asMap(t, body(t, kc(h, "read", testObjectReference(t, pub, "metrics/x", ""), "--dataset", "semantic")))
	if servingNew["reference"] != testObjectReference(t, pub, "metrics/x", "") {
		t.Fatal("merged fork must be visible on next read --dataset", servingNew)
	}
	servingDraft := kc(h, "read", testObjectReference(t, alice, "drafts/metric-x", ""), "--dataset", "semantic")
	expectCode(t, servingDraft, "OBJECT_REF_UNRESOLVED")
	if strings.Contains(servingDraft.Stdout, "alice draft") {
		t.Fatal("personal draft leaked into public workspace", servingDraft.Stdout)
	}
}

func TestSchemaRefOnPropose(t *testing.T) {
	h := testkit.TempDir(t)
	core := "kr://acme/public/core"
	kc(h, "init")
	seedRepo(t, h, core)
	expectCode(t, kc(h, "propose",
		"--proposal-id", "PR-schema",
		"--repo", core,
		"--target", "refs/heads/main",
		"--candidate", "refs/heads/candidates/PR-schema",
		"--object", "policy/A",
		"--value", `{"v":1}`,
		"--schema-ref", "schema/policy",
	), "SCHEMA_REVISION_UNRESOLVED")
	expectMsg(t, kc(h, "propose",
		"--proposal-id", "PR-missing-candidate",
		"--repo", core,
		"--target", "refs/heads/main",
		"--object", "policy/A",
		"--value", `{"v":1}`,
	), "missing --candidate")
	body(t, kc(h, "put", testObjectReference(t, core, "schema/policy", ""), "--command-id", "schema-policy", "--value", `{"entity":"Policy","pattern":"record"}`))
	proposal := asMap(t, body(t, kc(h, "propose",
		"--proposal-id", "PR-schema-ok",
		"--repo", core,
		"--target", "refs/heads/main",
		"--candidate", "refs/heads/candidates/PR-schema-ok",
		"--object", "policy/A",
		"--value", `{"v":1}`,
		"--schema-ref", "schema/policy",
	)))
	if proposal["candidateCommit"] == "" {
		t.Fatal(proposal)
	}
}

func TestWritePath(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	seedRepo(t, h, "kr://acme/public/core")
	expectCode(t, kc(h, "put", testObjectReference(t, "kr://acme/public/core", "policy/A", ""), "--command-id", "missing-schema", "--value", `{"v":1}`, "--schema-ref", "schema/policy@c1"), "SCHEMA_REVISION_UNRESOLVED")
	body(t, kc(h, "put", testObjectReference(t, "kr://acme/public/core", "schema/policy", ""), "--command-id", "schema-policy", "--value", `{"entity":"Policy","pattern":"record"}`))
	created := asMap(t, body(t, kc(h, "put", testObjectReference(t, "kr://acme/public/core", "policy/A", ""), "--command-id", "create-a", "--value", `{"v":1}`, "--if-absent", "--schema-ref", "schema/policy", "--origin-kind", "SOURCE", "--actor-ref", "alice")))
	if created["disposition"] != "APPLIED" {
		t.Fatal(created)
	}
	dup := kc(h, "put", testObjectReference(t, "kr://acme/public/core", "policy/A", ""), "--command-id", "create-a-again", "--value", `{"v":2}`, "--if-absent")
	expectCode(t, dup, "PRECONDITION_FAILED")
	draft := filepath.Join(h, "draft")
	if err := os.MkdirAll(draft, 0o755); err != nil {
		t.Fatal(err)
	}
	canonical := "---\nobject_id: runbooks/oncall\nschema_ref: schema/runbook.body\n---\n{\"text\":\"check freeze\"}\n"
	if err := os.WriteFile(filepath.Join(draft, "note.json"), []byte(canonical), 0o644); err != nil {
		t.Fatal(err)
	}
	schema := "---\nobject_id: schema/runbook.body\n---\n{\"entity\":\"Runbook\",\"pattern\":\"record\",\"fields\":{\"text\":{\"type\":\"string\",\"access\":[\"text\"]}}}\n"
	if err := os.WriteFile(filepath.Join(draft, "schema.json"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	committed := asMap(t, body(t, kc(h, "commit", "--command-id", "ingest-1", "--repo", "kr://acme/public/core", "--dir", draft)))
	if committed["disposition"] != "APPLIED" {
		t.Fatal(committed)
	}
	receipt := asMap(t, body(t, kc(h, "receipt", "--command-id", "ingest-1")))
	if receipt["commandId"] != "ingest-1" || receipt["digest"] == "" {
		t.Fatal(receipt)
	}
	missing := kc(h, "put", testObjectReference(t, "kr://acme/public/core", "derived/x", ""), "--command-id", "derived-bad", "--value", `{"v":1}`, "--origin-kind", "DERIVATION")
	expectCode(t, missing, "PRECONDITION_FAILED")
	body(t, kc(h, "put", testObjectReference(t, "kr://acme/public/core", "derived/x", ""), "--command-id", "derived-ok", "--value", `{"v":1}`, "--origin-kind", "DERIVATION", "--input-dataset-version", "vr-1", "--algorithm-hash", "abc"))
}

func TestDiffDirectoryAgainstCurrentVersion(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	repo := "kr://acme/public/core"
	seedRepo(t, h, repo)
	draft := filepath.Join(h, "draft")
	if err := os.MkdirAll(draft, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nobject_id: runbooks/oncall\n---\n{\"text\":\"check freeze\"}\n"
	if err := os.WriteFile(filepath.Join(draft, "note.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	expectCode(t, kc(h, "diff", "--as", "untrusted-agent", "--repo", repo, "--dir", draft), "FORBIDDEN")

	head := asMap(t, body(t, kc(h, "writer", "head", "--repo", repo)))
	added := asMap(t, body(t, kc(h, "diff", "--repo", repo, "--dir", draft)))
	if added["repository"] != repo || added["commit"] != head["commit"] {
		t.Fatalf("diff identity: %#v head %#v", added, head)
	}
	if _, ok := added["changeSet"]; ok {
		t.Fatalf("diff printed ChangeSet: %#v", added)
	}
	changes := added["changes"].([]any)
	if len(changes) != 1 {
		t.Fatalf("want one add, got %#v", added)
	}
	row := asMap(t, changes[0])
	if row["objectId"] != "runbooks/oncall" || row["change"] != "add" {
		t.Fatalf("add row: %#v", row)
	}

	body(t, kc(h, "commit", "--command-id", "diff-1", "--repo", repo, "--dir", draft))
	unchanged := asMap(t, body(t, kc(h, "diff", "--repo", repo, "--dir", draft)))
	if unchanged["changes"] == nil || len(unchanged["changes"].([]any)) != 0 {
		t.Fatalf("want empty diff after commit, got %#v", unchanged)
	}

	updatedContent := "---\nobject_id: runbooks/oncall\n---\n{\"text\":\"check freeze window\"}\n"
	if err := os.WriteFile(filepath.Join(draft, "note.json"), []byte(updatedContent), 0o644); err != nil {
		t.Fatal(err)
	}
	updated := asMap(t, body(t, kc(h, "diff", "--repo", repo, "--dir", draft)))
	updatedChanges := updated["changes"].([]any)
	if len(updatedChanges) != 1 {
		t.Fatalf("want one update, got %#v", updated)
	}
	updatedRow := asMap(t, updatedChanges[0])
	if updatedRow["objectId"] != "runbooks/oncall" || updatedRow["change"] != "update" {
		t.Fatalf("update row: %#v", updatedRow)
	}

	body(t, kc(h, "allow", "--principal", "preview-agent", "--action", "writer.preview", "--repo", repo))
	previewed := asMap(t, body(t, kc(h, "diff", "--as", "preview-agent", "--repo", repo, "--dir", draft)))
	if len(previewed["changes"].([]any)) != 1 {
		t.Fatalf("writer.preview must be enough to diff: %#v", previewed)
	}
}

func TestDesiredDirCommitRequiresWriterCommit(t *testing.T) {
	h := testkit.TempDir(t)
	kc(h, "init")
	repo := "kr://acme/public/core"
	seedRepo(t, h, repo)
	body(t, kc(h, "put", testObjectReference(t, repo, "schema/secret", ""), "--command-id", "schema-secret", "--value", `{"entity":"Secret","pattern":"record","fields":{"text":{"type":"string","access":["text"]}}}`))

	draft := filepath.Join(h, "untrusted-draft")
	if err := os.MkdirAll(draft, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nobject_id: notes/untrusted\nschema_ref: schema/secret\n---\n{\"text\":\"draft\"}\n"
	if err := os.WriteFile(filepath.Join(draft, "note.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	body(t, kc(h, "allow", "--principal", "untrusted-agent", "--action", "writer.preview", "--repo", repo))
	expectCode(t, kc(h, "commit", "--as", "untrusted-agent", "--command-id", "untrusted-dir", "--repo", repo, "--dir", draft), "FORBIDDEN")
}

func TestAuditTrail(t *testing.T) {
	h := testkit.TempDir(t)
	expectMsg(t, kc(h, "repo-add", "--repo", "kr://acme/public/core"), "no component fixture")
	body(t, kc(h, "init"))
	seedRepo(t, h, "kr://acme/public/core")
	body(t, kc(h, "put", testObjectReference(t, "kr://acme/public/core", "policy/P-1", ""), "--command-id", "sync-1", "--value", `{"secret":true}`))
	expectCode(t, kc(h, "put", testObjectReference(t, "kr://acme/public/core", "a", ""), "--as", "other", "--command-id", "x", "--value", "1"), "FORBIDDEN")

	trail := asMap(t, body(t, kc(h, "audit", "--layer", "kc")))
	entries := trail["entries"].([]any)
	cmds := make([]string, 0, len(entries))
	var putOk, putDenied map[string]any
	for _, item := range entries {
		row := asMap(t, item)
		cmds = append(cmds, row["cmd"].(string))
		if row["cmd"] == "writer.commit" && row["status"] == "ok" {
			putOk = row
		}
		if row["cmd"] == "writer.commit" && row["status"] == "error" {
			putDenied = row
		}
	}
	joined := strings.Join(cmds, " ")
	for _, want := range []string{"local.repository.attach", "local.init", "writer.commit"} {
		if !strings.Contains(joined, want) {
			t.Fatal(cmds)
		}
	}
	if asMap(t, entries[1])["cmd"] != "local.init" || asMap(t, entries[1])["layer"] != "kc" {
		t.Fatal(entries)
	}
	if refs, ok := asMap(t, entries[1])["refs"].(map[string]any); ok && refs["catalog"] != nil {
		t.Fatal(entries[1])
	}
	if putOk == nil || asMap(t, putOk["args"])["value"] != "<redacted>" {
		t.Fatal(putOk)
	}
	if asMap(t, putOk["refs"])["newCommit"] == "" || asMap(t, putOk["refs"])["disposition"] != "APPLIED" {
		t.Fatal(putOk)
	}
	if putDenied == nil || asMap(t, putDenied["error"])["code"] != "FORBIDDEN" {
		t.Fatal(putDenied)
	}

	inits := asMap(t, body(t, kc(h, "audit", "--layer", "kc", "--cmd", "local.init")))
	if len(inits["entries"].([]any)) != 1 {
		t.Fatal(inits)
	}

	sys := asMap(t, body(t, kc(h, "audit", "--layer", "system")))
	sawCommit, sawCatalogInit := false, false
	for _, item := range sys["entries"].([]any) {
		row := asMap(t, item)
		if row["layer"] != "system" {
			t.Fatal(row)
		}
		switch row["cmd"] {
		case "COMMIT":
			if row["face"] == "writer" && asMap(t, row["refs"])["newCommit"] != "" {
				sawCommit = true
			}
		case "init":
			if row["face"] == "registry" {
				sawCatalogInit = true
			}
		}
	}
	if !sawCommit || !sawCatalogInit {
		t.Fatal(sys)
	}

	catalogLog := asMap(t, body(t, kc(h, "audit")))
	sawInit := false
	for _, item := range catalogLog["entries"].([]any) {
		if strings.HasPrefix(asMap(t, item)["message"].(string), "init kr://kc/registry") {
			sawInit = true
		}
	}
	if !sawInit {
		t.Fatal(catalogLog)
	}

	again := asMap(t, body(t, kc(h, "audit")))
	for _, item := range again["entries"].([]any) {
		if asMap(t, item)["cmd"] == "audit" {
			t.Fatal("audit must not log itself", again)
		}
	}
}

func TestRetiredCatalogIdentityCannotCreateAnotherRegistry(t *testing.T) {
	h := testkit.TempDir(t)
	body(t, kc(h, "init"))
	before := body(t, kc(h, "show"))
	for _, argv := range [][]string{{"catalog-add"}, {"catalog", "use", "acme/docs"}, {"catalog", "archive"}} {
		expectCode(t, kc(h, argv...), "USAGE_INVALID")
	}
	after := body(t, kc(h, "show"))
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("retired selection changed registry: before=%#v after=%#v", before, after)
	}
}
