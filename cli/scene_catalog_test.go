package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"kc/cli"
)

type sceneCatalogFile struct {
	States  []sceneCatalogState  `json:"states"`
	Bundles []sceneCatalogBundle `json:"bundles"`
}

type sceneCatalogState struct {
	ID          string                `json:"id"`
	Layer       string                `json:"layer"`
	Role        string                `json:"role"`
	Surface     string                `json:"surface"`
	Source      string                `json:"source"`
	DependsOn   []string              `json:"depends_on"`
	Publishes   []string              `json:"publishes"`
	AlsoFreezes []string              `json:"also_freezes"`
	Construct   string                `json:"construct"`
	Processes   []sceneCatalogProcess `json:"processes"`
}

type sceneCatalogProcess struct {
	File     string   `json:"file"`
	Source   string   `json:"source"`
	Surface  string   `json:"surface"`
	Evidence []string `json:"evidence"`
}

type sceneCatalogBundle struct {
	ID         string             `json:"id"`
	Suite      string             `json:"suite"`
	Summary    string             `json:"summary"`
	EntryState string             `json:"entry_state"`
	Walk       []sceneCatalogWalk `json:"walk"`
}

type sceneCatalogWalk struct {
	Construct string `json:"construct"`
	Process   string `json:"process"`
}

func TestSceneBundlesDeclareExistingEntryState(t *testing.T) {
	doc := loadSceneCatalog(t)
	states := map[string]sceneCatalogState{}
	for _, state := range doc.States {
		states[state.ID] = state
	}
	localEntries := 0
	for _, bundle := range doc.Bundles {
		if _, ok := states[bundle.EntryState]; !ok {
			t.Errorf("bundle %s must name an existing entry_state, got %q", bundle.ID, bundle.EntryState)
		}
		if bundle.EntryState != "" && bundle.EntryState != "catalog-initialized" {
			localEntries++
		}
	}
	if localEntries == 0 {
		t.Error("user tasks must include journeys starting from an already usable state")
	}
}

func TestSceneJourneysRetireLocalAndSeparateRegister(t *testing.T) {
	for _, node := range discoverConstructableNodes(t) {
		paths := append([]string{node.Construct}, node.Probes...)
		for _, path := range paths {
			parsed, err := parseSceneFeatureFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, scenario := range parsed.scenarios {
				for i, step := range scenario.steps {
					if step.kind != "run" {
						continue
					}
					if strings.HasPrefix(step.command, "kc local ") || strings.HasPrefix(step.command, "kc catalog repo register") {
						if i+1 >= len(scenario.steps) || scenario.steps[i+1].kind != "error" || scenario.steps[i+1].errorCode != "USAGE_INVALID" {
							t.Errorf("%s must reject retired public setup command %q", path, step.command)
						}
					}
				}
			}
		}
	}
}

