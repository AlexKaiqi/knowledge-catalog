package cli_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"

	"kc/cli"
	"kc/internal/testkit"
	"kc/kernel"
)

type sceneFeature struct {
	scenarios []sceneScenario
}

type sceneAgentTask struct {
	principal string
	fixture   string
	brief     string
}

type sceneScenario struct {
	name       string
	tags       []string
	brief      string
	agentTasks []sceneAgentTask
	steps      []sceneStep
}

type sceneStep struct {
	line       int
	kind       string
	text       string
	table      [][]string
	command    string
	method     string
	path       string
	principal  string
	onBehalfOf string
	object     string
	errorCode  string
	whoami     string
	material   string
	fixtureDir string
	schema     *sceneSchemaSpec
	instance   *sceneInstanceSpec
}

type sceneSchemaSpec struct {
	object  string
	repo    string
	entity  string
	aspect  string
	pattern string
	fields  []sceneSchemaField
}

type sceneSchemaField struct {
	name     string
	typ      string
	required bool
	access   []string
}

type sceneInstanceSpec struct {
	object    string
	repo      string
	aspect    string
	schemaRef string
	fields    map[string]string
}

type sceneWorld struct {
	t           *testing.T
	home        string
	seq         int
	lastKind    string
	cli         kcRunResult
	lastPayload any
	ids         map[string]string
	httpServer  *httptest.Server
	httpCode    int
	httpBody    any
	canonical   map[string]string
	cache       *sceneHomeCache
	closeHTTP   func()
	projection  *sceneProjectionNamespace
}

// sceneHomeCache freezes each node's home after its own construct (not after
// probes). Children copy that snapshot instead of replaying ancestor
// constructs from an empty home. Snapshots live in the owner test's TempDir,
// never in `_results/`.
type sceneLakeFS interface {
	NewRepo() string
	DSN(name string) string
	OwnsDSN(dsn string) bool
	ForkStamps(dst string) error
	Credential() string
}

type sceneHomeCache struct {
	owner  *testing.T
	nodes  map[string]sceneTreeNode
	mu     sync.Mutex
	frozen map[string]frozenSceneHome
	inits  int
	lakefs sceneLakeFS
	replay bool
}

type frozenSceneHome struct {
	dir       string
	ids       map[string]string
	canonical map[string]string
	seq       int
	http      bool
}

var (
	reSchema = regexp.MustCompile(`^Schema (\S+) in (\S+) \(entity (\S+), aspect (\S+), pattern (\S+)\):$`)
	reInst   = regexp.MustCompile(`^instance (\S+) in (\S+) \(aspect (\S+), schema-ref (\S+)\):$`)
	reHTTP   = regexp.MustCompile(`^HTTP (GET|POST) (\S+)(?: as (\S+)(?: on-behalf-of (\S+))?)?$`)
	reHit    = regexp.MustCompile(`^1 hit (\S+) with (body stripped|full canonical)$`)
)

func TestMetricPermissionSceneFileParses(t *testing.T) {
	doc := loadSceneCatalog(t)
	var tasks []sceneAgentTask
	steps := 0
	for _, node := range discoverConstructableNodes(t) {
		if !nodeNeedsIndex(node) {
			continue
		}
		nodeSteps, agents := composeSceneNode(t, doc, node)
		steps += len(nodeSteps)
		tasks = append(tasks, agents...)
	}
	if steps < 20 {
		t.Fatalf("search tree executable steps=%d", steps)
	}
	want := []sceneAgentTask{
		{principal: "taihu:alice", fixture: "search-only"},
		{principal: "searcher", fixture: "search-only"},
		{principal: "searcher", fixture: "search+read"},
	}
	if len(tasks) != len(want) {
		t.Fatalf("agent tasks=%d want %d: %#v", len(tasks), len(want), tasks)
	}
	// Independent probes have no ordering contract; retain every role/brief.
	expected := map[string]bool{}
	for _, task := range want {
		expected[task.principal+"/"+task.fixture] = true
	}
	for _, task := range tasks {
		key := task.principal + "/" + task.fixture
		if !expected[key] || strings.TrimSpace(task.brief) == "" {
			t.Fatalf("unexpected, duplicate or empty Agent task: %#v", task)
		}
		delete(expected, key)
	}
	if len(expected) != 0 {
		t.Fatalf("missing Agent task roles: %v", expected)
	}
}

