package cli_test

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"kc/object/format"
)

func TestPublicOnboardingGuidesPreserveHistoryAndScope(t *testing.T) {
	requireOnboardingSchemaMatchesSystem(t)
	world := readExampleWorld(t)
	d := openExampleDeployment(t, world)
	d.publish(t, world, true)
	const reader = "agent:onboarding-reader"
	for _, tc := range []struct {
		dataset, path, repository string
		revision                  int
	}{
		{"qinghe-analysis", "semantic-models/onboarding/readme.md", "sales-semantic", 3},
		{"autumn-launch-kit", "assets/autumn-launch/readme.md", "media", 4},
		{"store-oncall", "services/onboarding/readme.md", "store-ops", 3},
	} {
		t.Run(tc.dataset, func(t *testing.T) {
			raw := d.readDelivered(t, reader, tc.dataset, tc.revision, tc.path)
			unit := format.ParseAt(string(raw), tc.path)
			if unit == nil || string(unit.ObjectID) != "example/"+tc.dataset || unit.SchemaRef != "schema/core/readme/v1" {
				t.Fatalf("public guide lost its object identity or shared Schema: %s", raw)
			}
			ref := "urn:kc:object:example/" + tc.dataset + "#/readme"
			read := asMap(t, body(t, d.as(t, reader, "read", ref, "--dataset", tc.dataset, "--revision", strconv.Itoa(tc.revision))))
			if !reflect.DeepEqual(read["value"], unit.Value) {
				t.Fatalf("file and object guide disagree: %#v", read)
			}
			executeOnboardingReads(t, d, reader, asMap(t, unit.Value)["body"].(string))
			expectCode(t, d.as(t, reader, "files", "read", "--dataset", tc.dataset,
				"--revision", strconv.Itoa(tc.revision-1), "--path", tc.path), "OBJECT_REF_UNRESOLVED")
			expectCode(t, d.as(t, reader, "read", ref, "--repo", tc.repository), "FORBIDDEN")
		})
	}
	if resumed := d.publish(t, world, false); resumed != len(world.Steps) {
		t.Fatalf("teaching publication must resume without replay: %d/%d", resumed, len(world.Steps))
	}
}

func executeOnboardingReads(t *testing.T, d *exampleDeployment, principal, guide string) {
	t.Helper()
	for _, line := range strings.Split(guide, "\n") {
		if !strings.HasPrefix(line, "kc ") || !strings.Contains(line, "--dataset ") || strings.Contains(line, "<") {
			continue
		}
		args := strings.Fields(strings.TrimPrefix(line, "kc "))
		if args[0] != "read" && args[0] != "files" && args[0] != "dataset" {
			t.Fatalf("guide consumption step unexpectedly mutates state: %s", line)
		}
		for i := range args {
			args[i] = strings.Trim(args[i], "'")
			if args[i] == "--out" && i+1 < len(args) {
				args[i+1] = filepath.Join(t.TempDir(), filepath.Base(args[i+1]))
			}
		}
		body(t, d.as(t, principal, args...))
	}
}