func TestSceneCatalogTreeFollowsLayersAndRoles(t *testing.T) {
	doc := loadSceneCatalog(t)

	ids := map[string]sceneCatalogState{}
	var roots []string
	for _, state := range doc.States {
		if strings.TrimSpace(state.ID) == "" {
			t.Fatal("state missing id")
		}
		if _, dup := ids[state.ID]; dup {
			t.Fatalf("duplicate state %s", state.ID)
		}
		ids[state.ID] = state
		if len(state.DependsOn) == 0 {
			roots = append(roots, state.ID)
		}
	}
	if _, err := os.Stat(filepath.Join(repoRoot(), "cli", "testdata", "scenes")); !os.IsNotExist(err) {
		t.Fatal("protocol scenes live under .data/scenes, not cli/testdata")
	}
	dirs := sceneStateDirs(t, ids)
	for _, state := range doc.States {
		dir := dirs[state.ID]
		parent := filepath.Base(filepath.Dir(dir))
		switch {
		case state.ID == "catalog-initialized":
			if parent != "scenes" || len(state.DependsOn) != 0 {
				t.Fatalf("catalog-initialized must be the nested tree root, parent=%s depends_on=%v", parent, state.DependsOn)
			}
		case len(state.DependsOn) != 1 || state.DependsOn[0] != parent:
			t.Fatalf("%s directory parent %s != depends_on %v", state.ID, parent, state.DependsOn)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			switch {
			case isSceneSpecialDir(name):
				if !entry.IsDir() {
					t.Fatalf("%s %s must be a directory", state.ID, name)
				}
			case name == "_meta.yaml" || name == "_bundles.yaml":
				if entry.IsDir() {
					t.Fatalf("%s %s must be a file", state.ID, name)
				}
			case entry.IsDir():
				if _, ok := ids[name]; !ok {
					t.Fatalf("%s child %s is not a fork state; mark build/materials/probes/results with _", state.ID, name)
				}
			default:
				t.Fatalf("%s has loose file %s; put construct in _build, fixtures in _materials, probes in _probes, results in _results, self-description in _meta.yaml", state.ID, name)
			}
		}
		switch state.Surface {
		case "feature", "both":
		default:
			t.Fatalf("%s unknown surface %q", state.ID, state.Surface)
		}
		switch state.Layer {
		case "0", "1", "2", "3", "M", "app":
		default:
			t.Fatalf("%s unknown layer %q", state.ID, state.Layer)
		}
		switch state.Role {
		case "provider", "consumer", "project", "operator", "governance":
		default:
			t.Fatalf("%s unknown role %q", state.ID, state.Role)
		}
		if state.Surface == "feature" || state.Surface == "both" {
			if state.Construct != "_build/construct.feature" {
				t.Fatalf("%s feature state construct must be _build/construct.feature", state.ID)
			}
			if _, err := os.Stat(filepath.Join(dir, state.Construct)); err != nil {
				t.Fatalf("%s missing construct process: %v", state.ID, err)
			}
		}
		for _, proc := range state.Processes {
			if strings.TrimSpace(proc.Source) == "" {
				t.Fatalf("%s process missing source tag", state.ID)
			}
			switch proc.Surface {
			case "feature":
				if strings.TrimSpace(proc.File) == "" {
					t.Fatalf("%s feature process needs file", state.ID)
				}
				if !strings.HasPrefix(proc.File, "_probes/") {
					t.Fatalf("%s probe must live under _probes/, got %s", state.ID, proc.File)
				}
				if _, err := os.Stat(filepath.Join(dir, proc.File)); err != nil {
					t.Fatalf("%s missing process %s: %v", state.ID, proc.File, err)
				}
			case "go-test":
				if len(proc.Evidence) == 0 {
					t.Fatalf("%s go-test process for %s needs evidence", state.ID, proc.Source)
				}
			default:
				t.Fatalf("%s process unknown surface %q", state.ID, proc.Surface)
			}
		}
	}
	if len(roots) != 1 || roots[0] != "catalog-initialized" {
		t.Fatalf("tree must have a single root catalog-initialized, got %v", roots)
	}
	for _, state := range doc.States {
		for _, parent := range state.DependsOn {
			if _, ok := ids[parent]; !ok {
				t.Fatalf("%s depends_on unknown %s", state.ID, parent)
			}
		}
	}
	if err := catalogAcyclic(doc.States); err != nil {
		t.Fatal(err)
	}

	// Directory edges describe reusable fixtures; one-off outcomes are cases.
	parents := map[string]string{
		"source-repositories-configured":   "catalog-initialized",
		"repository-attached":              "source-repositories-configured",
		"catalog-create-granted":           "catalog-initialized",
		"catalog-read-granted":             "catalog-initialized",
		"grants-bootstrapped":              "catalog-initialized",
		"http-served":                      "catalog-initialized",
		"domain-schema-published":          "repository-attached",
		"schema-read-granted":              "domain-schema-published",
		"semantic-knowledge-published":     "domain-schema-published",
		"knowledge-set-defined":            "semantic-knowledge-published",
		"dataset-manage-granted":           "knowledge-set-defined",
		"dataset-query-principals-granted": "knowledge-set-defined",
		"projection-synced":                "semantic-knowledge-published",
		"knowledge-search-granted":         "projection-synced",
		"knowledge-published":              "repository-attached",
		"dataset-defined":                  "knowledge-published",
		"proposal-opened":                  "dataset-defined",
		"proposal-preview-created":         "proposal-opened",
		"named-repositories-created":       "catalog-initialized",
		"qinghe-knowledge-published":       "named-repositories-created",
	}
	for child, parent := range parents {
		if !contains(ids[child].DependsOn, parent) {
			t.Errorf("%s must consume the reusable %s fixture", child, parent)
		}
	}
	for _, retired := range []string{"repository-registered", "product.gmv", "connector-registered", "schema-browsed", "catalog-allow-ready", "file-view-planned", "dataset-mounted", "proposal-validated", "validation-recorded", "proposal-merged", "observation-refreshed"} {
		if _, exists := ids[retired]; exists {
			t.Errorf("%s must not appear as a reusable scene state", retired)
		}
	}
	// Keep the former edge contracts at their actual verification hosts.
	cases := map[string]map[string][]string{
		"catalog-initialized": {
			"probe-audit-grant-without-inventory.feature": {"--action catalog.audit.read", "Then error FORBIDDEN"},
			"probe-revoke-catalog-read-denied.feature":    {"kc grant remove", "Then error FORBIDDEN"},
		},
		"repository-attached": {
			"probe-publish-permissions-keeps-read-denied.feature": {"kc writer put", "--aspect permissions", "Then error FORBIDDEN"},
			"probe-detach-keeps-head-readable.feature":            {"kc detach", "kc writer head", "| commit     | nonempty |"},
			"probe-archive-rejects-dataset-definition.feature":    {"kc catalog archive", "kc dataset define", "CATALOG_ARCHIVED"},
			"probe-writer-grant-isolates-principals.feature":      {"--action writer.commit", "Then error FORBIDDEN", "kc writer put"},
		},
		"proposal-preview-created": {
			"probe-structure-validation-keeps-main.feature":           {"kc governance preview validate", "reportId", "PASSED"},
			"probe-record-passed-validation-merges-candidate.feature": {"kc governance validation record", "kc governance proposal merge", "proposed"},
		},
		"knowledge-search-granted": {
			"read-grant-returns-canonical.feature": {"--action knowledge.read", "Then READ body is full canonical"},
		},
		"knowledge-set-defined": {
			"dataset-file-read-does-not-grant-repository-access.feature": {"--action file.read", "Then error FORBIDDEN"},
			"dataset-resolve-does-not-grant-read.feature":                {"--action dataset.resolve", "Then error FORBIDDEN"},
		},
	}
	for host, cases := range cases {
		for file, observations := range cases {
			if !hasProcessFile(ids[host], file) {
				t.Errorf("%s must declare verification case %s", host, file)
			}
			body, err := os.ReadFile(filepath.Join(dirs[host], "_probes", file))
			if err != nil {
				t.Fatal(err)
			}
			for _, observation := range observations {
				if !strings.Contains(string(body), observation) {
					t.Errorf("%s/%s lost %s", host, file, observation)
				}
			}
		}
	}

	access := processOn(ids["knowledge-search-granted"], "schema.access")
	if access.File != "_probes/schema-search-enforces-declared-access.feature" {
		t.Fatal("declared access is a process inside knowledge-search-granted")
	}

	for _, bundle := range doc.Bundles {
		if bundle.Suite != "product" && bundle.Suite != "search" {
			t.Fatalf("bundle %s suite=%q", bundle.ID, bundle.Suite)
		}
		if len(bundle.Walk) == 0 {
			t.Fatalf("bundle %s has an empty walk", bundle.ID)
		}
		entry, ok := ids[bundle.EntryState]
		if !ok || entry.Construct != "_build/construct.feature" {
			t.Fatalf("bundle %s entry_state %q must be constructable", bundle.ID, bundle.EntryState)
		}
		seen := map[string]struct{}{}
		var markEntry func(string)
		markEntry = func(id string) {
			if _, exists := seen[id]; exists {
				return
			}
			seen[id] = struct{}{}
			for _, parent := range ids[id].DependsOn {
				markEntry(parent)
			}
		}
		markEntry(bundle.EntryState)
		last := bundle.EntryState
		for _, step := range bundle.Walk {
			if (step.Construct == "") == (step.Process == "") {
				t.Fatalf("bundle %s walk item must be construct or process", bundle.ID)
			}
			if step.Construct != "" {
				state, ok := ids[step.Construct]
				if !ok {
					t.Fatalf("bundle %s unknown state %s", bundle.ID, step.Construct)
				}
				for _, parent := range state.DependsOn {
					if _, ok := seen[parent]; !ok {
						t.Fatalf("bundle %s construct %s missing parent %s", bundle.ID, step.Construct, parent)
					}
				}
				if _, dup := seen[step.Construct]; dup {
					t.Fatalf("bundle %s constructs %s twice", bundle.ID, step.Construct)
				}
				seen[step.Construct] = struct{}{}
				last = step.Construct
				continue
			}
			if last == "" {
				t.Fatalf("bundle %s process %s before any construct", bundle.ID, step.Process)
			}
			if !hasProcessFile(ids[last], step.Process) {
				t.Fatalf("bundle %s process %s is not in state %s", bundle.ID, step.Process, last)
			}
		}
	}
}

