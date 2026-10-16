package cli_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kc/internal/testkit"
)

// A failed replay must identify the prerequisite that failed, rather than
// retaining the last successful assertion from the previous probe. Run the
// expected failure in a subprocess so its testing.T and report are genuine.
func TestSceneReplayFailureReportIdentifiesCurrentStep(t *testing.T) {
	const modeEnv = "KC_SCENE_REPORT_FAILURE_CASE"
	const dirEnv = "KC_SCENE_REPORT_FAILURE_DIR"
	if mode := os.Getenv(modeEnv); mode != "" {
		probe := "When I run `kc show`\nThen the output has:\n  | catalogId | absent |\n"
		root, child, doc, cache := sceneIsolationFixture(t,
			"Given deployment fixture\nGiven existing repository kr://scene/object\n"+probe,
			map[string]string{"first": probe, "second": probe}, []string{"first", "second"}, probe)
		const repositoriesPerConstruct = 3     // independent Catalog, actual System, then business authority
		failAt := 3 * repositoriesPerConstruct // second probe's business replay fails
		node := root
		if mode == "ancestor" {
			failAt = repositoriesPerConstruct // ancestor's business prerequisite fails
			node = child
		}
		cache.replay = true
		cache.lakefs = &sceneUnavailableOnReplay{sceneLakeFS: cache.lakefs, failAt: failAt}
		node.Dir = os.Getenv(dirEnv)
		runSceneNode(t, doc, node, cache)
		t.Fatal("expected the authority to be unavailable during replay")
		return
	}
	for _, mode := range []string{"probe", "ancestor"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestSceneReplayFailureReportIdentifiesCurrentStep$", "-test.count=1")
			cmd.Env = append(os.Environ(), modeEnv+"="+mode, dirEnv+"="+dir,
				"KC_VALIDATION_RUN_DIR=", "KC_VALIDATION_RUN_ID=", "KC_VALIDATION_SOURCE_FINGERPRINT=",
				"KC_ASSERT_E2E_COVERAGE=", "KC_COMMAND_COVERAGE_REPORT=")
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatal("replay unexpectedly passed")
			}
			raw, err := os.ReadFile(filepath.Join(dir, "_results", "latest.json"))
			if err != nil {
				t.Fatalf("expected failure did not write its report: %v\n%s", err, output)
			}
			var report map[string]any
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatal(err)
			}
			wantState := "isolation-ready"
			if mode == "ancestor" {
				wantState = "isolation-child"
			}
			if report["ok"] != false || report["state"] != wantState || report["last_step_state"] != "isolation-ready" || report["last_step"] != "existing repository kr://scene/object" {
				t.Fatalf("failure report does not identify the actual failed prerequisite: %s\n%s", raw, output)
			}
		})
	}
}

type sceneUnavailableOnReplay struct {
	sceneLakeFS
	calls  int
	failAt int
}

func (f *sceneUnavailableOnReplay) NewRepo() string {
	f.calls++
	if f.calls == f.failAt {
		return "unavailable-replay-repository"
	}
	return f.sceneLakeFS.NewRepo()
}

// These fixtures deliberately mutate through the same protocol commands as
// scene features. The observer must pass alone, in either probe order, and as
// a successor, including when the mutated Snapshot authority is remote.
func TestSceneProbesIsolateAuthorityAndGrants(t *testing.T) {
	sceneProbeAuthorityOrders(t, false, false)
}

func TestSceneProbeReplayIsolatesUncloneableAuthority(t *testing.T) {
	sceneProbeAuthorityOrders(t, true, false)
}

func TestSceneCheckpointProbesReuseAuthorityAndGrants(t *testing.T) {
	sceneProbeAuthorityOrders(t, false, true)
}

type sceneUncloneableAuthority struct{ sceneLakeFS }

func (sceneUncloneableAuthority) ForkStamps(string) error {
	return errors.New("this authority cannot preserve commit IDs in a physical copy; replay is required")
}