func requireOnboardingSchemaMatchesSystem(t *testing.T) {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(repoRoot(), "systemrepo", "content", "_schemas", "readme.v1.aspect.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	copy, err := os.ReadFile(filepath.Join(exampleWorldRoot(), "updates", "onboarding-schema", "drafts", "readme.v1.aspect.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(source, copy) {
		t.Fatal("the shared onboarding Schema import must match the published system contract")
	}
}

func TestPublicLiveAccessExerciseUsesDownloadedMaterials(t *testing.T) {
	world := readExampleWorld(t)
	practice := world.Repositories[0]
	practice.ID = "onboarding-practice"
	world.Repositories = append(world.Repositories, practice)
	d := openExampleDeployment(t, world)
	d.publish(t, world, true)
	const learner, repo = "agent:onboarding-learner", "onboarding-practice"
	body(t, d.as(t, d.operator, "grant", "add", "--principal", learner, "--repo", repo,
		"--action", "object.read,object.schema.read,writer.preview,writer.commit,writer.receipt.read"))
	guide := asMap(t, body(t, d.as(t, learner, "read", "urn:kc:object:example/store-oncall#/readme",
		"--dataset", "store-oncall", "--revision", "3")))
	lab := downloadLiveExercise(t, d, learner, asMap(t, guide["value"])["body"].(string))
	origin := startDownloadedHealthSource(t, lab)
	descriptorPath := filepath.Join(lab, "live", "descriptor.yaml")
	raw, err := os.ReadFile(descriptorPath)
	if err != nil {
		t.Fatal(err)
	}
	writeExampleFile(t, descriptorPath, strings.Replace(string(raw), "http://127.0.0.1:7391", origin, 1))
	publish := func(stage string) string {
		t.Helper()
		head := asMap(t, body(t, d.as(t, learner, "writer", "head", "--repo", repo)))
		receipt := asMap(t, body(t, d.as(t, learner, "writer", "commit", "--repo", repo, "--base", head["commit"].(string),
			"--command-id", "onboarding-"+stage, "--dir", filepath.Join(lab, stage))))
		return asMap(t, receipt["result"])["newCommit"].(string)
	}
	read := func(commit, requestID string) map[string]any {
		t.Helper()
		return asMap(t, body(t, d.as(t, learner, "read", "urn:kc:object:learning/order-service",
			"--repo", repo, "--commit", commit, "--request-id", requestID)))
	}
	staticCommit := publish("static")
	before := read(staticCommit, "lab-static")
	requireLabHealth(t, before, "healthy", "")
	liveCommit := publish("live")
	first := read(liveCommit, "lab-read-1")
	requireLabHealth(t, first, "healthy", "demo-1")
	requireLabStaticSibling(t, before, first)
	writeExampleFile(t, filepath.Join(lab, "state.json"), `{"revision":"demo-2","status":"degraded"}`)
	second := read(liveCommit, "lab-read-2")
	requireLabHealth(t, second, "degraded", "demo-2")
	if first["commit"] != second["commit"] || second["commit"] != liveCommit {
		t.Fatal("changing the source must not move the fixed declaration")
	}
	for _, tc := range []struct{ state, code string }{
		{`{"revision":"demo-2","status":"degraded","fail":true}`, "TEMPORARY_UNAVAILABLE"},
		{`{"revision":"demo-2","status":"degraded","deny":true}`, "FORBIDDEN"},
	} {
		writeExampleFile(t, filepath.Join(lab, "state.json"), tc.state)
		result := read(liveCommit, "lab-"+tc.code)
		requireLabStaticSibling(t, before, result)
		failures, _ := result["failures"].([]any)
		if len(failures) != 1 || asMap(t, failures[0])["code"] != tc.code {
			t.Fatalf("source failure must be preserved at its unit: %#v", result)
		}
		health := asMap(t, asMap(t, asMap(t, result["value"])["runtime"])["health"])
		if health["status"] != nil || asMap(t, health["error"])["code"] != tc.code {
			t.Fatalf("source failure must replace the value with its unit error: %#v", result)
		}
		address := asMap(t, asMap(t, failures[0])["address"])
		if address["objectId"] != "learning/order-service" || address["aspectName"] != "runtime" || address["memberKey"] != "health" {
			t.Fatalf("failure must identify the affected member: %#v", failures)
		}
	}
	requireLabHealth(t, read(staticCommit, "lab-old"), "healthy", "")
	expectCode(t, d.as(t, learner, "operations", "audit", "access", "--repo", repo, "--filter-request-id", "lab-read-1"), "FORBIDDEN")
	body(t, d.as(t, d.operator, "grant", "add", "--principal", learner, "--repo", repo, "--action", "audit.read"))
	audit := body(t, d.as(t, learner, "operations", "audit", "access", "--repo", repo, "--filter-request-id", "lab-read-1"))
	requireLabOriginalEvidence(t, audit, liveCommit)
	writeExampleFile(t, filepath.Join(lab, "state.json"), `{"revision":"demo-3","status":"healthy"}`)
	requireLabHealth(t, read(liveCommit, "lab-recovered"), "healthy", "demo-3")
}

func downloadLiveExercise(t *testing.T, d *exampleDeployment, principal, guide string) string {
	t.Helper()
	lab := t.TempDir()
	count := 0
	for _, line := range strings.Split(guide, "\n") {
		if !strings.HasPrefix(line, "kc files read ") || !strings.Contains(line, "--out ./live-lab/") {
			continue
		}
		args := strings.Fields(strings.TrimPrefix(line, "kc "))
		for i := range args {
			if args[i] != "--out" || i+1 >= len(args) {
				continue
			}
			rel := strings.TrimPrefix(args[i+1], "./live-lab/")
			if !filepath.IsLocal(rel) {
				t.Fatalf("download escapes exercise directory: %s", line)
			}
			args[i+1] = filepath.Join(lab, rel)
			if err := os.MkdirAll(filepath.Dir(args[i+1]), 0700); err != nil {
				t.Fatal(err)
			}
		}
		body(t, d.as(t, principal, args...))
		count++
	}
	if count != 8 {
		t.Fatalf("guide must supply all source and declaration files, got %d", count)
	}
	return lab
}

func writeExampleFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}