func TestSceneRepositoryVerificationCasesStayOnAttachedState(t *testing.T) {
	doc := loadSceneCatalog(t)
	state := loadState(doc, "repository-attached")
	for _, id := range []string{"catalog-inventory-visible", "repository-declared-readable", "changeset-previewed"} {
		if loadState(doc, id).ID != "" {
			t.Errorf("%s verifies an attached repository; it must not introduce a reusable state", id)
		}
	}
	for _, capability := range []string{"catalog.allow", "collector.preview"} {
		if !contains(state.Publishes, capability) {
			t.Errorf("repository-attached lost verification capability %s", capability)
		}
	}
	wantEvidence := map[string][]string{
		"catalog.allow": {
			"TestCatalogReadDiscoversWithoutKnowledgeRead",
			"TestCatalogInventoryDoesNotHideReposWithoutKnowledgeRead",
			"TestUndeclaredSystemRepositoryStillRequiresGrant",
			"TestAuthenticatedPrincipalReadsDeclaredRepositoryWithoutGrant",
			"TestDeclaredSystemRepositoryUsesAuthenticatedDefault",
			"TestRuntimeWriterRefusesSystemRepository",
		},
		"collector.preview": {"connector/preview_test.go", "TestPreviewThenCommit"},
	}
	for source, names := range wantEvidence {
		for _, name := range names {
			found := false
			for _, process := range state.Processes {
				found = found || process.Surface == "go-test" && process.Source == source && contains(process.Evidence, name)
			}
			if !found {
				t.Errorf("repository-attached lost %s Go evidence %s", source, name)
			}
		}
	}
	probe := "_probes/probe-inventory-without-body.feature"
	foundProbe := false
	for _, process := range state.Processes {
		foundProbe = foundProbe || process.File == probe && process.Surface == "feature" && process.Source == "catalog.allow"
	}
	if !foundProbe {
		t.Fatal("inventory discovery and body denial must be one probe on repository-attached")
	}
	path := filepath.Join(scenesRoot(), "catalog-initialized", "source-repositories-configured", "repository-attached", probe)
	feature, err := parseSceneFeatureFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(feature.scenarios) != 1 {
		t.Fatal("inventory verification must keep grant setup and observations in one isolated scenario")
	}
	observed := map[string]map[string]string{}
	command := ""
	for _, step := range feature.scenarios[0].steps {
		if step.kind == "run" {
			command = step.command
			observed[command] = map[string]string{}
		} else if step.kind == "output-has" || step.kind == "output-includes" {
			for _, row := range step.table {
				observed[command][row[0]] = row[1]
			}
		} else if step.kind == "error" {
			observed[command]["error"] = step.errorCode
		}
	}
	wantObservations := map[string]map[string]string{
		"kc grant add --principal inventory-reader --action catalog.read --catalog kr://scene/catalog": {
			"id": "nonempty", "principal": "inventory-reader", "catalog": "kr://scene/catalog", "actions.0": "catalog.read",
		},
		"kc grant list": {
			"rules[].principal": "inventory-reader", "rules[].catalog": "kr://scene/catalog", "rules[].actions.0": "catalog.read",
		},
		"kc show --as inventory-reader": {
			"catalogId": "kr://scene/catalog", "repositories[].id": "kr://scene/knowledge",
		},
		"kc catalog list --as inventory-reader":                                      {"catalogs[].id": "kr://scene/catalog"},
		"kc read --as inventory-reader --repo kr://scene/knowledge --object missing": {"error": "FORBIDDEN"},
	}
	for command, fields := range wantObservations {
		for field, want := range fields {
			if got := observed[command][field]; got != want {
				t.Errorf("inventory probe lost %q observation %s=%q; got %q", command, field, want, got)
			}
		}
	}
}

func TestSceneSystemSchemaMaterialsMatchEmbed(t *testing.T) {
	files := []string{
		"schema-definition.v1.aspect.yaml",
		"resource-descriptor.v1.aspect.yaml",
		"relation.v1.aspect.yaml",
		"readme.v1.aspect.yaml",
	}
	sceneDir := filepath.Join(scenesRoot(), "catalog-initialized", "_materials", "system")
	embedDir := filepath.Join(repoRoot(), "knowledge", "system", "schemas")
	for _, name := range files {
		want, err := os.ReadFile(filepath.Join(embedDir, name))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(sceneDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s drifted from knowledge/system/schemas", name)
		}
	}
}

func TestSceneCatalogCoversPublicProductSurfaces(t *testing.T) {
	doc := loadSceneCatalog(t)
	ids := map[string]sceneCatalogState{}
	for _, state := range doc.States {
		ids[state.ID] = state
	}
	dirs := sceneStateDirs(t, ids)
	want := map[string]string{
		"kcfs":                      "knowledge-set-defined",
		"dataset overlay":           "knowledge-set-defined",
		"attach":                    "repository-attached",
		"create":                    "catalog-initialized",
		"detach":                    "repository-attached",
		"deployment init":           "catalog-initialized",
		"governance preview create": "proposal-preview-created",
		"writer commit":             "domain-schema-published",
		"writer receipt":            "domain-schema-published",
		"writer put":                "knowledge-published",
		"schema list":               "schema-read-granted",
		"read":                      "knowledge-search-granted",
		"catalog list":              "catalog-read-granted",
	}
	for command, stateID := range want {
		state, ok := ids[stateID]
		if !ok {
			t.Errorf("command %q hangs on missing state %s", command, stateID)
			continue
		}
		if !sceneStateEvidencesCommand(t, state, dirs[stateID], command) {
			t.Errorf("command %q must be evidenced on %s", command, stateID)
		}
	}
	if _, ok := ids["http-served"]; !ok {
		t.Error("serve hangs on missing state http-served")
	}
}

