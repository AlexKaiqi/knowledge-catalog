package cli_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"

	"kc/cli"
	"kc/internal/testkit"
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
		{principal: "bot", fixture: "search-only"},
		{principal: "bot", fixture: "search+read"},
	}
	if len(tasks) != len(want) {
		t.Fatalf("agent tasks=%d want %d: %#v", len(tasks), len(want), tasks)
	}
	for i, task := range want {
		if tasks[i].principal != task.principal || tasks[i].fixture != task.fixture || strings.TrimSpace(tasks[i].brief) == "" {
			t.Fatalf("agent task %d = %#v want %#v with brief", i, tasks[i], task)
		}
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
	if _, err := os.Stat(filepath.Join(root, spec, "catalog.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestMetricPermissionScenes(t *testing.T) {
	doc := loadSceneCatalog(t)
	n := 0
	for _, node := range discoverConstructableNodes(t) {
		if !nodeNeedsIndex(node) {
			continue
		}
		n++
		node := node
		t.Run(node.ID, func(t *testing.T) {
			runSceneNode(t, doc, node)
		})
	}
	if n == 0 {
		t.Fatal("no constructable search nodes")
	}
}

func TestProductScenes(t *testing.T) {
	doc := loadSceneCatalog(t)
	n := 0
	for _, node := range discoverConstructableNodes(t) {
		if nodeNeedsIndex(node) || nodeNeedsState(node) {
			continue
		}
		n++
		node := node
		t.Run(node.ID, func(t *testing.T) {
			runSceneNode(t, doc, node)
		})
	}
	if n == 0 {
		t.Fatal("no constructable product nodes")
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
	runSceneNode(t, doc, node)
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
}

func runSceneNode(t *testing.T, doc sceneCatalogFile, node sceneTreeNode) {
	t.Helper()
	started := time.Now()
	report := sceneRunReport{
		State:     node.ID,
		Ancestors: append([]string{}, node.Ancestors...),
		Probes:    []string{},
	}
	if shouldRunSceneProbes(doc, node.ID) {
		for _, probe := range node.Probes {
			report.Probes = append(report.Probes, filepath.Base(probe))
		}
	}
	defer func() {
		report.ElapsedMS = time.Since(started).Milliseconds()
		if err := writeSceneResult(node.Dir, report); err != nil {
			t.Errorf("write _results: %v", err)
		}
	}()
	steps, _ := composeSceneNode(t, doc, node)
	world := newSceneWorld(t)
	for _, step := range steps {
		report.LastStep = step.text
		world.run(step)
	}
	report.OK = true
}

type sceneRunReport struct {
	State     string   `json:"state"`
	OK        bool     `json:"ok"`
	ElapsedMS int64    `json:"elapsed_ms"`
	Ancestors []string `json:"ancestors"`
	Probes    []string `json:"probes"`
	LastStep  string   `json:"last_step,omitempty"`
}

func writeSceneResult(nodeDir string, report sceneRunReport) error {
	dir := filepath.Join(nodeDir, "_results")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
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
	appendFeature := func(path, materials string) {
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
		steps = append(steps, scene.steps...)
		agents = append(agents, scene.agentTasks...)
	}
	for _, id := range chain {
		dir := dirs[id]
		appendFeature(filepath.Join(dir, "_build", "construct.feature"), filepath.Join(dir, "_materials"))
	}
	if shouldRunSceneProbes(doc, node.ID) {
		for _, probe := range node.Probes {
			appendFeature(probe, filepath.Join(node.Dir, "_materials"))
		}
	}
	return steps, agents
}

func TestSceneArgvSplit(t *testing.T) {
	got, err := splitSceneArgs(`kc knowledge search --eq "name=Gross merchandise value"`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"knowledge", "search", "--eq", "name=Gross merchandise value"}
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

func newSceneWorld(t *testing.T) *sceneWorld {
	t.Helper()
	isolateClientCredentials(t)
	t.Setenv("HOME", t.TempDir())
	return &sceneWorld{t: t, home: testkit.TempDir(t), ids: map[string]string{}, canonical: map[string]string{}}
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
	if sceneClientCredentialCommand(args) {
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
	if args[0] != "whoami" {
		return false
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
		path := filepath.Join(w.home, "scene-pin.json")
		if err := os.WriteFile(path, []byte(w.cli.Stdout), 0o644); err == nil {
			w.ids["pinFile"] = path
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
		if err := matchJSONExpect(payload, row[0], row[1]); err != nil {
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
		if err := matchJSONIncludes(payload, row[0], row[1]); err != nil {
			w.t.Fatalf("line %d: %v in %#v", step.line, err, payload)
		}
	}
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
			for _, key := range []string{"id", "workspaceId", "objectId", "object", "principal"} {
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
	var knowledge map[string]any
	switch w.lastKind {
	case "cli":
		knowledge = requireMetricSearchHit(w.t, w.cli, objectID)
	case "http":
		knowledge = requireHTTPSearchHit(w.t, w.httpCode, w.httpBody, objectID)
	default:
		w.t.Fatal("Then 1 hit requires a prior search")
	}
	value := asMap(w.t, knowledge)["value"]
	if !full {
		if value != nil {
			w.t.Fatalf("missing knowledge.read must strip Canonical: repository=%v knowledgeRef=%v value=%#v", knowledge["repository"], knowledge["knowledgeRef"], value)
		}
		return
	}
	assertPublishedCanonical(w.t, w.canonical, value)
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
	handler := cli.HTTPHandler(w.home)
	if closer, ok := handler.(interface{ Close() error }); ok {
		w.t.Cleanup(func() { _ = closer.Close() })
	}
	w.httpServer = httptest.NewServer(handler)
	w.t.Cleanup(w.httpServer.Close)
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
	knowledge := asMap(t, asMap(t, hits[0])["knowledge"])
	if asMap(t, knowledge["knowledgeRef"])["object"] != objectID {
		t.Fatalf("hit object = %#v, want %s", knowledge["knowledgeRef"], objectID)
	}
	return knowledge
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