func TestMetricPermissionAgentCompanionStaysOnTheFeature(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	raw, err := os.ReadFile(filepath.Join(root, "dsh-plugin", "scripts", "agent-scenarios.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		ExtendedCompanions []struct {
			ID   string `json:"id"`
			Spec string `json:"spec"`
		} `json:"extendedCompanions"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	var spec string
	for _, companion := range catalog.ExtendedCompanions {
		if companion.ID == "KC-AGENT-01" {
			spec = companion.Spec
		}
	}
	if spec != ".data/scenes" {
		t.Fatalf("KC-AGENT-01 spec=%q", spec)
	}
	if _, err := os.Stat(filepath.Join(root, spec, "catalog-initialized", "_meta.yaml")); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("python3", filepath.Join(root, "dsh-plugin", "scripts", "e2e_agent_metric_permission.py"), "--check-only").CombinedOutput()
	if err != nil {
		t.Fatalf("Agent companion scene references: %v\n%s", err, out)
	}
}

func TestMetricPermissionScenes(t *testing.T) {
	doc := loadSceneCatalog(t)
	cache := newSceneHomeCache(t)
	n := 0
	for _, node := range discoverConstructableNodes(t) {
		if !nodeNeedsIndex(node) || nodeNeedsState(node) || nodeNeedsWalk(node) {
			continue
		}
		n++
		node := node
		t.Run(node.ID, func(t *testing.T) {
			runSceneNode(t, doc, node, cache)
		})
	}
	if n == 0 {
		t.Fatal("no constructable search nodes")
	}
}

func TestProductScenes(t *testing.T) {
	doc := loadSceneCatalog(t)
	cache := newSceneHomeCache(t)
	n := 0
	for _, node := range discoverConstructableNodes(t) {
		if nodeNeedsIndex(node) || nodeNeedsState(node) || nodeNeedsWalk(node) {
			continue
		}
		n++
		node := node
		t.Run(node.ID, func(t *testing.T) {
			runSceneNode(t, doc, node, cache)
		})
	}
	if n == 0 {
		t.Fatal("no constructable product nodes")
	}
}

func TestLiveLakeFSSceneDFS(t *testing.T) {
	origin := strings.TrimSpace(os.Getenv("KC_SCENE_LIVE_LAKEFS_URL"))
	if origin == "" {
		t.Skip("set KC_SCENE_LIVE_LAKEFS_URL to DFS .data/scenes against Graveler")
	}
	cred := strings.TrimSpace(os.Getenv("KC_LAKEFS_CREDENTIAL"))
	if cred == "" {
		t.Fatal("KC_LAKEFS_CREDENTIAL is required for the live scene DFS")
	}
	doc := loadSceneCatalog(t)
	live := testkit.NewLakeFSLive(t, origin, cred, strings.TrimSpace(os.Getenv("KC_SCENE_LIVE_LAKEFS_STORAGE")))
	nodes := discoverConstructableNodes(t)
	if len(nodes) == 0 {
		t.Fatal("no constructable nodes")
	}
	indexURL := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL"))
	stateURL := strings.TrimSpace(os.Getenv("KC_TEST_STATE_RUNTIME_URL"))
	for _, node := range nodes {
		node := node
		t.Run(node.ID, func(t *testing.T) {
			if nodeNeedsWalk(node) {
				t.Skip("named-repositories-created is the live walk leaf; use scenes/goto.py")
			}
			if nodeNeedsState(node) && stateURL == "" {
				t.Skip("dynamic State execution needs KC_TEST_STATE_RUNTIME_URL")
			}
			if nodeNeedsIndex(node) && indexURL == "" {
				t.Skip("index spine needs KC_TEST_OPENSEARCH_URL")
			}
			// Live Graveler cannot fork commit IDs, so each node replays from
			// root on a fresh cache instead of cloning a parent home.
			cache := newSceneHomeCacheWith(t, live)
			runSceneNode(t, doc, node, cache)
		})
	}
}

func TestSceneExecutorReusesParentConstructHome(t *testing.T) {
	doc := loadSceneCatalog(t)
	cache := newSceneHomeCache(t)
	var root, child sceneTreeNode
	for _, node := range discoverConstructableNodes(t) {
		switch node.ID {
		case "catalog-initialized":
			root = node
		case "source-repositories-configured":
			child = node
		}
	}
	if root.ID == "" || child.ID == "" {
		t.Fatal("catalog-initialized / source-repositories-configured missing")
	}
	runSceneNode(t, doc, root, cache)
	runSceneNode(t, doc, child, cache)
	if cache.inits != 1 {
		t.Fatalf("deployment fixture initialized %d times; child must reuse the frozen parent state", cache.inits)
	}
}

func TestSceneExistingRepositoryFixtureUsesLakeFS(t *testing.T) {
	doc := loadSceneCatalog(t)
	cache := newSceneHomeCache(t)
	var published, attached sceneTreeNode
	for _, node := range discoverConstructableNodes(t) {
		switch node.ID {
		case "source-repositories-configured":
			published = node
		case "repository-attached":
			attached = node
		}
	}
	if published.ID == "" || attached.ID == "" {
		t.Fatal("source-repositories-configured / repository-attached missing")
	}
	runSceneNode(t, doc, published, cache)
	cache.mu.Lock()
	frozen := cache.frozen[published.ID]
	cache.mu.Unlock()
	stamps := sceneRemoteStamps(t, frozen.dir)
	if len(stamps) != 2 {
		t.Fatalf("frozen existing-repository stamps=%d want knowledge and graph: %#v", len(stamps), stamps)
	}
	wantRepositories := map[string]bool{"kr://scene/knowledge": true, "kr://scene/graph": true}
	for _, stamp := range stamps {
		if !wantRepositories[stamp.id] || stamp.driver != "lakefs" || !cache.lakefs.OwnsDSN(stamp.dsn) {
			t.Fatalf("unexpected existing repository stamp: %#v", stamp)
		}
		delete(wantRepositories, stamp.id)
	}
	homeA, _ := cache.cloneParent(t, attached)
	homeB, _ := cache.cloneParent(t, attached)
	stampsA := sceneRemoteStamps(t, homeA)
	stampsB := sceneRemoteStamps(t, homeB)
	if len(stampsA) != 2 || len(stampsB) != 2 {
		t.Fatalf("cloned stamps A=%#v B=%#v", stampsA, stampsB)
	}
	seenDSNs := map[string]bool{}
	for _, group := range [][]sceneRemoteStamp{stamps, stampsA, stampsB} {
		for _, stamp := range group {
			if seenDSNs[stamp.dsn] {
				t.Fatalf("copied homes share lakeFS physical repository %s: parent=%#v A=%#v B=%#v", stamp.dsn, stamps, stampsA, stampsB)
			}
			seenDSNs[stamp.dsn] = true
		}
	}
}

func TestSceneRunWritesLatestResult(t *testing.T) {
	doc := loadSceneCatalog(t)
	var node sceneTreeNode
	for _, candidate := range discoverConstructableNodes(t) {
		if candidate.ID == "catalog-initialized" {
			node = candidate
			break
		}
	}
	if node.ID == "" {
		t.Fatal("catalog-initialized is not constructable")
	}
	runSceneNode(t, doc, node, newSceneHomeCache(t))
	raw, err := os.ReadFile(filepath.Join(node.Dir, "_results", "latest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report sceneRunReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if !report.OK || report.State != "catalog-initialized" {
		t.Fatalf("result = %#v", report)
	}
	if report.ElapsedMS < 0 {
		t.Fatalf("elapsed_ms=%d", report.ElapsedMS)
	}
	if report.StartedAt == "" || report.FinishedAt == "" || report.Test != t.Name() {
		t.Fatalf("scene result has no execution identity: %#v", report)
	}
	if runDir := os.Getenv("KC_VALIDATION_RUN_DIR"); runDir != "" {
		if report.RunID != os.Getenv("KC_VALIDATION_RUN_ID") || report.SourceFingerprint != os.Getenv("KC_VALIDATION_SOURCE_FINGERPRINT") {
			t.Fatalf("scene result detached from parent run: %#v", report)
		}
		bound, err := os.ReadFile(filepath.Join(runDir, "scenes", t.Name(), report.ExecutionID+".json"))
		if err != nil || string(bound) != string(raw) {
			t.Fatalf("run-scoped result differs from latest: %v", err)
		}
	}
}

func TestSceneResultKeepsRunScopedHistory(t *testing.T) {
	runDir, nodeDir := t.TempDir(), t.TempDir()
	t.Setenv("KC_VALIDATION_RUN_DIR", runDir)
	first := sceneRunReport{RunID: "same-run", State: "example-state", Test: "FirstJourney", OK: true}
	if err := writeSceneResult(nodeDir, first); err != nil {
		t.Fatal(err)
	}
	second := sceneRunReport{RunID: "same-run", State: "example-state", Test: "FirstJourney", OK: false}
	if err := writeSceneResult(nodeDir, second); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(runDir, "scenes", first.Test, first.State+"-*.json"))
	if err != nil || len(paths) != 2 {
		t.Fatalf("same-run repeated test lost history: %v %v", paths, err)
	}
	statuses := map[bool]int{}
	executions := map[string]bool{}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var got sceneRunReport
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if got.Test != first.Test || got.ExecutionID == "" || executions[got.ExecutionID] {
			t.Fatalf("journey result overwritten: %#v", got)
		}
		executions[got.ExecutionID] = true
		statuses[got.OK]++
	}
	if statuses[true] != 1 || statuses[false] != 1 {
		t.Fatalf("prior pass/fail lost: %#v", statuses)
	}
}

func runSceneNode(t *testing.T, doc sceneCatalogFile, node sceneTreeNode, cache *sceneHomeCache) {
	t.Helper()
	if cache == nil {
		cache = newSceneHomeCache(t)
	}
	started := time.Now()
	report := sceneRunReport{
		State:             node.ID,
		Ancestors:         append([]string{}, node.Ancestors...),
		Probes:            []string{},
		RunID:             os.Getenv("KC_VALIDATION_RUN_ID"),
		SourceFingerprint: os.Getenv("KC_VALIDATION_SOURCE_FINGERPRINT"),
		Test:              t.Name(),
		StartedAt:         started.UTC().Format(time.RFC3339Nano),
	}
	if shouldRunSceneProbes(doc, node.ID) {
		for _, probe := range node.Probes {
			report.Probes = append(report.Probes, filepath.Base(probe))
		}
	}
	defer func() {
		report.ElapsedMS = time.Since(started).Milliseconds()
		report.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := writeSceneResult(node.Dir, report); err != nil {
			t.Errorf("write _results: %v", err)
		}
	}()
	world := cache.worldAtParent(t, node, &report)
	defer world.close()
	runSceneConstruct(t, node, world, &report)
	cache.snapshot(node.ID, world)
	world.close()
	if shouldRunSceneProbes(doc, node.ID) {
		for _, probe := range node.Probes {
			runIsolatedSceneProbe(t, node, probe, cache, &report)
		}
	}
	report.OK = !t.Failed()
}

func runIsolatedSceneProbe(t *testing.T, node sceneTreeNode, probe string, cache *sceneHomeCache, report *sceneRunReport) {
	t.Helper()
	report.observeStep(node.ID, "prepare probe "+filepath.Base(probe))
	var world *sceneWorld
	if cache.requiresReplay(node) {
		world = cache.replayConstructs(t, append(append([]string{}, node.Ancestors...), node.ID), report)
	} else {
		world = cache.cloneWorld(t, node.ID)
	}
	defer world.close()
	report.observeStep(node.ID, "load "+probe)
	steps, _ := loadSceneFeatureSteps(t, probe, filepath.Join(node.Dir, "_materials"))
	for _, step := range steps {
		report.observeStep(node.ID, step.text)
		world.run(step)
	}
}

func runSceneConstruct(t *testing.T, node sceneTreeNode, world *sceneWorld, report *sceneRunReport) {
	t.Helper()
	report.observeStep(node.ID, "load "+node.Construct)
	for _, step := range nodeConstructSteps(t, node) {
		report.observeStep(node.ID, step.text)
		world.run(step)
	}
}

func newSceneHomeCache(t *testing.T) *sceneHomeCache {
	t.Helper()
	return newSceneHomeCacheWith(t, testkit.NewLakeFSFake(t))
}

func newSceneHomeCacheWith(t *testing.T, lakefs sceneLakeFS) *sceneHomeCache {
	t.Helper()
	nodes := map[string]sceneTreeNode{}
	for _, node := range discoverConstructableNodes(t) {
		nodes[node.ID] = node
	}
	// Real Graveler cannot preserve a commit graph while copying a physical
	// repository. Its probes and descendants must replay their prerequisites.
	_, replay := lakefs.(*testkit.LakeFSLive)
	return &sceneHomeCache{owner: t, nodes: nodes, frozen: map[string]frozenSceneHome{}, lakefs: lakefs, replay: replay}
}

func (c *sceneHomeCache) recordInit() {
	c.mu.Lock()
	c.inits++
	c.mu.Unlock()
}

func (c *sceneHomeCache) snapshot(id string, world *sceneWorld) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.frozen[id]; ok {
		return
	}
	dst := testkit.TempDir(c.owner)
	if err := copySceneHome(world.home, dst); err != nil {
		c.owner.Fatalf("freeze %s: %v", id, err)
	}
	if !c.replay {
		// A directory copy alone still points at mutable remote HEADs. Freeze
		// those authorities before any probe can change the constructing world.
		if err := c.lakefs.ForkStamps(dst); err != nil {
			c.owner.Fatalf("freeze lakeFS stamps for %s: %v", id, err)
		}
	}
	c.frozen[id] = frozenSceneHome{
		dir: dst, ids: remapSceneIDs(world.ids, world.home, dst),
		canonical: remapSceneIDs(world.canonical, "", ""), seq: world.seq, http: world.httpServer != nil,
	}
}

func (c *sceneHomeCache) cloneParent(t *testing.T, node sceneTreeNode) (string, map[string]string) {
	t.Helper()
	if len(node.Ancestors) == 0 {
		return testkit.TempDir(t), map[string]string{}
	}
	parentID := node.Ancestors[len(node.Ancestors)-1]
	parent, ok := c.nodes[parentID]
	if !ok {
		t.Fatalf("parent %s of %s is not constructable", parentID, node.ID)
	}
	c.ensureFrozen(t, parent, nil)
	return c.cloneFrozen(t, parentID)
}

func (c *sceneHomeCache) cloneFrozen(t *testing.T, id string) (string, map[string]string) {
	t.Helper()
	c.mu.Lock()
	src, ok := c.frozen[id]
	c.mu.Unlock()
	if !ok {
		t.Fatalf("state %s has not been frozen", id)
	}
	dst := testkit.TempDir(t)
	if err := copySceneHome(src.dir, dst); err != nil {
		t.Fatalf("clone state %s: %v", id, err)
	}
	if err := c.lakefs.ForkStamps(dst); err != nil {
		t.Fatalf("isolate lakeFS stamps for %s: %v", id, err)
	}
	return dst, remapSceneIDs(src.ids, src.dir, dst)
}

func (c *sceneHomeCache) worldAtParent(t *testing.T, node sceneTreeNode, report *sceneRunReport) *sceneWorld {
	t.Helper()
	if c.requiresReplay(node) {
		return c.replayConstructs(t, node.Ancestors, report)
	}
	if len(node.Ancestors) == 0 {
		return newSceneWorldAt(t, testkit.TempDir(t), c)
	}
	parentID := node.Ancestors[len(node.Ancestors)-1]
	parent, ok := c.nodes[parentID]
	if !ok {
		t.Fatalf("parent %s of %s is not constructable", parentID, node.ID)
	}
	c.ensureFrozen(t, parent, report)
	return c.cloneWorld(t, parentID)
}

func (c *sceneHomeCache) requiresReplay(node sceneTreeNode) bool {
	if c.replay || nodeNeedsIndex(node) {
		return true
	}
	// A new world has an empty, isolated OpenSearch namespace. Restore the
	// indexed build chain before each consumer; a home copy cannot restore
	// external projection media. Scene execution remains sequential.
	for _, id := range node.Ancestors {
		if nodeNeedsIndex(c.nodes[id]) {
			return true
		}
	}
	return false
}

func (c *sceneHomeCache) cloneWorld(t *testing.T, id string) *sceneWorld {
	t.Helper()
	home, ids := c.cloneFrozen(t, id)
	world := newSceneWorldAt(t, home, c)
	world.ids = ids
	c.mu.Lock()
	src := c.frozen[id]
	c.mu.Unlock()
	world.seq = src.seq
	world.canonical = remapSceneIDs(src.canonical, "", "")
	if src.http {
		// Listening sockets and handler caches belong to this clone. Copying
		// a URL would send requests back to the constructing state's home.
		world.startHTTPServer()
	}
	return world
}

func (c *sceneHomeCache) replayConstructs(t *testing.T, chain []string, report *sceneRunReport) *sceneWorld {
	t.Helper()
	world := newSceneWorldAt(t, testkit.TempDir(t), c)
	for _, id := range chain {
		node, ok := c.nodes[id]
		if !ok {
			t.Fatalf("replay state %s is not constructable", id)
		}
		runSceneConstruct(t, node, world, report)
	}
	return world
}

func (c *sceneHomeCache) ensureFrozen(t *testing.T, node sceneTreeNode, report *sceneRunReport) {
	t.Helper()
	c.mu.Lock()
	_, ok := c.frozen[node.ID]
	c.mu.Unlock()
	if ok {
		return
	}
	world := c.worldAtParent(t, node, report)
	defer world.close()
	runSceneConstruct(t, node, world, report)
	c.snapshot(node.ID, world)
}

func remapSceneIDs(ids map[string]string, oldHome, newHome string) map[string]string {
	out := map[string]string{}
	for key, value := range ids {
		if oldHome != "" && (value == oldHome || strings.HasPrefix(value, oldHome+string(os.PathSeparator))) {
			out[key] = newHome + strings.TrimPrefix(value, oldHome)
			continue
		}
		out[key] = value
	}
	return out
}

func copySceneHome(src, dst string) error {
	return os.CopyFS(dst, os.DirFS(src))
}

type sceneRemoteStamp struct {
	id     string
	driver string
	dsn    string
}

func sceneRemoteStamps(t *testing.T, home string) []sceneRemoteStamp {
	t.Helper()
	var stamps []sceneRemoteStamp
	err := filepath.WalkDir(home, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || d.Name() != "remote.yaml" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var stamp struct {
			ID     string `yaml:"id"`
			Driver string `yaml:"driver"`
			DSN    string `yaml:"dsn"`
		}
		if err := yaml.Unmarshal(raw, &stamp); err != nil {
			return err
		}
		stamps = append(stamps, sceneRemoteStamp{id: stamp.ID, driver: stamp.Driver, dsn: stamp.DSN})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return stamps
}

type sceneRunReport struct {
	ExecutionID       string   `json:"executionId,omitempty"`
	RunID             string   `json:"runId,omitempty"`
	SourceFingerprint string   `json:"sourceFingerprint,omitempty"`
	Test              string   `json:"test"`
	StartedAt         string   `json:"startedAt"`
	FinishedAt        string   `json:"finishedAt"`
	State             string   `json:"state"`
	OK                bool     `json:"ok"`
	ElapsedMS         int64    `json:"elapsed_ms"`
	Ancestors         []string `json:"ancestors"`
	Probes            []string `json:"probes"`
	LastStep          string   `json:"last_step,omitempty"`
	LastStepState     string   `json:"last_step_state,omitempty"`
}

func (r *sceneRunReport) observeStep(state, step string) {
	if r != nil {
		r.LastStepState = state
		r.LastStep = step
	}
}

func writeSceneResult(nodeDir string, report sceneRunReport) error {
	dir := filepath.Join(nodeDir, "_results")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var history *os.File
	if runDir := os.Getenv("KC_VALIDATION_RUN_DIR"); runDir != "" {
		path := filepath.Join(runDir, "scenes", report.Test)
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
		var err error
		history, err = os.CreateTemp(path, report.State+"-*.json")
		if err != nil {
			return err
		}
		defer history.Close()
		report.ExecutionID = strings.TrimSuffix(filepath.Base(history.Name()), ".json")
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if history != nil {
		if _, err := history.Write(append(raw, '\n')); err != nil {
			return err
		}
	}
	tmp := filepath.Join(dir, "latest.json.tmp")
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "latest.json"))
}

func composeSceneNode(t *testing.T, doc sceneCatalogFile, node sceneTreeNode) ([]sceneStep, []sceneAgentTask) {
	t.Helper()
	ids := map[string]sceneCatalogState{}
	for _, state := range doc.States {
		ids[state.ID] = state
	}
	dirs := sceneStateDirs(t, ids)
	chain := append(append([]string{}, node.Ancestors...), node.ID)
	var steps []sceneStep
	var agents []sceneAgentTask
	for _, id := range chain {
		dir := dirs[id]
		featureSteps, featureAgents := loadSceneFeatureSteps(t, filepath.Join(dir, "_build", "construct.feature"), filepath.Join(dir, "_materials"))
		steps = append(steps, featureSteps...)
		agents = append(agents, featureAgents...)
	}
	if shouldRunSceneProbes(doc, node.ID) {
		for _, probe := range node.Probes {
			featureSteps, featureAgents := loadSceneFeatureSteps(t, probe, filepath.Join(node.Dir, "_materials"))
			steps = append(steps, featureSteps...)
			agents = append(agents, featureAgents...)
		}
	}
	return steps, agents
}

func nodeConstructSteps(t *testing.T, node sceneTreeNode) []sceneStep {
	t.Helper()
	steps, _ := loadSceneFeatureSteps(t, node.Construct, filepath.Join(node.Dir, "_materials"))
	return steps
}

func loadSceneFeatureSteps(t *testing.T, path, materials string) ([]sceneStep, []sceneAgentTask) {
	t.Helper()
	parsed, err := parseSceneFeatureFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if len(parsed.scenarios) != 1 {
		t.Fatalf("%s scenarios=%d want 1", path, len(parsed.scenarios))
	}
	scene := parsed.scenarios[0]
	for i := range scene.steps {
		scene.steps[i].fixtureDir = materials
	}
	return scene.steps, scene.agentTasks
}

func TestSceneArgvSplit(t *testing.T) {
	got, err := splitSceneArgs(`kc search --eq "name=Gross merchandise value"`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"search", "--eq", "name=Gross merchandise value"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestSceneExpandScenePath(t *testing.T) {
	got, err := expandScenePath("--dir", "/tmp/home", "/tmp/materials")
	if err != nil || got != "--dir" {
		t.Fatalf("unchanged: %q %v", got, err)
	}
	got, err = expandScenePath("$materials/drafts", "/tmp/home", "/tmp/materials")
	if err != nil || got != "/tmp/materials/drafts" {
		t.Fatalf("materials: %q %v", got, err)
	}
	got, err = expandScenePath("$home/schema.changeset.json", "/tmp/home", "/tmp/materials")
	if err != nil || got != "/tmp/home/schema.changeset.json" {
		t.Fatalf("home: %q %v", got, err)
	}
	if _, err := expandScenePath("$materials/drafts", "/tmp/home", ""); err == nil {
		t.Fatal("empty materials must fail")
	}
}

func TestSceneJSONExpect(t *testing.T) {
	root := map[string]any{
		"catalog": "kr://scene/catalog",
		"repos":   []any{},
		"system":  map[string]any{"repositoryId": "kr://kc/system", "commit": "abc"},
		"repositories": []any{
			map[string]any{"id": "kr://kc/system"},
			"kr://scene/knowledge",
		},
	}
	if err := matchJSONExpect(root, "catalog", "kr://scene/catalog"); err != nil {
		t.Fatal(err)
	}
	if err := matchJSONExpect(root, "repos", "[]"); err != nil {
		t.Fatal(err)
	}
	if err := matchJSONExpect(root, "home", "absent"); err != nil {
		t.Fatal(err)
	}
	if err := matchJSONExpect(root, "system.commit", "nonempty"); err != nil {
		t.Fatal(err)
	}
	if err := matchJSONIncludes(root, "repositories[].id", "kr://kc/system"); err != nil {
		t.Fatal(err)
	}
	if err := matchJSONIncludes(root, "repositories[].id", "nonempty"); err != nil {
		t.Fatal(err)
	}
	if err := matchJSONIncludes(root, "repositories[].missing", "nonempty"); err == nil {
		t.Fatal("missing values must not satisfy an includes nonempty assertion")
	}
	if err := matchJSONIncludes([]any{map[string]any{"commit": ""}}, "[].commit", "nonempty"); err == nil {
		t.Fatal("empty values must not satisfy an includes nonempty assertion")
	}
	if err := matchJSONIncludes(root, "repositories", "kr://scene/knowledge"); err != nil {
		t.Fatal(err)
	}
	if err := matchJSONExpect(root, "repos", "nonempty"); err == nil {
		t.Fatal("empty repos must not count as nonempty")
	}
}

func TestSceneFeaturesPinObservedState(t *testing.T) {
	err := filepath.WalkDir(scenesRoot(), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "_results" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".feature") {
			return nil
		}
		parsed, parseErr := parseSceneFeatureFile(path)
		if parseErr != nil {
			t.Errorf("%s: %v", path, parseErr)
			return nil
		}
		for _, scene := range parsed.scenarios {
			if err := sceneObservationGaps(scene); err != nil {
				t.Errorf("%s: %v", path, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func sceneObservationGaps(scene sceneScenario) error {
	if len(scene.steps) == 1 && scene.steps[0].kind == "http-server" {
		return nil
	}
	sawObservation := false
	for i, step := range scene.steps {
		switch step.kind {
		case "run", "http":
			if !sceneHasObservationAfter(scene.steps[i+1:]) {
				return fmt.Errorf("scenario %q %s %q has no observed Then", scene.name, step.kind, step.text)
			}
			sawObservation = true
		case "material":
			if !sceneEventuallyObserves(scene.steps[i+1:]) {
				return fmt.Errorf("scenario %q material %q is never read back", scene.name, step.material)
			}
			sawObservation = true
		case "output-has", "output-includes", "error", "hit-stripped", "hit-full", "hit-located", "zero-hits", "read-full", "whoami":
			sawObservation = true
		case "succeeds":
			if !sceneHasObservationAfter(scene.steps[i+1:]) {
				return fmt.Errorf("scenario %q uses bare Then the command succeeds", scene.name)
			}
		}
	}
	if !sawObservation {
		return fmt.Errorf("scenario %q never observes a post-state or error", scene.name)
	}
	return nil
}

func sceneEventuallyObserves(steps []sceneStep) bool {
	for i, step := range steps {
		switch step.kind {
		case "output-has", "output-includes", "error", "hit-stripped", "hit-full", "hit-located", "zero-hits", "read-full", "whoami":
			return true
		case "run", "http":
			return sceneHasObservationAfter(steps[i+1:])
		}
	}
	return false
}

func sceneHasObservationAfter(steps []sceneStep) bool {
	for _, step := range steps {
		switch step.kind {
		case "run", "http", "material", "http-server":
			return false
		case "output-has", "output-includes", "error", "hit-stripped", "hit-full", "hit-located", "zero-hits", "read-full", "whoami":
			return true
		}
	}
	return false
}

func newSceneWorldAt(t *testing.T, home string, cache *sceneHomeCache) *sceneWorld {
	t.Helper()
	isolateClientCredentials(t)
	t.Setenv("HOME", t.TempDir())
	// Client profiles are mutable process configuration, not a state-node
	// fixture. Never let login/logout in one probe affect another probe.
	t.Setenv("KC_CONFIG_DIR", t.TempDir())
	if cache != nil && cache.lakefs != nil {
		t.Setenv("KC_LAKEFS_CREDENTIAL", cache.lakefs.Credential())
	}
	world := &sceneWorld{t: t, home: home, ids: map[string]string{}, canonical: map[string]string{}, cache: cache}
	if endpoint := strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL")); endpoint != "" {
		world.projection = newSceneProjectionNamespace(t, endpoint)
		world.bindProjection()
	}
	return world
}

func (w *sceneWorld) bindProjection() {
	w.t.Helper()
	if w.projection == nil {
		return
	}
	if _, err := os.Stat(cli.StoresPath(w.home)); os.IsNotExist(err) {
		return // The deployment fixture has not initialized this home yet.
	} else if err != nil {
		w.t.Fatal(err)
	}
	stores, err := cli.ReadStores(w.home)
	if err != nil {
		w.t.Fatal(err)
	}
	if stores.Index == "opensearch" {
		// A frozen home may name a previous world's already closed proxy.
		// Only this temporary fixture's endpoint changes, never the env source.
		stores.OpenSearch.URL = w.projection.server.URL
		if err := cli.WriteStores(w.home, stores); err != nil {
			w.t.Fatal(err)
		}
	}
}

func (w *sceneWorld) run(step sceneStep) {
	w.t.Helper()
	switch step.kind {
	case "run":
		w.runCommand(step.command, step.fixtureDir)
	case "http":
		w.runHTTP(step)
	case "schema":
		w.publishSchema(step.schema)
	case "instance":
		w.publishInstance(step.instance)
	case "material":
		w.publishMaterial(step)
	case "http-server":
		w.startHTTPServer()
	case "deployment-fixture":
		if _, _, err := cli.InitHome(w.home, "kr://scene/catalog"); err != nil {
			w.t.Fatal(err)
		}
		w.bindProjection()
		if _, err := cli.EnsureSystemRepository(w.home, "kr://scene/catalog"); err != nil {
			w.t.Fatal(err)
		}
		if w.cache != nil {
			w.cache.recordInit()
		}
	case "repository-fixture":
		if w.cache == nil || w.cache.lakefs == nil {
			w.t.Fatal("existing repository fixture requires the scene lakeFS cache")
		}
		ws, err := cli.Open(w.home)
		if err != nil {
			w.t.Fatal(err)
		}
		defer ws.Close()
		if _, exists := ws.Store.Get(kernel.RepositoryID(step.object)); !exists {
			name := w.cache.lakefs.NewRepo()
			if _, err := cli.AddRepository(ws, step.object, "lakefs", w.cache.lakefs.DSN(name), "", ""); err != nil {
				w.t.Fatal(err)
			}
		}
	case "catalog-fixture":
		ws, err := cli.Open(w.home)
		if err != nil {
			w.t.Fatal(err)
		}
		defer ws.Close()
		if _, err := cli.AddCatalog(ws, step.object); err != nil {
			w.t.Fatal(err)
		}
	case "bootstrap-fixture":
		allow, err := cli.ReadAllow(w.home)
		if err != nil {
			w.t.Fatal(err)
		}
		if len(allow.Rules) != 0 {
			w.t.Fatal("bootstrap fixture requires empty grants")
		}
		allow.Rules = []cli.AllowRule{{ID: "bootstrap-deployment-admin", Principal: step.principal, Actions: []string{"*"}}}
		if err := cli.WriteAllow(w.home, allow); err != nil {
			w.t.Fatal(err)
		}
	case "succeeds":
		w.thenSucceeds()
	case "output-has":
		w.thenOutputHas(step)
	case "output-includes":
		w.thenOutputIncludes(step)
	case "error":
		w.thenError(step.errorCode)
	case "hit-stripped":
		w.thenHit(step.object, false)
	case "hit-full":
		w.thenHit(step.object, true)
	case "hit-located":
		w.thenLocated(step.object)
	case "zero-hits":
		w.thenZeroHits()
	case "read-full":
		w.thenReadFull()
	case "whoami":
		w.thenWhoAmI(step.whoami)
	default:
		w.t.Fatalf("line %d: unknown step kind %q", step.line, step.kind)
	}
}

func (w *sceneWorld) runCommand(command, fixtureDir string) {
	w.t.Helper()
	args, err := splitSceneArgs(command)
	if err != nil {
		w.t.Fatal(err)
	}
	for i, arg := range args {
		expanded, expErr := w.expandArg(arg, fixtureDir)
		if expErr != nil {
			w.t.Fatal(expErr)
		}
		args[i] = expanded
	}
	w.lastKind = "cli"
	if sceneClientCredentialCommand(args) || (len(args) > 0 && (args[0] == "deployment" || args[0] == "local")) {
		w.cli = kcClientLocal(args...)
	} else {
		w.cli = kc(w.home, args...)
	}
	w.captureCLI()
}

func sceneClientCredentialCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if args[0] == "login" || args[0] == "logout" {
		return true
	}
	if len(args) >= 2 && args[0] == "dataset" && args[1] == "overlay" {
		return true
	}
	for _, arg := range args {
		if arg == "--server" {
			return true
		}
	}
	return false
}

func (w *sceneWorld) expandArg(arg, materials string) (string, error) {
	expanded, err := expandScenePath(arg, w.home, materials)
	if err != nil {
		return "", err
	}
	if strings.Contains(expanded, "$server") {
		if w.httpServer == nil {
			return "", fmt.Errorf("$server used without Given local HTTP server")
		}
		expanded = strings.ReplaceAll(expanded, "$server", w.httpServer.URL)
	}
	if strings.HasPrefix(expanded, "$last.") {
		field := strings.TrimPrefix(expanded, "$last.")
		got, ok := lookupJSONPath(w.lastPayload, field)
		if !ok {
			return "", fmt.Errorf("$last.%s missing in last CLI output %#v", field, w.lastPayload)
		}
		return fmt.Sprint(got), nil
	}
	for _, key := range []string{"previewId", "proposalId", "reportId", "pinId", "pinFile"} {
		token := "$" + key
		if !strings.Contains(expanded, token) {
			continue
		}
		val := w.ids[key]
		if val == "" {
			return "", fmt.Errorf("%s not captured from a prior CLI result", token)
		}
		expanded = strings.ReplaceAll(expanded, token, val)
	}
	return expanded, nil
}

func (w *sceneWorld) captureCLI() {
	if w.cli.Status != 0 {
		return
	}
	var payload any
	if err := json.Unmarshal([]byte(w.cli.Stdout), &payload); err != nil {
		return
	}
	w.lastPayload = payload
	row, ok := payload.(map[string]any)
	if !ok {
		return
	}
	for _, key := range []string{"previewId", "proposalId", "reportId", "pinId"} {
		if v, exists := row[key]; exists && fmt.Sprint(v) != "" {
			w.ids[key] = fmt.Sprint(v)
		}
	}
	if _, ok := row["pinId"]; ok {
		if out, exists := row["out"]; exists && fmt.Sprint(out) != "" {
			w.ids["pinFile"] = fmt.Sprint(out)
		} else {
			path := filepath.Join(w.home, "scene-pin.json")
			if err := os.WriteFile(path, []byte(w.cli.Stdout), 0o644); err == nil {
				w.ids["pinFile"] = path
			}
		}
	}
}

func expandScenePath(arg, home, materials string) (string, error) {
	if strings.Contains(arg, "$materials") {
		if strings.TrimSpace(materials) == "" {
			return "", fmt.Errorf("$materials used without a node _materials directory")
		}
		arg = strings.ReplaceAll(arg, "$materials", materials)
	}
	if strings.Contains(arg, "$home") {
		arg = strings.ReplaceAll(arg, "$home", home)
	}
	return arg, nil
}

func (w *sceneWorld) thenSucceeds() {
	w.t.Helper()
	if w.lastKind != "cli" {
		w.t.Fatal("Then the command succeeds requires a prior When I run")
	}
	body(w.t, w.cli)
}

func (w *sceneWorld) thenOutputHas(step sceneStep) {
	w.t.Helper()
	payload := w.observedCLI(step.line)
	for _, row := range step.table {
		if len(row) != 2 {
			w.t.Fatalf("line %d: output has row want 2 cells, got %#v", step.line, row)
		}
		if err := matchJSONExpect(payload, row[0], w.expandExpected(row[1])); err != nil {
			w.t.Fatalf("line %d: %v in %#v", step.line, err, payload)
		}
	}
}

func (w *sceneWorld) thenOutputIncludes(step sceneStep) {
	w.t.Helper()
	payload := w.observedCLI(step.line)
	for _, row := range step.table {
		if len(row) != 2 {
			w.t.Fatalf("line %d: output includes row want 2 cells, got %#v", step.line, row)
		}
		if err := matchJSONIncludes(payload, row[0], w.expandExpected(row[1])); err != nil {
			w.t.Fatalf("line %d: %v in %#v", step.line, err, payload)
		}
	}
}

// expandExpected resolves the same $home token the run step resolves in argv,
// so a Then table can pin a returned path against this trip's home.
func (w *sceneWorld) expandExpected(value string) string {
	value = strings.ReplaceAll(value, "$home", w.home)
	for _, key := range []string{"previewId", "proposalId", "reportId", "pinId", "pinFile"} {
		token := "$" + key
		if !strings.Contains(value, token) {
			continue
		}
		if val := w.ids[key]; val != "" {
			value = strings.ReplaceAll(value, token, val)
		}
	}
	return value
}

func (w *sceneWorld) observedCLI(line int) any {
	w.t.Helper()
	if w.lastKind != "cli" {
		w.t.Fatalf("line %d: output assertion requires a prior When I run", line)
	}
	return body(w.t, w.cli)
}

func matchJSONExpect(root any, path, want string) error {
	got, ok := lookupJSONPath(root, path)
	if want == "absent" {
		if ok {
			return fmt.Errorf("%s: want absent, got %#v", path, got)
		}
		return nil
	}
	if !ok {
		return fmt.Errorf("%s: missing", path)
	}
	if want == "nonempty" {
		if !jsonNonempty(got) {
			return fmt.Errorf("%s: want nonempty, got %#v", path, got)
		}
		return nil
	}
	if want == "[]" {
		arr, isArr := got.([]any)
		if !isArr || len(arr) != 0 {
			return fmt.Errorf("%s: want [], got %#v", path, got)
		}
		return nil
	}
	if want == "{}" {
		obj, isObj := got.(map[string]any)
		if !isObj || len(obj) != 0 {
			return fmt.Errorf("%s: want {}, got %#v", path, got)
		}
		return nil
	}
	if fmt.Sprint(got) != want {
		return fmt.Errorf("%s: got %#v want %q", path, got, want)
	}
	return nil
}

func matchJSONIncludes(root any, path, want string) error {
	parentPath, field, ok := strings.Cut(path, "[]")
	if !ok {
		got, exists := lookupJSONPath(root, path)
		if !exists {
			return fmt.Errorf("%s: missing", path)
		}
		arr, isArr := got.([]any)
		if !isArr {
			return fmt.Errorf("%s: want array, got %#v", path, got)
		}
		for _, item := range arr {
			if jsonIncludesValue(item, "", want) {
				return nil
			}
		}
		return fmt.Errorf("%s: %#v does not include %q", path, got, want)
	}
	got, exists := lookupJSONPath(root, parentPath)
	if !exists {
		return fmt.Errorf("%s: missing", parentPath)
	}
	arr, isArr := got.([]any)
	if !isArr {
		return fmt.Errorf("%s: want array, got %#v", parentPath, got)
	}
	field = strings.TrimPrefix(field, ".")
	for _, item := range arr {
		if jsonIncludesValue(item, field, want) {
			return nil
		}
	}
	return fmt.Errorf("%s: %#v does not include %q", path, got, want)
}

func jsonIncludesValue(item any, field, want string) bool {
	if field == "" {
		switch typed := item.(type) {
		case map[string]any:
			for _, key := range []string{"id", "setId", "objectId", "object", "principal"} {
				if fmt.Sprint(typed[key]) == want {
					return true
				}
			}
			return false
		default:
			return fmt.Sprint(item) == want
		}
	}
	got, ok := lookupJSONPath(item, field)
	if want == "nonempty" {
		return ok && jsonNonempty(got)
	}
	return ok && fmt.Sprint(got) == want
}

func lookupJSONPath(root any, path string) (any, bool) {
	if strings.TrimSpace(path) == "" {
		return root, true
	}
	cur := root
	for _, part := range strings.Split(path, ".") {
		switch typed := cur.(type) {
		case map[string]any:
			next, ok := typed[part]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(typed) {
				return nil, false
			}
			cur = typed[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

func jsonNonempty(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return typed != ""
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	case bool:
		return typed
	case float64:
		return typed != 0
	default:
		s := fmt.Sprint(typed)
		return s != "" && s != "0" && s != "<nil>"
	}
}

func (w *sceneWorld) thenError(code string) {
	w.t.Helper()
	switch w.lastKind {
	case "cli":
		expectCode(w.t, w.cli, code)
	case "http":
		wantStatus := http.StatusForbidden
		if code == "UNAUTHENTICATED" {
			wantStatus = http.StatusUnauthorized
		}
		if w.httpCode != wantStatus {
			w.t.Fatalf("status=%d want %d payload=%#v", w.httpCode, wantStatus, w.httpBody)
		}
		errObj, _ := w.httpBody.(map[string]any)["error"].(map[string]any)
		if fmt.Sprint(errObj["code"]) != code {
			w.t.Fatalf("error=%#v want %s", w.httpBody, code)
		}
	default:
		w.t.Fatal("Then error requires a prior When")
	}
}

func (w *sceneWorld) thenLocated(objectID string) {
	w.t.Helper()
	switch w.lastKind {
	case "cli":
		requireMetricSearchHit(w.t, w.cli, objectID)
	case "http":
		requireHTTPSearchHit(w.t, w.httpCode, w.httpBody, objectID)
	default:
		w.t.Fatal("Then 1 hit requires a prior search")
	}
}

func (w *sceneWorld) thenHit(objectID string, full bool) {
	w.t.Helper()
	switch w.lastKind {
	case "cli":
		requireMetricSearchHit(w.t, w.cli, objectID)
		return
	case "http":
		knowledge := requireHTTPSearchHit(w.t, w.httpCode, w.httpBody, objectID)
		value := asMap(w.t, knowledge)["value"]
		if !full {
			if value != nil {
				w.t.Fatalf("missing knowledge.read must strip Canonical: repository=%v knowledgeRef=%v value=%#v", knowledge["repository"], knowledge["knowledgeRef"], value)
			}
			return
		}
		assertPublishedCanonical(w.t, w.canonical, value)
	default:
		w.t.Fatal("Then 1 hit requires a prior search")
	}
}

func (w *sceneWorld) thenZeroHits() {
	w.t.Helper()
	var hits []any
	switch w.lastKind {
	case "cli":
		hits = searchHits(w.t, body(w.t, w.cli))
	case "http":
		if w.httpCode != http.StatusOK {
			w.t.Fatalf("status=%d payload=%#v", w.httpCode, w.httpBody)
		}
		hits = searchHits(w.t, w.httpBody)
	default:
		w.t.Fatal("Then 0 hits requires a prior search")
	}
	if len(hits) != 0 {
		w.t.Fatalf("want zero hits: %#v", hits)
	}
}

func (w *sceneWorld) thenReadFull() {
	w.t.Helper()
	var value any
	switch w.lastKind {
	case "cli":
		value = asMap(w.t, body(w.t, w.cli))["value"]
	case "http":
		if w.httpCode != http.StatusOK {
			w.t.Fatalf("READ status=%d payload=%#v", w.httpCode, w.httpBody)
		}
		value = readPayloadValue(w.t, w.httpBody)
	default:
		w.t.Fatal("Then READ body requires a prior read")
	}
	assertPublishedCanonical(w.t, w.canonical, value)
}

func (w *sceneWorld) thenWhoAmI(want string) {
	w.t.Helper()
	if w.lastKind != "http" {
		w.t.Fatal("Then whoami requires a prior HTTP GET")
	}
	if w.httpCode != http.StatusOK {
		w.t.Fatalf("whoami status=%d payload=%#v", w.httpCode, w.httpBody)
	}
	got := asMap(w.t, w.httpBody)
	if got["principal"] != want {
		w.t.Fatalf("whoami principal=%#v want %s", got, want)
	}
	if onBehalf, _ := got["onBehalfOf"].(string); onBehalf != "" {
		w.t.Fatalf("local pairing must not inject onBehalfOf: %#v", got)
	}
}

func (w *sceneWorld) runHTTP(step sceneStep) {
	w.t.Helper()
	if w.httpServer == nil {
		w.t.Fatal("Given local HTTP server first")
	}
	var payload any
	if len(step.table) > 0 {
		payload = httpTablePayload(w.t, step.table)
	}
	w.lastKind = "http"
	w.httpCode, w.httpBody, _ = identityHTTP(w.t, w.httpServer, step.method, step.path, step.principal, step.onBehalfOf, payload)
}

func (w *sceneWorld) startHTTPServer() {
	w.t.Helper()
	w.stopHTTPServer()
	handler := cli.HTTPHandler(w.home)
	w.httpServer = httptest.NewServer(handler)
	server := w.httpServer
	var once sync.Once
	w.closeHTTP = func() {
		once.Do(func() {
			server.Close()
			if closer, ok := handler.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
		})
	}
	w.t.Cleanup(w.closeHTTP)
}

func (w *sceneWorld) close() {
	w.stopHTTPServer()
	if w.projection != nil {
		w.projection.close()
		w.projection = nil
	}
}

func (w *sceneWorld) stopHTTPServer() {
	if w.closeHTTP != nil {
		w.closeHTTP()
		w.closeHTTP = nil
		w.httpServer = nil
	}
}

func (w *sceneWorld) nextCommandID(prefix string) string {
	w.seq++
	return fmt.Sprintf("%s-%d", prefix, w.seq)
}

func (w *sceneWorld) publishSchema(spec *sceneSchemaSpec) {
	w.t.Helper()
	fields := map[string]any{}
	for _, field := range spec.fields {
		entry := map[string]any{"type": field.typ}
		if field.required {
			entry["required"] = true
		}
		if len(field.access) > 0 {
			access := make([]any, len(field.access))
			for i, token := range field.access {
				access[i] = token
			}
			entry["access"] = access
		}
		fields[field.name] = entry
	}
	value, err := json.Marshal(map[string]any{
		"entity":  spec.entity,
		"aspect":  spec.aspect,
		"pattern": spec.pattern,
		"fields":  fields,
	})
	if err != nil {
		w.t.Fatal(err)
	}
	body(w.t, kc(w.home, "writer", "put",
		"--command-id", w.nextCommandID("schema"),
		"--repo", spec.repo,
		"--object", spec.object,
		"--value", string(value)))
}

func (w *sceneWorld) publishInstance(spec *sceneInstanceSpec) {
	w.t.Helper()
	value, err := json.Marshal(spec.fields)
	if err != nil {
		w.t.Fatal(err)
	}
	w.canonical = spec.fields
	body(w.t, kc(w.home, "writer", "put",
		"--command-id", w.nextCommandID("instance"),
		"--repo", spec.repo,
		"--object", spec.object,
		"--aspect", spec.aspect,
		"--schema-ref", spec.schemaRef,
		"--value", string(value)))
}

func (w *sceneWorld) publishMaterial(step sceneStep) {
	w.t.Helper()
	if strings.TrimSpace(step.material) == "" || strings.TrimSpace(step.fixtureDir) == "" {
		w.t.Fatal("Given material needs an id and the constructing state's directory")
	}
	raw, err := os.ReadFile(filepath.Join(step.fixtureDir, step.material+".yaml"))
	if err != nil {
		w.t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		w.t.Fatal(err)
	}
	kind := yamlString(doc, "kind")
	switch kind {
	case "schema":
		var fields []sceneSchemaField
		fieldMap, _ := doc["fields"].(map[string]any)
		for name, spec := range fieldMap {
			entry, _ := spec.(map[string]any)
			field := sceneSchemaField{name: name, typ: yamlString(entry, "type")}
			if req, ok := entry["required"].(bool); ok {
				field.required = req
			}
			if access, ok := entry["access"].([]any); ok {
				for _, token := range access {
					field.access = append(field.access, fmt.Sprint(token))
				}
			}
			fields = append(fields, field)
		}
		w.publishSchema(&sceneSchemaSpec{
			object: yamlString(doc, "object"), repo: yamlString(doc, "repo"),
			entity: yamlString(doc, "entity"), aspect: yamlString(doc, "aspect"),
			pattern: yamlString(doc, "pattern"), fields: fields,
		})
	case "instance":
		fields := map[string]string{}
		fieldMap, _ := doc["fields"].(map[string]any)
		for name, value := range fieldMap {
			fields[name] = fmt.Sprint(value)
		}
		w.publishInstance(&sceneInstanceSpec{
			object: yamlString(doc, "object"), repo: yamlString(doc, "repo"),
			aspect: yamlString(doc, "aspect"), schemaRef: yamlString(doc, "schema_ref"),
			fields: fields,
		})
	case "put":
		value, err := json.Marshal(doc["value"])
		if err != nil {
			w.t.Fatal(err)
		}
		args := []string{"writer", "put", "--command-id", w.nextCommandID("put"),
			"--repo", yamlString(doc, "repo"), "--object", yamlString(doc, "object"),
			"--value", string(value)}
		if aspect := yamlString(doc, "aspect"); aspect != "" {
			args = append(args, "--aspect", aspect)
		}
		if member := yamlString(doc, "member"); member != "" {
			args = append(args, "--member", member)
		}
		body(w.t, kc(w.home, args...))
	default:
		w.t.Fatalf("material %s kind %q is not a Writer step", step.material, kind)
	}
}

func yamlString(doc map[string]any, key string) string {
	value, ok := doc[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func parseSceneFeatureFile(path string) (sceneFeature, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return sceneFeature{}, err
	}
	return parseSceneFeature(string(raw))
}

func parseSceneFeature(src string) (sceneFeature, error) {
	var doc sceneFeature
	var current *sceneScenario
	var pendingTags []string
	inBrief := false
	var brief strings.Builder
	lines := strings.Split(src, "\n")
	for i := 0; i < len(lines); i++ {
		lineNo := i + 1
		raw := strings.TrimRight(lines[i], "\r")
		trimmed := strings.TrimSpace(raw)
		if inBrief {
			if trimmed == `"""` {
				if err := applySceneBrief(current, strings.TrimSpace(brief.String())); err != nil {
					return doc, fmt.Errorf("line %d: %w", lineNo, err)
				}
				brief.Reset()
				inBrief = false
				continue
			}
			if brief.Len() > 0 {
				brief.WriteByte('\n')
			}
			brief.WriteString(strings.TrimSpace(raw))
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "Feature:") {
			continue
		}
		if strings.HasPrefix(trimmed, "@") {
			pendingTags = append(pendingTags, strings.Fields(trimmed)...)
			continue
		}
		if trimmed == `"""` {
			if current == nil {
				return doc, fmt.Errorf("line %d: brief before Scenario", lineNo)
			}
			inBrief = true
			continue
		}
		if strings.HasPrefix(trimmed, "Scenario:") {
			scene := sceneScenario{
				name: strings.TrimSpace(strings.TrimPrefix(trimmed, "Scenario:")),
				tags: append([]string{}, pendingTags...),
			}
			pendingTags = nil
			doc.scenarios = append(doc.scenarios, scene)
			current = &doc.scenarios[len(doc.scenarios)-1]
			continue
		}
		if current == nil {
			return doc, fmt.Errorf("line %d: step before Scenario", lineNo)
		}
		if strings.HasPrefix(trimmed, "|") {
			return doc, fmt.Errorf("line %d: table without a step", lineNo)
		}
		keyword, rest, ok := splitSceneKeyword(trimmed)
		if !ok {
			return doc, fmt.Errorf("line %d: not a step: %q", lineNo, trimmed)
		}
		table := [][]string{}
		for i+1 < len(lines) {
			next := strings.TrimSpace(strings.TrimRight(lines[i+1], "\r"))
			if !strings.HasPrefix(next, "|") {
				break
			}
			i++
			row, err := parseSceneTableRow(next)
			if err != nil {
				return doc, fmt.Errorf("line %d: %w", i+1, err)
			}
			table = append(table, row)
		}
		step, err := classifySceneStep(lineNo, keyword, rest, table)
		if err != nil {
			return doc, fmt.Errorf("line %d: %w", lineNo, err)
		}
		current.steps = append(current.steps, step)
	}
	if inBrief {
		return doc, fmt.Errorf("unclosed brief")
	}
	return doc, nil
}

var reAgentHeader = regexp.MustCompile(`^Agent as (\S+) \(([^)]+)\)$`)

func applySceneBrief(scene *sceneScenario, text string) error {
	if scene == nil {
		return fmt.Errorf("brief without Scenario")
	}
	first, rest, _ := strings.Cut(text, "\n")
	first = strings.TrimSpace(first)
	if match := reAgentHeader.FindStringSubmatch(first); match != nil {
		fixture := match[2]
		if fixture != "search-only" && fixture != "search+read" {
			return fmt.Errorf("unknown agent fixture %q", fixture)
		}
		scene.agentTasks = append(scene.agentTasks, sceneAgentTask{
			principal: match[1],
			fixture:   fixture,
			brief:     strings.TrimSpace(rest),
		})
		return nil
	}
	if strings.HasPrefix(first, "Agent as ") {
		return fmt.Errorf("agent brief must be `Agent as <principal> (<fixture>)`")
	}
	if scene.brief != "" {
		return fmt.Errorf("duplicate human brief; later blocks must start with Agent as")
	}
	scene.brief = text
	return nil
}

func splitSceneKeyword(line string) (string, string, bool) {
	for _, keyword := range []string{"Given ", "When ", "Then ", "And "} {
		if strings.HasPrefix(line, keyword) {
			return strings.TrimSpace(keyword), strings.TrimSpace(line[len(keyword):]), true
		}
	}
	return "", "", false
}

func classifySceneStep(line int, keyword, rest string, table [][]string) (sceneStep, error) {
	step := sceneStep{line: line, text: rest, table: table}
	switch {
	case strings.HasPrefix(rest, "I run `"):
		if !strings.HasSuffix(rest, "`") {
			return step, fmt.Errorf("unclosed command: %q", rest)
		}
		step.kind = "run"
		step.command = strings.TrimSuffix(strings.TrimPrefix(rest, "I run `"), "`")
		if len(table) > 0 {
			return step, fmt.Errorf("When I run does not take a table")
		}
	case strings.HasPrefix(rest, "HTTP "):
		httpLine := rest
		if strings.Contains(httpLine, " as ") && strings.HasSuffix(httpLine, ":") {
			httpLine = strings.TrimSuffix(httpLine, ":")
		}
		match := reHTTP.FindStringSubmatch(httpLine)
		if match == nil {
			return step, fmt.Errorf("bad HTTP step: %q", rest)
		}
		step.kind = "http"
		step.method = match[1]
		step.path = match[2]
		step.principal = match[3]
		step.onBehalfOf = match[4]
	case reSchema.MatchString(rest):
		match := reSchema.FindStringSubmatch(rest)
		fields, err := parseSchemaTable(table)
		if err != nil {
			return step, err
		}
		step.kind = "schema"
		step.schema = &sceneSchemaSpec{
			object: match[1], repo: match[2], entity: match[3], aspect: match[4], pattern: match[5], fields: fields,
		}
	case reInst.MatchString(rest):
		match := reInst.FindStringSubmatch(rest)
		fields, err := parseInstanceTable(table)
		if err != nil {
			return step, err
		}
		step.kind = "instance"
		step.instance = &sceneInstanceSpec{
			object: match[1], repo: match[2], aspect: match[3], schemaRef: match[4], fields: fields,
		}
	case rest == "local HTTP server":
		step.kind = "http-server"
	case rest == "deployment fixture":
		step.kind = "deployment-fixture"
	case strings.HasPrefix(rest, "existing repository "):
		step.kind = "repository-fixture"
		step.object = strings.TrimSpace(strings.TrimPrefix(rest, "existing repository "))
	case strings.HasPrefix(rest, "configured catalog "):
		step.kind = "catalog-fixture"
		step.object = strings.TrimSpace(strings.TrimPrefix(rest, "configured catalog "))
	case strings.HasPrefix(rest, "bootstrap principal "):
		step.kind = "bootstrap-fixture"
		step.principal = strings.TrimSpace(strings.TrimPrefix(rest, "bootstrap principal "))
	case strings.HasPrefix(rest, "material "):
		step.kind = "material"
		step.material = strings.TrimSpace(strings.TrimPrefix(rest, "material "))
		if step.material == "" {
			return step, fmt.Errorf("material id missing")
		}
		if len(table) > 0 {
			return step, fmt.Errorf("Given material does not take a table")
		}
	case rest == "the command succeeds":
		step.kind = "succeeds"
		if len(table) > 0 {
			return step, fmt.Errorf("Then the command succeeds does not take a table")
		}
	case rest == "the output has:" || rest == "the output has":
		if len(table) == 0 {
			return step, fmt.Errorf("Then the output has needs a table")
		}
		step.kind = "output-has"
	case rest == "the output includes:" || rest == "the output includes":
		if len(table) == 0 {
			return step, fmt.Errorf("Then the output includes needs a table")
		}
		step.kind = "output-includes"
	case strings.HasPrefix(rest, "error "):
		step.kind = "error"
		step.errorCode = strings.TrimSpace(strings.TrimPrefix(rest, "error "))
	case rest == "0 hits":
		step.kind = "zero-hits"
	case rest == "READ body is full canonical":
		step.kind = "read-full"
	case strings.HasPrefix(rest, "whoami is "):
		step.kind = "whoami"
		step.whoami = strings.TrimSpace(strings.TrimPrefix(rest, "whoami is "))
	case reHit.MatchString(rest):
		match := reHit.FindStringSubmatch(rest)
		step.object = match[1]
		if match[2] == "full canonical" {
			step.kind = "hit-full"
		} else {
			step.kind = "hit-stripped"
		}
	case strings.HasPrefix(rest, "1 hit "):
		step.kind = "hit-located"
		step.object = strings.TrimSpace(strings.TrimPrefix(rest, "1 hit "))
		if strings.Contains(step.object, " ") {
			return step, fmt.Errorf("ambiguous hit step: %q", rest)
		}
	default:
		return step, fmt.Errorf("unclassified %s %q", keyword, rest)
	}
	return step, nil
}