func TestSceneFeaturesCoverPublicCLI(t *testing.T) {
	covered := map[string]string{}
	err := filepath.WalkDir(scenesRoot(), func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
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
			for _, step := range scene.steps {
				if step.kind != "run" {
					continue
				}
				args, splitErr := splitSceneArgs(step.command)
				if splitErr != nil {
					t.Errorf("%s: %v", path, splitErr)
					continue
				}
				surface := sceneCLISurface(args)
				if surface == "" {
					continue
				}
				if _, ok := covered[surface]; !ok {
					covered[surface] = path
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A go-test state may own a formal transport journey. Count only named
	// tests referenced by that capability's state, with literal argv reaching
	// the real Run entry (directly or through the shared kcRemote helper).
	doc := loadSceneCatalog(t)
	for _, command := range cli.CLICommandsForTest() {
		if _, ok := covered[command]; !ok {
			if !sceneCommandHasFormalGoEvidence(t, doc, command) {
				t.Errorf("public command %q has neither a scene When I run nor named formal go-test command evidence", command)
			}
		}
	}
}

func sceneStateEvidencesCommand(t *testing.T, state sceneCatalogState, dir, command string) bool {
	t.Helper()
	if sceneDirHasWhenRun(t, dir, command) {
		return true
	}
	if sceneStateHasFormalGoCommand(t, state, command) {
		return true
	}
	// kcfs is a separate binary: require its named public command contract.
	if command == "kcfs" {
		for _, process := range state.Processes {
			if process.Source == "dataset.files" && contains(process.Evidence, "TestKnowledgeSetFSPublicCommandAndUsageSurface") {
				return true
			}
		}
	}
	return false
}

func sceneDirHasWhenRun(t *testing.T, dir, command string) bool {
	t.Helper()
	for _, sub := range []string{"_build", "_probes"} {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".feature") {
				continue
			}
			parsed, err := parseSceneFeatureFile(filepath.Join(dir, sub, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			for _, scene := range parsed.scenarios {
				for _, step := range scene.steps {
					if step.kind != "run" {
						continue
					}
					args, err := splitSceneArgs(step.command)
					if err != nil {
						t.Fatal(err)
					}
					if sceneCLISurface(args) == command {
						return true
					}
				}
			}
		}
	}
	return false
}

func sceneCommandHasFormalGoEvidence(t *testing.T, doc sceneCatalogFile, command string) bool {
	t.Helper()
	for _, state := range doc.States {
		if sceneStateHasFormalGoCommand(t, state, command) {
			return true
		}
	}
	return false
}

func sceneStateHasFormalGoCommand(t *testing.T, state sceneCatalogState, command string) bool {
	t.Helper()
	paths := sceneCLITestFiles(t)
	for _, process := range state.Processes {
		if process.Surface != "go-test" {
			continue
		}
		for _, evidence := range process.Evidence {
			if !strings.HasPrefix(evidence, "Test") || strings.ContainsAny(evidence, "/. ") {
				continue // A file reference is not a named executable journey.
			}
			for _, path := range paths {
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				prefixes, err := sceneGoTestCommandPrefixes(raw, evidence)
				if err != nil {
					continue
				}
				for _, prefix := range prefixes {
					if sceneCLISurface(strings.Fields(prefix)) == command {
						return true
					}
				}
			}
		}
	}
	return false
}

var (
	sceneCLITestOnce  sync.Once
	sceneCLITestPaths []string
)

func sceneCLITestFiles(t *testing.T) []string {
	t.Helper()
	sceneCLITestOnce.Do(func() {
		paths, err := filepath.Glob(filepath.Join(repoRoot(), "cli", "*_test.go"))
		if err != nil {
			panic(err)
		}
		sceneCLITestPaths = paths
	})
	return sceneCLITestPaths
}

func sceneGoTestCommandPrefixes(source []byte, name string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "scene_evidence_test.go", source, 0)
	if err != nil {
		return nil, err
	}
	var declaration *ast.FuncDecl
	for _, item := range file.Decls {
		fn, ok := item.(*ast.FuncDecl)
		if ok && fn.Name.Name == name && fn.Recv == nil && strings.HasPrefix(name, "Test") {
			declaration = fn
			break
		}
	}
	if declaration == nil {
		return nil, fmt.Errorf("named test %s is absent", name)
	}
	var prefixes []string
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		var argv []ast.Expr
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			if fn.Name == "kcRemote" && len(call.Args) > 3 && !call.Ellipsis.IsValid() {
				argv = call.Args[3:]
			}
		case *ast.SelectorExpr:
			pkg, ok := fn.X.(*ast.Ident)
			if ok && pkg.Name == "cli" && fn.Sel.Name == "Run" && len(call.Args) == 1 {
				if literal, ok := call.Args[0].(*ast.CompositeLit); ok {
					argv = literal.Elts
				}
			}
		}
		var words []string
		for _, arg := range argv {
			literal, ok := arg.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				break
			}
			word, err := strconv.Unquote(literal.Value)
			if err != nil {
				break
			}
			words = append(words, word)
		}
		if len(words) != 0 {
			prefixes = append(prefixes, strings.Join(words, " "))
		}
		return true
	})
	return prefixes, nil
}

func TestSceneGoTestEvidenceRequiresNamedPublicRunCalls(t *testing.T) {
	source := `package cli_test
func TestFormal(t *testing.T) {
  kcRemote(t, server.URL, "provider", "create", "--name", repositoryName)
  cli.Run([]string{"writer", "put", "--repo", repositoryID})
  kcRemote(t, server.URL, principal, dynamicCommand...)
  kc(home, "detach")
}
func helper() { kcRemote(t, server.URL, principal, "catalog", "archive") }
func TestOther(t *testing.T) { cli.Run([]string{"dataset", "retire"}) }
`
	got, err := sceneGoTestCommandPrefixes([]byte(source), "TestFormal")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "\n") != "create --name\nwriter put --repo" {
		t.Fatalf("named formal evidence included dynamic, embedded, or unrelated calls: %q", got)
	}
	if _, err := sceneGoTestCommandPrefixes([]byte(source), "TestMissing"); err == nil {
		t.Fatal("missing evidence test was accepted")
	}
}