func sceneProbeAuthorityOrders(t *testing.T, replay, checkpoint bool) {
	t.Helper()
	for _, order := range []struct {
		name   string
		probes []string
	}{
		{"mutating-first", []string{"mutate", "observe"}},
		{"mutating-last", []string{"observe", "mutate"}},
		{"observe-only", []string{"observe"}},
		{"mutation-only-child", []string{"mutate"}},
	} {
		t.Run(order.name, func(t *testing.T) {
			root, child, doc, cache := sceneIsolationFixture(t, `
Given deployment fixture
Given existing repository kr://scene/object
When I run `+"`kc attach --repo kr://scene/object`"+`
Then the output has:
  | repositoryId | kr://scene/object |
When I run `+"`kc writer put urn:kc:object:note/hello --repo kr://scene/object --command-id baseline --file $materials/baseline.json`"+`
Then the output has:
  | disposition | APPLIED |
When I run `+"`kc grant add --principal reader --action inventory.read`"+`
Then the output has:
  | principal | reader |
`, map[string]string{
				"mutate": `
When I run ` + "`kc writer put urn:kc:object:note/hello --repo kr://scene/object --command-id change --file $materials/changed.json`" + `
Then the output has:
  | disposition | APPLIED |
When I run ` + "`kc read urn:kc:object:note/hello`" + `
Then the output has:
  | value.text | changed |
When I run ` + "`kc grant list`" + `
Then the output has:
  | rules.0.principal | reader |
When I run ` + "`kc grant remove --id $last.rules.0.id`" + `
Then the output has:
  | revoked | nonempty |
When I run ` + "`kc show --as reader`" + `
Then error FORBIDDEN
`,
				"observe": sceneIsolationObserve,
			}, order.probes, sceneIsolationObserve)
			writeSceneIsolationFile(t, filepath.Join(root.Dir, "_materials", "baseline.json"), `{"text":"baseline"}`)
			writeSceneIsolationFile(t, filepath.Join(root.Dir, "_materials", "changed.json"), `{"text":"changed"}`)
			if checkpoint {
				fake := cache.lakefs.(*testkit.LakeFSFake)
				nodes := cache.nodes
				cache = newSceneHomeCacheWith(t, testkit.NewLakeFSLive(t, fake.Origin(), fake.Credential(), "s3://checkpoint-test"))
				cache.nodes = nodes
			}
			if replay {
				cache.replay = true
				cache.lakefs = sceneUncloneableAuthority{cache.lakefs}
			}
			runSceneNode(t, doc, root, cache)
			runSceneNode(t, doc, child, cache)
			wantInits := 1
			if replay {
				wantInits = len(order.probes) + 2 // node, each probe, then successor
			}
			if cache.inits != wantInits {
				t.Fatalf("deployment initialized %d times, want %d (replay=%t)", cache.inits, wantInits, replay)
			}
		})
	}
}

const sceneIsolationObserve = `
When I run ` + "`kc read urn:kc:object:note/hello`" + `
Then the output has:
  | value.text | baseline |
When I run ` + "`kc show --as reader`" + `
Then the output has:
  | catalogId | absent |
`

func TestSceneProbesIsolateHTTPAndClientSessions(t *testing.T) {
	for _, order := range []struct {
		name   string
		probes []string
	}{
		{"login-first", []string{"login", "anonymous"}},
		{"login-last", []string{"anonymous", "login"}},
		{"anonymous-only", []string{"anonymous"}},
		{"revoke-first", []string{"revoke", "anonymous"}},
		{"revoke-last", []string{"anonymous", "revoke"}},
	} {
		t.Run(order.name, func(t *testing.T) {
			anonymous := `
When I run ` + "`kc whoami --server $server`" + `
Then error UNAUTHENTICATED
When HTTP GET /identity/v1/whoami as reader
Then whoami is reader
When I run ` + "`kc show --as reader --server $server`" + `
Then the output has:
  | catalogId | absent |
`
			root, child, doc, cache := sceneIsolationFixture(t, `
Given deployment fixture
When I run `+"`kc grant add --principal reader --action inventory.read`"+`
Then the output has:
  | principal | reader |
Given local HTTP server
`, map[string]string{
				"login": `
When I run ` + "`kc login --server $server --mode local --as reader`" + `
Then the output has:
  | principal | reader |
When I run ` + "`kc whoami --server $server`" + `
Then the output has:
  | principal | reader |
`,
				"anonymous": anonymous,
				"revoke": `
When I run ` + "`kc grant list`" + `
Then the output has:
  | rules.0.principal | reader |
When I run ` + "`kc grant remove --id $last.rules.0.id`" + `
Then the output has:
  | revoked | nonempty |
When I run ` + "`kc show --as reader --server $server`" + `
Then error FORBIDDEN
`,
			}, order.probes, anonymous)
			runSceneNode(t, doc, root, cache)
			runSceneNode(t, doc, child, cache)
			if cache.inits != 1 {
				t.Fatalf("deployment initialized %d times; HTTP resources must be recreated from the frozen home", cache.inits)
			}
		})
	}
}

func TestSceneProjectionProbesRestoreSharedBaseline(t *testing.T) {
	sceneProjectionProbeOrders(t, false)
}

func TestSceneCheckpointProjectionProbesRestoreSharedBaseline(t *testing.T) {
	sceneProjectionProbeOrders(t, true)
}