func startDownloadedHealthSource(t *testing.T, lab string) string {
	t.Helper()
	python, err := filepath.Abs("../.venv/bin/python")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(python, filepath.Join(lab, "origin.py"), "--state", filepath.Join(lab, "state.json"), "--port", "0")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			ready <- scanner.Text()
		} else {
			ready <- ""
		}
	}()
	select {
	case origin := <-ready:
		if !strings.HasPrefix(origin, "http://127.0.0.1:") {
			t.Fatalf("source did not report a loopback endpoint: %q", origin)
		}
		return origin
	case <-time.After(5 * time.Second):
		t.Fatal("downloaded source failed to start")
		return ""
	}
}

func requireLabStaticSibling(t *testing.T, before, after map[string]any) {
	t.Helper()
	original := asMap(t, asMap(t, before["value"])["readme"])
	if original["body"] == "" || !reflect.DeepEqual(original, asMap(t, after["value"])["readme"]) {
		t.Fatalf("changing or failing health changed static sibling: %#v", after)
	}
}

func requireLabHealth(t *testing.T, result map[string]any, status, revision string) {
	t.Helper()
	health := asMap(t, asMap(t, asMap(t, result["value"])["runtime"])["health"])
	if health["status"] != status {
		t.Fatalf("unexpected health: %#v", result)
	}
	observations, _ := result["observations"].([]any)
	if revision == "" {
		if len(observations) != 0 {
			t.Fatalf("static value acquired an observation: %#v", result)
		}
		return
	}
	if len(observations) != 1 {
		t.Fatalf("expected one external unit observation: %#v", result)
	}
	obs := asMap(t, observations[0])
	if asMap(t, obs["basis"])["sourceRevision"] != revision || obs["declarationCommit"] != result["commit"] {
		t.Fatalf("health lost its source or declaration basis: %#v", result)
	}
}

func requireLabOriginalEvidence(t *testing.T, audit any, commit string) {
	t.Helper()
	entries, _ := asMap(t, audit)["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("expected only the original request: %#v", audit)
	}
	entry := asMap(t, entries[0])
	record := asMap(t, entry["results"])
	units, _ := record["units"].([]any)
	// This request was served before demo-2 and the source still refuses reads.
	// A live re-fetch therefore cannot produce the original successful evidence.
	if entry["requestId"] != "lab-read-1" || record["requestId"] != "lab-read-1" || len(units) != 1 {
		t.Fatalf("audit changed the request or external unit scope: %#v", audit)
	}
	sample := asMap(t, units[0])
	address := asMap(t, sample["address"])
	basis := asMap(t, asMap(t, sample["observation"])["basis"])
	if sample["commit"] != commit || asMap(t, sample["value"])["status"] != "healthy" ||
		basis["sourceRevision"] != "demo-1" || address["objectId"] != "learning/order-service" || address["memberKey"] != "health" {
		t.Fatalf("audit did not retain the original unit, value and basis: %#v", sample)
	}
}