func TestSceneFeaturesCoverHelpShortestPaths(t *testing.T) {
	needles := []string{
		"kc attach",
		"kc dataset define",
		"kc grant add",
		"kc writer commit",
		"kc writer put",
		"kc writer head",
		"kc read --repo",
		"kc search --as agent:copilot --dataset",
		"kc read --as agent:copilot --dataset",
		"kc read --dataset",
		"kc login --server",
		"kc admission show",
		"kc catalog list",
		"kc show",
		"kc schema list",
	}
	found := map[string]bool{}
	err := filepath.WalkDir(scenesRoot(), func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
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
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(body)
		for _, needle := range needles {
			if strings.Contains(text, needle) {
				found[needle] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range needles {
		if !found[needle] {
			t.Errorf("help shortest path never appears in When I run: %s", needle)
		}
	}
}

func TestSceneConsumeJourneyIsOneFeature(t *testing.T) {
	needles := []string{
		"kc catalog list",
		"kc show",
		"kc schema list --repo",
		"kc search --as agent:copilot --dataset",
		"kc read --as agent:copilot --dataset",
	}
	found := ""
	err := filepath.WalkDir(scenesRoot(), func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
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
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(body)
		idx := 0
		for _, needle := range needles {
			next := strings.Index(text[idx:], needle)
			if next < 0 {
				return nil
			}
			idx += next + len(needle)
		}
		found = path
		return filepath.SkipAll
	})
	if err != nil {
		t.Fatal(err)
	}
	if found == "" {
		t.Fatal("help consume list/show/schema/SEARCH/READ --dataset is not one feature file")
	}
}

func TestSceneConsumerTaskKeepsOneAuthenticatedPrincipal(t *testing.T) {
	for _, node := range discoverConstructableNodes(t) {
		for _, path := range node.Probes {
			if filepath.Base(path) != "dataset-cli-discovers-searches-reads.feature" {
				continue
			}
			feature, err := parseSceneFeatureFile(path)
			if err != nil {
				t.Fatal(err)
			}
			principal := ""
			for _, scenario := range feature.scenarios {
				for _, step := range scenario.steps {
					if step.kind != "run" {
						continue
					}
					args, err := splitSceneArgs(step.command)
					if err != nil {
						t.Fatal(err)
					}
					parsed, err := cli.ParseArgs(args)
					if err != nil {
						t.Fatal(err)
					}
					as := cli.FlagString(parsed.Flags, "as")
					if as == "" || cli.FlagString(parsed.Flags, "server") != "$server" {
						t.Errorf("consumer task must use Run through its Server with an explicit principal: %s", step.command)
					}
					if principal == "" {
						principal = as
					} else if principal != as {
						t.Errorf("consumer task changed principal from %s to %s", principal, as)
					}
				}
			}
			return
		}
	}
	t.Fatal("missing consumer task feature")
}

func sceneCLISurface(args []string) string {
	for n := len(args); n > 0; n-- {
		path := strings.Join(args[:n], " ")
		if cli.CLICommandForTest(path) {
			return path
		}
	}
	return ""
}

func TestSceneWriteSpineUsesPublicWriter(t *testing.T) {
	doc := loadSceneCatalog(t)
	ids := map[string]sceneCatalogState{}
	for _, state := range doc.States {
		ids[state.ID] = state
	}
	dirs := sceneStateDirs(t, ids)
	want := map[string]string{
		"domain-schema-published":      "writer commit",
		"semantic-knowledge-published": "writer put",
		"knowledge-published":          "writer put",
	}
	for id, needle := range want {
		body, err := os.ReadFile(filepath.Join(dirs[id], "_build", "construct.feature"))
		if err != nil {
			t.Errorf("%s missing construct: %v", id, err)
			continue
		}
		text := string(body)
		if strings.Contains(text, "Given material") {
			t.Errorf("%s construct uses Given material instead of public kc writer", id)
		}
		if !strings.Contains(text, needle) {
			t.Errorf("%s construct must run `%s`", id, needle)
		}
	}
}

func TestSceneCatalogCoversPermissionActions(t *testing.T) {
	doc := loadSceneCatalog(t)
	ids := map[string]struct{}{}
	for _, state := range doc.States {
		ids[state.ID] = struct{}{}
	}
	want := map[string]string{
		"catalog.read":                "catalog-read-granted",
		"catalog.audit.read":          "catalog-initialized",
		"catalog.repositories.create": "catalog-create-granted",
		"knowledge.search":            "knowledge-search-granted",
		"knowledge.read":              "knowledge-search-granted",
		"knowledge.schema.read":       "schema-read-granted",
		"file.read":                   "knowledge-set-defined",
		"dataset.resolve":             "knowledge-set-defined",
		"dataset.manage":              "dataset-manage-granted",
	}
	for action, state := range want {
		if _, ok := ids[state]; !ok {
			t.Errorf("PERMISSIONS action %s: missing state %s", action, state)
		}
	}
	if processOn(loadState(doc, "knowledge-search-granted"), "delivery.chain").Source != "delivery.chain" {
		t.Fatal("AUTH-03 delivery chain must be a process on knowledge-search-granted")
	}
	if processOn(loadState(doc, "knowledge-set-defined"), "catalog.allow").Source != "catalog.allow" {
		t.Fatal("AUTH-02 consume isolation must be a process on knowledge-set-defined")
	}
}

func TestSceneCatalogFeaturesCoverProductPoints(t *testing.T) {
	doc := loadSceneCatalog(t)
	ids := map[string]sceneCatalogState{}
	for _, state := range doc.States {
		ids[state.ID] = state
	}
	dirs := sceneStateDirs(t, ids)
	for _, id := range []string{
		"domain-schema-published", "projection-synced",
		"knowledge-search-granted",
	} {
		if _, ok := ids[id]; !ok {
			t.Errorf("missing state %s", id)
		}
	}
	observation := processOn(ids["repository-attached"], "binding.observation")
	for _, name := range []string{"TestProjectionControllerNoticePullsStateWithoutChangingSnapshot", "TestStateRefreshFindsDynamicValueWithoutChangingSnapshot", "TestObservedNullProvesMissingAndFailedRefreshKeepsPublishedRevision"} {
		if !contains(observation.Evidence, name) {
			t.Errorf("dynamic observation lost named evidence %s", name)
		}
	}
	strip := filepath.Join(dirs["knowledge-search-granted"], "_probes", "search-only-principal-cannot-read.feature")
	if _, err := os.Stat(strip); err != nil {
		t.Fatalf("delivery strip probe: %v", err)
	}

	searchOnly := map[string]struct{}{
		"projection-synced":                {},
		"knowledge-search-granted":         {},
		"dataset-query-principals-granted": {},
	}
	seenSearch := map[string]struct{}{}
	for _, node := range discoverConstructableNodes(t) {
		if nodeNeedsIndex(node) {
			seenSearch[node.ID] = struct{}{}
			continue
		}
		if _, ok := searchOnly[node.ID]; ok {
			t.Errorf("product tree walk must not construct search node %s", node.ID)
		}
	}
	for id := range searchOnly {
		if _, ok := seenSearch[id]; !ok {
			t.Errorf("index tree walk must construct %s", id)
		}
	}
}

func TestSceneExecutorDiscoversEveryConstructFeature(t *testing.T) {
	want := map[string]string{}
	err := filepath.WalkDir(scenesRoot(), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), "_") && d.Name() != "_build" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "construct.feature" || filepath.Base(filepath.Dir(path)) != "_build" {
			return nil
		}
		id := filepath.Base(filepath.Dir(filepath.Dir(path)))
		if prev, dup := want[id]; dup {
			t.Fatalf("construct.feature for %s at %s and %s", id, prev, path)
		}
		want[id] = path
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, node := range discoverConstructableNodes(t) {
		got[node.ID] = node.Construct
	}
	for id, path := range want {
		if got[id] != path {
			t.Errorf("constructable node %s: got %q want %s", id, got[id], path)
		}
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("discover invented %s", id)
		}
	}
}