func parseSceneTableRow(line string) ([]string, error) {
	trim := strings.TrimSpace(line)
	if !strings.HasPrefix(trim, "|") || !strings.HasSuffix(trim, "|") {
		return nil, fmt.Errorf("table row must start and end with |: %q", line)
	}
	parts := strings.Split(trim[1:len(trim)-1], "|")
	row := make([]string, len(parts))
	for i, part := range parts {
		row[i] = strings.TrimSpace(part)
	}
	return row, nil
}

func parseSchemaTable(table [][]string) ([]sceneSchemaField, error) {
	if len(table) < 2 {
		return nil, fmt.Errorf("Schema table needs a header and fields")
	}
	header := table[0]
	idx := map[string]int{}
	for i, name := range header {
		idx[name] = i
	}
	for _, col := range []string{"field", "type", "required", "access"} {
		if _, ok := idx[col]; !ok {
			return nil, fmt.Errorf("Schema table missing column %s", col)
		}
	}
	var fields []sceneSchemaField
	for _, row := range table[1:] {
		if len(row) != len(header) {
			return nil, fmt.Errorf("Schema row width %d want %d", len(row), len(header))
		}
		required := row[idx["required"]]
		field := sceneSchemaField{
			name:     row[idx["field"]],
			typ:      row[idx["type"]],
			required: required == "yes" || required == "true",
		}
		if access := row[idx["access"]]; access != "" {
			for _, token := range strings.Split(access, ",") {
				token = strings.TrimSpace(token)
				if token != "" {
					field.access = append(field.access, token)
				}
			}
		}
		fields = append(fields, field)
	}
	return fields, nil
}