func sceneProjectionProbeOrders(t *testing.T, checkpoint bool) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL")) == "" {
		t.Fatal("KC_TEST_OPENSEARCH_URL is required for the scene projection isolation contract")
	}
	for _, order := range []struct {
		name   string
		probes []string
	}{
		{"mutating-first", []string{"mutate", "observe"}},
		{"mutating-last", []string{"observe", "mutate"}},
		{"observe-only", []string{"observe"}},
	} {
		t.Run(order.name, func(t *testing.T) {
			doc := loadSceneCatalog(t)
			cache := newSceneHomeCache(t)
			if checkpoint {
				fake := cache.lakefs.(*testkit.LakeFSFake)
				nodes := cache.nodes
				cache = newSceneHomeCacheWith(t, testkit.NewLakeFSLive(t, fake.Origin(), fake.Credential(), "s3://checkpoint-test"))
				cache.nodes = nodes
			}
			node, ok := cache.nodes["semantic-dataset-defined"]
			if !ok {
				t.Fatal("prepared Dataset state missing")
			}
			republish := sceneIsolationRepublish(t, node)
			// Reuse the actual publication spine, but keep this runner's
			// adversarial probes and results in an independent fixture.
			node.Dir = t.TempDir()
			node.Probes = nil
			observe := `
When I run ` + "`kc operations projection describe --dataset scene-set`" + `
Then the output has:
  | dataset | scene-set |
  | revision | 1 |
  | snapshot.state | READY |
When I run ` + "`kc search --dataset scene-set --revision 1 --query merchandise`" + `
Then 1 hit metric/gmv
When I run ` + "`kc read urn:kc:object:metric/gmv#/definition --dataset scene-set --revision 1`" + `
Then the output has:
  | value.name | Gross merchandise value |
`
			probes := map[string]string{
				"observe": observe,
				"mutate": `
When I run ` + "`kc writer put urn:kc:object:metric/gmv#/definition --repo kr://scene/object --command-id changed-projection --schema-ref schema/metric.definition --file $materials/changed.json`" + `
Then the output has:
  | disposition | APPLIED |
When I run ` + "`" + republish + "`" + `
Then the output has:
  | setId | scene-set |
  | revision | 2 |
When I run ` + "`kc operations projection sync --dataset scene-set --revision 2`" + `
Then the output has:
  | dataset | scene-set |
  | revision | 2 |
  | snapshot.mode | ready |
When I run ` + "`kc search --dataset scene-set --revision 2 --query merchandise`" + `
Then 0 hits
When I run ` + "`kc read urn:kc:object:metric/gmv#/definition --dataset scene-set --revision 2`" + `
Then the output has:
  | value.name | Changed total |
`,
			}
			writeSceneIsolationFile(t, filepath.Join(node.Dir, "_materials", "changed.json"), `{"name":"Changed total","expression":"SUM(amount)","unit":"CNY","measureKey":"changed-measure"}`)
			for _, name := range order.probes {
				path := filepath.Join(node.Dir, "_probes", name+".feature")
				writeSceneIsolationFile(t, path, "Feature: projection isolation\nScenario: "+name+"\n"+probes[name])
				node.Probes = append(node.Probes, path)
			}
			cache.nodes[node.ID] = node
			child := sceneTreeNode{
				ID: "projection-isolation-child", Dir: filepath.Join(node.Dir, "child"),
				Ancestors: append(append([]string{}, node.Ancestors...), node.ID),
				Construct: filepath.Join(node.Dir, "child", "_build", "construct.feature"),
			}
			writeSceneIsolationFile(t, child.Construct, "Feature: projection isolation\nScenario: successor\n"+observe)
			cache.nodes[child.ID] = child
			doc.States = append(doc.States, sceneCatalogState{ID: child.ID, Surface: "scene"})
			runSceneNode(t, doc, node, cache)
			runSceneNode(t, doc, child, cache)
		})
	}
}

func TestSceneProbesRestoreAbsentProjection(t *testing.T) {
	if strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL")) == "" {
		t.Fatal("KC_TEST_OPENSEARCH_URL is required for the absent projection isolation contract")
	}
	for _, order := range []struct {
		name   string
		probes []string
	}{
		{"build-first", []string{"build", "absent"}},
		{"build-last", []string{"absent", "build"}},
		{"absent-only", []string{"absent"}},
	} {
		t.Run(order.name, func(t *testing.T) {
			absent := "When I run `kc relations --dataset isolated-notes --revision 1 --object note/hello`\nThen error CAPABILITY_UNSATISFIED\n"
			root, child, doc, cache := sceneIsolationFixture(t, `
Given deployment fixture
Given existing repository kr://scene/object
When I run `+"`kc attach --repo kr://scene/object`"+`
Then the output has:
  | repositoryId | kr://scene/object |
When I run `+"`kc dataset define --dataset isolated-notes --revision 1 --source kr://scene/object=refs/heads/main`"+`
Then the output has:
  | setId | isolated-notes |
  | revision | 1 |
`, map[string]string{
				"absent": absent,
				"build": `
When I run ` + "`kc operations projection sync --dataset isolated-notes --revision 1`" + `
Then the output has:
  | dataset | isolated-notes |
  | revision | 1 |
  | members.0.repository | kr://scene/object |
  | members.0.commit | nonempty |
  | snapshot.mode | ready |
  | state.mode | rebuild |
When I run ` + "`kc relations --dataset isolated-notes --revision 1 --object note/hello`" + `
Then the output has:
  | hits | [] |
`,
			}, order.probes, absent)
			freezeSceneWithoutProjection(t, root, cache)
			for _, probe := range root.Probes {
				runIsolatedSceneProbe(t, root, probe, cache, &sceneRunReport{})
			}
			runSceneNode(t, doc, child, cache)
		})
	}
}