func TestSceneGoTestFeaturesDoNotHideSceneProbes(t *testing.T) {
	doc := loadSceneCatalog(t)
	var skipped []string
	for _, node := range discoverConstructableNodes(t) {
		if len(node.Probes) == 0 || shouldRunSceneProbes(doc, node.ID) {
			continue
		}
		skipped = append(skipped, node.ID)
	}
	sort.Strings(skipped)
	var want []string
	if strings.Join(skipped, ",") != strings.Join(want, ",") {
		t.Fatalf("skipped probes %v want %v", skipped, want)
	}
}

func TestSceneMutatingGrantProbesAreIsolated(t *testing.T) {
	doc := loadSceneCatalog(t)
	coveredLocal, coveredIndexed := false, false
	for _, node := range discoverConstructableNodes(t) {
		if len(node.Probes) < 2 || nodeNeedsWalk(node) || nodeNeedsState(node) || !shouldRunSceneProbes(doc, node.ID) {
			continue
		}
		var mutating, remaining []string
		for _, probe := range node.Probes {
			if sceneProbeChangesGrants(t, probe) {
				mutating = append(mutating, probe)
			} else {
				remaining = append(remaining, probe)
			}
		}
		if len(mutating) == 0 || len(remaining) == 0 {
			continue
		}
		indexed := nodeNeedsIndex(node)
		coveredLocal = coveredLocal || !indexed
		coveredIndexed = coveredIndexed || indexed
		t.Run(node.ID, func(t *testing.T) {
			if indexed && strings.TrimSpace(os.Getenv("KC_TEST_OPENSEARCH_URL")) == "" {
				t.Fatal("KC_TEST_OPENSEARCH_URL is required for the real indexed grant-probe isolation contract")
			}
			// Exercise the actual features, keeping every Then: grant writes
			// run first, followed by all other probes, then the grant probes
			// again. Their initial denial assertions must still hold after
			// the first run has granted access in its private environment.
			node.Probes = append(append(append([]string{}, mutating...), remaining...), mutating...)
			runSceneNode(t, doc, node, newSceneHomeCache(t))
		})
	}
	if !coveredLocal || !coveredIndexed {
		t.Fatalf("grant isolation must cover actual local and indexed scene nodes; local=%t indexed=%t", coveredLocal, coveredIndexed)
	}
}

func sceneProbeChangesGrants(t *testing.T, path string) bool {
	t.Helper()
	feature, err := parseSceneFeatureFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range feature.scenarios {
		for i, step := range scenario.steps {
			if step.kind != "run" {
				continue
			}
			args, err := splitSceneArgs(step.command)
			if err != nil {
				t.Fatal(err)
			}
			if len(args) < 2 || args[0] != "grant" || (args[1] != "add" && args[1] != "remove") {
				continue
			}
			if i+1 >= len(scenario.steps) {
				t.Fatalf("%s: grant command has no observed outcome", path)
			}
			switch scenario.steps[i+1].kind {
			case "error":
				// A rejected grant attempt is an observer of the boundary.
			case "output-has", "output-includes", "succeeds":
				return true
			default:
				t.Fatalf("%s: grant command must directly observe its success or rejection", path)
			}
		}
	}
	return false
}

func TestSceneCatalogDoesNotRegisterMaterials(t *testing.T) {
	if _, err := os.Stat(filepath.Join(scenesRoot(), "catalog.yaml")); err == nil {
		t.Fatal("protocol scenes must not use catalog.yaml; that name is the product Catalog registry")
	}
	if _, err := os.Stat(filepath.Join(scenesRoot(), "sources.yaml")); err == nil {
		t.Fatal("protocol scenes must not keep sources.yaml; node labels stay in _meta.yaml")
	}
	if _, err := os.Stat(filepath.Join(scenesRoot(), "checklist.md")); err == nil {
		t.Fatal("protocol scenes must not keep checklist.md; command coverage is the construct/probe tests")
	}
	if _, err := os.Stat(filepath.Join(scenesRoot(), "check")); err == nil {
		t.Fatal("protocol scenes must not keep check/; the state directories are the use cases")
	}
	initFeature, err := os.ReadFile(filepath.Join(scenesRoot(), "catalog-initialized", "_build", "construct.feature"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(initFeature), "Given material") {
		t.Fatal("the root only inits; materials enter later via Writer steps")
	}
	doc := loadSceneCatalog(t)
	ids := map[string]sceneCatalogState{}
	for _, state := range doc.States {
		ids[state.ID] = state
	}
	for id, dir := range sceneStateDirs(t, ids) {
		construct := filepath.Join(dir, "_build", "construct.feature")
		body, err := os.ReadFile(construct)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(body), "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "Given material ") {
				continue
			}
			material := strings.TrimSpace(strings.TrimPrefix(trimmed, "Given material "))
			path := filepath.Join(dir, "_materials", material+".yaml")
			if _, err := os.Stat(path); err != nil {
				t.Errorf("%s Given material %s missing fixture %s", id, material, path)
			}
		}
	}
}