func parseInstanceTable(table [][]string) (map[string]string, error) {
	if len(table) == 0 {
		return nil, fmt.Errorf("instance table is empty")
	}
	fields := map[string]string{}
	for _, row := range table {
		if len(row) != 2 {
			return nil, fmt.Errorf("instance row want 2 cells, got %#v", row)
		}
		fields[row[0]] = row[1]
	}
	return fields, nil
}

func httpTablePayload(t *testing.T, table [][]string) map[string]any {
	t.Helper()
	payload := map[string]any{}
	for _, row := range table {
		if len(row) != 2 {
			t.Fatalf("HTTP table row want 2 cells, got %#v", row)
		}
		if row[0] == "equal" {
			payload[row[0]] = []string{row[1]}
			continue
		}
		payload[row[0]] = row[1]
	}
	return payload
}

func splitSceneArgs(command string) ([]string, error) {
	var out []string
	var buf strings.Builder
	quote := rune(0)
	escape := false
	for _, r := range command {
		switch {
		case escape:
			buf.WriteRune(r)
			escape = false
		case r == '\\' && quote != '\'':
			escape = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				buf.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case unicode.IsSpace(r):
			if buf.Len() > 0 {
				out = append(out, buf.String())
				buf.Reset()
			}
		default:
			buf.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unclosed quote in %q", command)
	}
	if buf.Len() > 0 {
		out = append(out, buf.String())
	}
	if len(out) > 0 && out[0] == "kc" {
		out = out[1:]
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return out, nil
}

func requireHTTPSearchHit(t *testing.T, status int, payload any, objectID string) map[string]any {
	t.Helper()
	if status != http.StatusOK {
		t.Fatalf("SEARCH status=%d payload=%#v", status, payload)
	}
	hits := searchHits(t, payload)
	if len(hits) != 1 {
		t.Fatalf("want one hit for %s: %#v", objectID, hits)
	}
	knowledge := asMap(t, asMap(t, hits[0])["knowledge"])
	if asMap(t, knowledge["knowledgeRef"])["object"] != objectID {
		t.Fatalf("hit object = %#v, want %s", knowledge["knowledgeRef"], objectID)
	}
	return knowledge
}

func searchHits(t *testing.T, result any) []any {
	t.Helper()
	hits, _ := asMap(t, result)["hits"].([]any)
	return hits
}

func requireMetricSearchHit(t *testing.T, result kcRunResult, objectID string) map[string]any {
	t.Helper()
	hits := searchHits(t, body(t, result))
	if len(hits) != 1 {
		t.Fatalf("want one hit for %s: %#v", objectID, hits)
	}
	hit := asMap(t, hits[0])
	if fmt.Sprint(hit["objectId"]) != objectID {
		t.Fatalf("hit object = %#v, want %s", hit, objectID)
	}
	return hit
}

func metricDefinition(t *testing.T, value any) map[string]any {
	t.Helper()
	fields := asMap(t, value)
	if nested, ok := fields["definition"]; ok {
		return asMap(t, nested)
	}
	return fields
}

func assertPublishedCanonical(t *testing.T, want map[string]string, value any) {
	t.Helper()
	fields := metricDefinition(t, value)
	for key, expected := range want {
		if fmt.Sprint(fields[key]) != expected {
			t.Fatalf("authorized Canonical field %s=%#v want %q in %#v", key, fields[key], expected, value)
		}
	}
}

func identityHTTP(t *testing.T, server *httptest.Server, method, path, principal, onBehalfOf string, payload any) (int, any, string) {
	t.Helper()
	var bodyReader *strings.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		bodyReader = strings.NewReader(string(encoded))
	}
	var request *http.Request
	var err error
	if bodyReader != nil {
		request, err = http.NewRequest(method, server.URL+path, bodyReader)
	} else {
		request, err = http.NewRequest(method, server.URL+path, http.NoBody)
	}
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if principal != "" {
		request.Header.Set("X-Kc-As", principal)
	}
	if onBehalfOf != "" {
		request.Header.Set("X-Kc-On-Behalf-Of", onBehalfOf)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("%s %s returned invalid JSON: %v: %s", method, path, err, raw)
	}
	return response.StatusCode, decoded, string(raw)
}

func readPayloadValue(t *testing.T, payload any) any {
	t.Helper()
	switch typed := payload.(type) {
	case []any:
		if len(typed) != 1 {
			t.Fatalf("READ envelope: %#v", payload)
		}
		return asMap(t, typed[0])["value"]
	case map[string]any:
		return typed["value"]
	default:
		t.Fatalf("READ envelope: %#v", payload)
		return nil
	}
}