func TestSceneIndexedWorldDoesNotLeakIntoUnindexedWorld(t *testing.T) {
	if strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL")) == "" {
		t.Fatal("KC_TEST_OPENSEARCH_URL is required for the cross-world projection isolation contract")
	}
	doc := loadSceneCatalog(t)
	indexed := newSceneHomeCache(t)
	indexedNode, ok := indexed.nodes["semantic-dataset-defined"]
	if !ok {
		t.Fatal("prepared Dataset state missing")
	}
	runSceneNode(t, doc, indexedNode, indexed)
	// This separate cache repeats logical Repository IDs and fake commit IDs,
	// just as TestProductScenes does after TestMetricPermissionScenes.
	unindexed := newSceneHomeCache(t)
	unindexedNode := unindexed.nodes["semantic-dataset-defined"]
	freezeSceneWithoutProjection(t, unindexedNode, unindexed)
	probe := filepath.Join(t.TempDir(), "absent.feature")
	writeSceneIsolationFile(t, probe, "Feature: cross-world projection isolation\nScenario: absent projection\nWhen I run `kc relations --dataset scene-set --revision 1 --object metric/gmv`\nThen error CAPABILITY_UNSATISFIED\n")
	runIsolatedSceneProbe(t, unindexedNode, probe, unindexed, &sceneRunReport{})
}

func freezeSceneWithoutProjection(t *testing.T, node sceneTreeNode, cache *sceneHomeCache) {
	t.Helper()
	cache.ensureFrozen(t, node, nil)
	media := cache.frozen[node.ID].projection
	if media == nil || len(media.indices) == 0 {
		t.Fatal("accepted Dataset did not prepare projection media")
	}
	// Publication prepares the Dataset first. Remove only this frozen world's
	// exact owned media so every probe retains the accepted files and revision
	// while starting without a projection; no other world's indices are touched.
	media.close()
}

func sceneIsolationRepublish(t *testing.T, node sceneTreeNode) string {
	t.Helper()
	for _, step := range nodeConstructSteps(t, node) {
		if step.kind == "run" && strings.HasPrefix(step.command, "kc dataset define --dataset scene-set --revision 1 ") {
			return strings.Replace(step.command, "--revision 1 ", "--revision 2 ", 1)
		}
	}
	t.Fatal("prepared Dataset construct has no publication command")
	return ""
}

func sceneIsolationFixture(t *testing.T, construct string, probes map[string]string, order []string, childConstruct string) (sceneTreeNode, sceneTreeNode, sceneCatalogFile, *sceneHomeCache) {
	t.Helper()
	dir := t.TempDir()
	root := sceneTreeNode{ID: "isolation-ready", Dir: dir, Construct: filepath.Join(dir, "_build", "construct.feature")}
	writeSceneIsolationFile(t, root.Construct, "Feature: isolation\nScenario: construct\n"+construct)
	for _, name := range order {
		path := filepath.Join(dir, "_probes", name+".feature")
		writeSceneIsolationFile(t, path, "Feature: isolation\nScenario: "+name+"\n"+probes[name])
		root.Probes = append(root.Probes, path)
	}
	child := sceneTreeNode{ID: "isolation-child", Dir: filepath.Join(dir, "child"), Ancestors: []string{root.ID}, Construct: filepath.Join(dir, "child", "_build", "construct.feature")}
	writeSceneIsolationFile(t, child.Construct, "Feature: isolation\nScenario: successor\n"+childConstruct)
	doc := sceneCatalogFile{States: []sceneCatalogState{{ID: root.ID, Surface: "scene"}, {ID: child.ID, Surface: "scene"}}}
	cache := &sceneHomeCache{owner: t, nodes: map[string]sceneTreeNode{root.ID: root, child.ID: child}, frozen: map[string]frozenSceneHome{}, lakefs: testkit.NewLakeFSFake(t)}
	return root, child, doc, cache
}

func writeSceneIsolationFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