func TestSceneResultsHangOnEveryNode(t *testing.T) {
	doc := loadSceneCatalog(t)
	ids := map[string]sceneCatalogState{}
	for _, state := range doc.States {
		ids[state.ID] = state
	}
	for id, dir := range sceneStateDirs(t, ids) {
		info, err := os.Stat(filepath.Join(dir, "_results"))
		if err != nil || !info.IsDir() {
			t.Errorf("%s missing _results directory: %v", id, err)
		}
	}
	ignore, err := os.ReadFile(filepath.Join(scenesRoot(), ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ignore), "_results/") {
		t.Fatal(".data/scenes/.gitignore must ignore per-node _results/")
	}
	cmd := exec.Command("git", "check-ignore", "-v", "--", filepath.Join(".data", "scenes", "catalog-initialized", "_results", "latest.json"))
	cmd.Dir = repoRoot()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git check-ignore: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "_results") {
		t.Fatalf("per-node _results must be gitignored, got %s", out)
	}
}

func TestSceneAccessorMaterialsAreTheOnboardingRuntime(t *testing.T) {
	dir := filepath.Join(
		scenesRoot(),
		"catalog-initialized",
		"source-repositories-configured",
		"repository-attached",
		"_materials",
		"accessor",
	)
	for _, name := range []string{"compose.yaml", "compose.walk.yaml", "Dockerfile", "access.py", "collector.py", "observer.py", "shop.py", "source.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("accessor onboarding material %s: %v", name, err)
		}
	}
}

func TestSceneWarehouseMaterialsUseTypeDirectories(t *testing.T) {
	base := filepath.Join(scenesRoot(), "catalog-initialized", "named-repositories-created", "qinghe-knowledge-published", "_materials")
	tableMeta := filepath.Join(base, "table-meta")
	sales := filepath.Join(base, "sales-semantic")
	if _, err := os.Stat(filepath.Join(tableMeta, "objects")); err == nil {
		t.Fatal("table-meta must not wrap instances in objects/; use type directories")
	}
	if _, err := os.Stat(filepath.Join(sales, "objects")); err == nil {
		t.Fatal("sales-semantic must not wrap instances in objects/; use type directories")
	}
	for _, name := range []string{"tables", "columns", "data-jobs", "relations", "resources"} {
		if _, err := os.Stat(filepath.Join(tableMeta, name)); err != nil {
			t.Fatalf("table-meta missing %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(tableMeta, "jobs")); err == nil {
		t.Fatal("table-meta jobs belong in data-jobs/, not jobs/")
	}
	for _, name := range []string{"metrics", "semantic-models", "relations"} {
		if _, err := os.Stat(filepath.Join(sales, name)); err != nil {
			t.Fatalf("sales-semantic missing %s: %v", name, err)
		}
	}
}

func TestSceneTreeIsSelfContained(t *testing.T) {
	forbidden := []string{".data/data-warehouse", "kr://dw/", "warehouse-agent"}
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
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(raw)
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Errorf("%s depends on warehouse suite via %q", path, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func loadState(doc sceneCatalogFile, id string) sceneCatalogState {
	for _, state := range doc.States {
		if state.ID == id {
			return state
		}
	}
	return sceneCatalogState{}
}

func processOn(state sceneCatalogState, source string) sceneCatalogProcess {
	for _, proc := range state.Processes {
		if proc.Source == source {
			return proc
		}
	}
	return sceneCatalogProcess{}
}

func hasProcessFile(state sceneCatalogState, file string) bool {
	for _, proc := range state.Processes {
		if proc.File == file || filepath.Base(proc.File) == filepath.Base(file) {
			return true
		}
	}
	return false
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func catalogAcyclic(states []sceneCatalogState) error {
	ids := map[string]sceneCatalogState{}
	for _, state := range states {
		ids[state.ID] = state
	}
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case 1:
			return errSceneCatalogCycle(id)
		case 2:
			return nil
		}
		state[id] = 1
		for _, parent := range ids[id].DependsOn {
			if err := visit(parent); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range ids {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

type sceneCatalogCycleError string

func (e sceneCatalogCycleError) Error() string { return "cycle at " + string(e) }

func errSceneCatalogCycle(id string) error { return sceneCatalogCycleError(id) }

type sceneTreeNode struct {
	ID        string
	Dir       string
	Ancestors []string
	Construct string
	Probes    []string
}

func discoverConstructableNodes(t *testing.T) []sceneTreeNode {
	t.Helper()
	doc := loadSceneCatalog(t)
	ids := map[string]sceneCatalogState{}
	for _, state := range doc.States {
		ids[state.ID] = state
	}
	dirs := sceneStateDirs(t, ids)
	var nodes []sceneTreeNode
	var walk func(id string, ancestors []string)
	walk = func(id string, ancestors []string) {
		dir, ok := dirs[id]
		if !ok {
			t.Fatalf("state %s has no directory", id)
		}
		construct := filepath.Join(dir, "_build", "construct.feature")
		runnable := false
		if _, err := os.Stat(construct); err == nil {
			runnable = true
		}
		var probes []string
		if entries, err := os.ReadDir(filepath.Join(dir, "_probes")); err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".feature") {
					continue
				}
				probes = append(probes, filepath.Join(dir, "_probes", entry.Name()))
			}
			sort.Strings(probes)
		}
		if runnable {
			nodes = append(nodes, sceneTreeNode{
				ID: id, Dir: dir, Ancestors: append([]string{}, ancestors...),
				Construct: construct, Probes: probes,
			})
		}
		var children []string
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), "_") {
				continue
			}
			if _, ok := ids[entry.Name()]; ok {
				children = append(children, entry.Name())
			}
		}
		sort.Strings(children)
		next := ancestors
		if runnable {
			next = append(append([]string{}, ancestors...), id)
		}
		for _, child := range children {
			walk(child, next)
		}
	}
	walk("catalog-initialized", nil)
	return nodes
}

func nodeNeedsIndex(node sceneTreeNode) bool {
	chain := append(append([]string{}, node.Ancestors...), node.ID)
	for _, id := range chain {
		if id == "projection-synced" {
			return true
		}
	}
	files := append([]string{node.Construct}, node.Probes...)
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.Contains(string(raw), "operations projection sync") {
			return true
		}
	}
	return false
}

func nodeNeedsWalk(node sceneTreeNode) bool {
	chain := append(append([]string{}, node.Ancestors...), node.ID)
	for _, id := range chain {
		if id == "named-repositories-created" {
			return true
		}
	}
	return false
}

func TestSceneWalkLeafIsTwoNamedRepositories(t *testing.T) {
	var created, ready sceneTreeNode
	for _, node := range discoverConstructableNodes(t) {
		switch node.ID {
		case "named-repositories-created":
			created = node
		case "qinghe-knowledge-published":
			ready = node
		case "index-ready", "sales-dataset-defined":
			t.Errorf("%s is a walkthrough result, not a shared fixture", node.ID)
		}
	}
	if created.ID == "" || ready.ID == "" || !nodeNeedsWalk(created) || !nodeNeedsWalk(ready) {
		t.Fatal("the two-repository walkthrough fixtures must be constructable and separate from product DFS")
	}
	body, err := os.ReadFile(created.Construct)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, needle := range []string{"kc create --name table-meta", "kc create --name sales-semantic", "kc attach --repo table-meta", "kc attach --repo sales-semantic"} {
		if !strings.Contains(text, needle) {
			t.Errorf("named-repositories-created missing %s", needle)
		}
	}
	readyBody, err := os.ReadFile(ready.Construct)
	if err != nil {
		t.Fatal(err)
	}
	readyText := string(readyBody)
	for _, needle := range []string{"--dir $materials/sales-semantic", "--dir $materials/table-meta", "semantic-model/shop.sales", "metric/shop.gmv", "table/shop.orders", "column/shop.orders.order_id", "data-job/shop.refresh_sales_mart", "resource/shop-sql", "kc access --repo table-meta", "kc invoke --repo table-meta"} {
		if !strings.Contains(readyText, needle) {
			t.Errorf("qinghe-knowledge-published missing %s", needle)
		}
	}
	indexBody, err := os.ReadFile(filepath.Join(ready.Dir, "_probes", "probe-sync-projections-and-search.feature"))
	if err != nil {
		t.Fatal(err)
	}
	indexText := string(indexBody)
	for _, needle := range []string{"kc operations projection sync --repo table-meta", "kc operations projection sync --repo sales-semantic", "kc search --repo table-meta", "kc search --repo sales-semantic", "table/shop.orders", "metric/shop.gmv"} {
		if !strings.Contains(indexText, needle) {
			t.Errorf("index-ready missing %s", needle)
		}
	}
	datasetBody, err := os.ReadFile(filepath.Join(ready.Dir, "_probes", "probe-publish-dataset-with-scoped-members.feature"))
	if err != nil {
		t.Fatal(err)
	}
	datasetText := string(datasetBody)
	for _, needle := range []string{"kc dataset define --dataset qinghe-sales", "--source table-meta=", "--source sales-semantic=", "@tables@tables", "@semantic-models@semantic-models", "@metrics@metrics", "kc read --dataset qinghe-sales --object table/shop.orders", "kc read --dataset qinghe-sales --object metric/shop.gmv", "0.objectId", "data-job/shop.refresh_sales_mart"} {
		if !strings.Contains(datasetText, needle) {
			t.Errorf("sales-dataset-defined missing %s", needle)
		}
	}
}

func nodeNeedsState(node sceneTreeNode) bool {
	raw, err := os.ReadFile(node.Construct)
	if err != nil {
		return false
	}
	return strings.Contains(string(raw), "operations projection notice")
}

func shouldRunSceneProbes(doc sceneCatalogFile, stateID string) bool {
	return loadState(doc, stateID).Surface != "go-test"
}

func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func scenesRoot() string {
	return filepath.Join(repoRoot(), ".data", "scenes")
}

func sceneStateDirs(t *testing.T, ids map[string]sceneCatalogState) map[string]string {
	t.Helper()
	root := scenesRoot()
	found := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		if strings.HasPrefix(name, "_") {
			return filepath.SkipDir
		}
		if _, ok := ids[name]; !ok {
			return nil
		}
		if prev, dup := found[name]; dup {
			t.Fatalf("state %s at %s and %s", name, prev, path)
		}
		found[name] = path
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for id := range ids {
		if _, ok := found[id]; !ok {
			t.Fatalf("state %s has no nested directory under %s", id, root)
		}
	}
	for id, path := range found {
		if err := os.MkdirAll(filepath.Join(path, "_results"), 0o755); err != nil {
			t.Fatalf("state %s _results: %v", id, err)
		}
	}
	return found
}

func isSceneSpecialDir(name string) bool {
	switch name {
	case "_build", "_materials", "_probes", "_results":
		return true
	default:
		return false
	}
}

var (
	sceneCatalogOnce sync.Once
	sceneCatalogDoc  sceneCatalogFile
	sceneCatalogErr  error
)

func loadSceneCatalog(t *testing.T) sceneCatalogFile {
	t.Helper()
	sceneCatalogOnce.Do(func() {
		cmd := exec.Command("python3", filepath.Join(scenesRoot(), "tree.py"), "--json")
		out, err := cmd.Output()
		if err != nil {
			msg := err.Error()
			if ee, ok := err.(*exec.ExitError); ok {
				if text := strings.TrimSpace(string(ee.Stderr)); text != "" {
					msg = text
				}
			}
			sceneCatalogErr = fmt.Errorf("scene tree: %s", msg)
			return
		}
		if err := json.Unmarshal(out, &sceneCatalogDoc); err != nil {
			sceneCatalogErr = err
		}
	})
	if sceneCatalogErr != nil {
		t.Fatal(sceneCatalogErr)
	}
	return sceneCatalogDoc
}
