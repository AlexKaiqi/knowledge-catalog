package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"kc/cli"

	"gopkg.in/yaml.v3"
)

type sceneCatalogFile struct {
	Version      int                      `yaml:"version"`
	Sources      []sceneCatalogSource     `yaml:"sources"`
	States       []sceneCatalogState      `yaml:"states"`
	Bundles      []sceneCatalogBundle     `yaml:"bundles"`
	Capabilities []sceneCatalogCapability `yaml:"capabilities"`
	Actions      []sceneCatalogAction     `yaml:"actions"`
	Features     []sceneCatalogFeature    `yaml:"features"`
}

type sceneCatalogCapability struct {
	Surface string `yaml:"surface"`
	State   string `yaml:"state"`
}

type sceneCatalogAction struct {
	Action string `yaml:"action"`
	State  string `yaml:"state"`
}

type sceneCatalogFeature struct {
	ID       string   `yaml:"id"`
	Group    string   `yaml:"group"`
	Scene    string   `yaml:"scene"`
	Runner   string   `yaml:"runner"`
	Evidence []string `yaml:"evidence"`
}

type sceneCatalogSource struct {
	ID       string `yaml:"id"`
	OwnerDoc string `yaml:"owner_doc"`
	Summary  string `yaml:"summary"`
}

type sceneCatalogState struct {
	ID          string                `yaml:"id"`
	Layer       string                `yaml:"layer"`
	Role        string                `yaml:"role"`
	Surface     string                `yaml:"surface"`
	Source      string                `yaml:"source"`
	DependsOn   []string              `yaml:"depends_on"`
	Publishes   []string              `yaml:"publishes"`
	AlsoFreezes []string              `yaml:"also_freezes"`
	Construct   string                `yaml:"construct"`
	Processes   []sceneCatalogProcess `yaml:"processes"`
}

type sceneCatalogProcess struct {
	File     string   `yaml:"file"`
	Source   string   `yaml:"source"`
	Surface  string   `yaml:"surface"`
	Evidence []string `yaml:"evidence"`
}

type sceneCatalogBundle struct {
	ID      string             `yaml:"id"`
	Suite   string             `yaml:"suite"`
	Summary string             `yaml:"summary"`
	Walk    []sceneCatalogWalk `yaml:"walk"`
}

type sceneCatalogWalk struct {
	Construct string `yaml:"construct"`
	Process   string `yaml:"process"`
}

func TestSceneCatalogTreeFollowsLayersAndRoles(t *testing.T) {
	doc := loadSceneCatalog(t)
	if doc.Version != 18 {
		t.Fatalf("catalog version=%d", doc.Version)
	}

	sources := map[string]sceneCatalogSource{}
	for _, src := range doc.Sources {
		if strings.TrimSpace(src.ID) == "" || strings.TrimSpace(src.OwnerDoc) == "" {
			t.Fatal("source needs id and owner_doc")
		}
		if _, dup := sources[src.ID]; dup {
			t.Fatalf("duplicate source %s", src.ID)
		}
		sources[src.ID] = src
	}

	ids := map[string]sceneCatalogState{}
	verified := map[string]struct{}{}
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
			case entry.IsDir():
				if _, ok := ids[name]; !ok {
					t.Fatalf("%s child %s is not a fork state; mark build/materials/probes/results with _", state.ID, name)
				}
			default:
				t.Fatalf("%s has loose file %s; put construct in _build, fixtures in _materials, probes in _probes, results in _results", state.ID, name)
			}
		}
		switch state.Surface {
		case "feature", "go-test", "both":
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
		if state.Source != "" {
			if _, ok := sources[state.Source]; !ok {
				t.Fatalf("%s unknown source %s", state.ID, state.Source)
			}
		}
		for _, src := range state.Publishes {
			if _, ok := sources[src]; !ok {
				t.Fatalf("%s publishes unknown %s", state.ID, src)
			}
		}
		for _, src := range state.AlsoFreezes {
			if _, ok := sources[src]; !ok {
				t.Fatalf("%s also_freezes unknown %s", state.ID, src)
			}
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
			if _, ok := sources[proc.Source]; !ok {
				t.Fatalf("%s process unknown source %s", state.ID, proc.Source)
			}
			verified[proc.Source] = struct{}{}
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
	for id := range sources {
		if _, ok := verified[id]; !ok {
			t.Fatalf("source %s has no process", id)
		}
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

	if !contains(ids["system-schema-published"].DependsOn, "catalog-initialized") {
		t.Fatal("system schema is the first provider-visible step after init")
	}
	if !contains(ids["repository-attached"].DependsOn, "system-schema-published") {
		t.Fatal("an empty knowledge repository opens after System Schema is readable")
	}
	if !contains(ids["repository-registered"].DependsOn, "repository-attached") {
		t.Fatal("Catalog admit is a separate user step from host attach")
	}
	if !contains(ids["repository-archived"].DependsOn, "repository-registered") {
		t.Fatal("repository archive hangs on a Catalog-admitted source")
	}
	if !contains(ids["grants-bootstrapped"].DependsOn, "catalog-initialized") {
		t.Fatal("grant bootstrap forks from an initialized catalog")
	}
	if !contains(ids["drafts-ingested"].DependsOn, "repository-registered") {
		t.Fatal("ingest hangs on a Catalog-admitted repository")
	}
	if !contains(ids["domain-schema-published"].DependsOn, "drafts-ingested") {
		t.Fatal("domain schema commits the ingested ChangeSet; attach is not enough")
	}
	if !contains(ids["semantic-knowledge-constructed"].DependsOn, "domain-schema-published") {
		t.Fatal("semantic knowledge hangs on the published Domain Schema, not a GMV node")
	}
	if !contains(ids["knowledge-set-defined"].DependsOn, "semantic-knowledge-constructed") {
		t.Fatal("knowledge set forks from constructed semantic knowledge, not from projection")
	}
	if _, still := ids["product.gmv"]; still {
		t.Fatal("do not name states after GMV")
	}
	if _, still := ids["connector-registered"]; still {
		t.Fatal("connector runtime is not registered in Catalog")
	}
	if !contains(ids["access-handle-published"].DependsOn, "domain-schema-published") {
		t.Fatal("access handle forks from published schema, not a Catalog connector registry")
	}
	if !contains(ids["observation-refreshed"].DependsOn, "access-handle-published") {
		t.Fatal("observation refresh hangs on the published handle")
	}
	if !contains(ids["file-view-planned"].DependsOn, "knowledge-set-defined") {
		t.Fatal("file gateway plan hangs on a named knowledge set")
	}
	if !contains(ids["workspace-mounted"].DependsOn, "file-view-planned") {
		t.Fatal("kcfs mount hangs on the file gateway plan, not on Writer")
	}
	if !contains(ids["proposal-opened"].DependsOn, "workspace-defined") {
		t.Fatal("governance proposal hangs on a composed workspace")
	}
	if !contains(ids["proposal-previewed"].DependsOn, "proposal-opened") {
		t.Fatal("preview hangs on an opened proposal")
	}
	if !contains(ids["proposal-validated"].DependsOn, "proposal-previewed") {
		t.Fatal("validate hangs on a preview")
	}
	if !contains(ids["validation-recorded"].DependsOn, "proposal-validated") {
		t.Fatal("validation record hangs on a protocol validate")
	}
	if !contains(ids["proposal-merged"].DependsOn, "validation-recorded") {
		t.Fatal("merge hangs on recorded validation")
	}
	if !contains(ids["http-served"].DependsOn, "catalog-initialized") {
		t.Fatal("HTTP facade hangs on an initialized catalog")
	}
	if !contains(ids["catalog-read-granted"].DependsOn, "repository-registered") {
		t.Fatal("catalog.read hangs on a Catalog-admitted source")
	}
	if !contains(ids["knowledge-published"].DependsOn, "repository-registered") {
		t.Fatal("canonical publish hangs on a Catalog-admitted source")
	}
	if !contains(ids["workspace-consume-granted"].DependsOn, "knowledge-set-defined") {
		t.Fatal("workspace.consume hangs on a named knowledge set")
	}
	if !contains(ids["schema-read-granted"].DependsOn, "domain-schema-published") {
		t.Fatal("schema.read hangs on published Domain Schema")
	}
	if !contains(ids["permissions-aspect-published"].DependsOn, "knowledge-published") {
		t.Fatal("permissions Aspect is knowledge, not an allow rule")
	}
	if !contains(ids["catalog-archived"].DependsOn, "repository-attached") {
		t.Fatal("catalog archive hangs on an attached repository")
	}
	if !contains(ids["grant-revoked"].DependsOn, "catalog-read-granted") {
		t.Fatal("revoke hangs on an existing grant world")
	}

	access := processOn(ids["knowledge-search-granted"], "schema.access")
	if access.File != "_probes/probe-declared-access.feature" {
		t.Fatal("declared access is a process inside knowledge-search-granted")
	}

	for _, bundle := range doc.Bundles {
		if bundle.Suite != "product" && bundle.Suite != "search" {
			t.Fatalf("bundle %s suite=%q", bundle.ID, bundle.Suite)
		}
		if len(bundle.Walk) == 0 {
			t.Fatalf("bundle %s has an empty walk", bundle.ID)
		}
		if bundle.Walk[0].Construct != "catalog-initialized" {
			t.Fatalf("bundle %s must start by constructing catalog-initialized", bundle.ID)
		}
		seen := map[string]struct{}{}
		last := ""
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

func TestSceneSystemSchemaMaterialsMatchEmbed(t *testing.T) {
	files := []string{
		"schema-definition.v1.aspect.yaml",
		"resource-descriptor.v1.aspect.yaml",
		"relation.v1.aspect.yaml",
		"source-profile.v1.aspect.yaml",
	}
	sceneDir := filepath.Join(scenesRoot(), "catalog-initialized", "system-schema-published", "_materials")
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
	ids := map[string]struct{}{}
	for _, state := range doc.States {
		ids[state.ID] = struct{}{}
	}
	mapped := map[string]string{}
	for _, cap := range doc.Capabilities {
		if strings.TrimSpace(cap.Surface) == "" || strings.TrimSpace(cap.State) == "" {
			t.Fatal("capability needs surface and state")
		}
		if _, dup := mapped[cap.Surface]; dup {
			t.Fatalf("duplicate capability surface %q", cap.Surface)
		}
		if _, ok := ids[cap.State]; !ok {
			t.Fatalf("capability %q points at unknown state %s", cap.Surface, cap.State)
		}
		mapped[cap.Surface] = cap.State
	}
	for _, command := range cli.CLICommandsForTest() {
		if _, ok := mapped[command]; !ok {
			t.Errorf("public command %q has no scene capability", command)
		}
	}
	for _, extra := range []string{"serve", "kcfs"} {
		if _, ok := mapped[extra]; !ok {
			t.Errorf("product surface %q has no scene capability", extra)
		}
	}
	if mapped["kcfs"] != "workspace-mounted" {
		t.Fatalf("kcfs must hang on workspace-mounted, got %q", mapped["kcfs"])
	}
	if mapped["local workspace overlay"] != "file-view-planned" {
		t.Fatalf("overlay must hang on file-view-planned, got %q", mapped["local workspace overlay"])
	}
	if mapped["catalog repo register"] != "repository-registered" {
		t.Fatalf("register must hang on repository-registered, got %q", mapped["catalog repo register"])
	}
	if mapped["catalog repo archive"] != "repository-archived" {
		t.Fatalf("repo archive must hang on repository-archived, got %q", mapped["catalog repo archive"])
	}
	if mapped["local grant bootstrap"] != "grants-bootstrapped" {
		t.Fatalf("bootstrap must hang on grants-bootstrapped, got %q", mapped["local grant bootstrap"])
	}
	if mapped["governance preview create"] != "proposal-previewed" {
		t.Fatalf("preview must hang on proposal-previewed, got %q", mapped["governance preview create"])
	}
	if mapped["pack"] != "drafts-ingested" {
		t.Fatalf("ingest must hang on drafts-ingested, got %q", mapped["pack"])
	}
	if mapped["writer commit"] != "domain-schema-published" {
		t.Fatalf("commit must hang on domain-schema-published, got %q", mapped["writer commit"])
	}
	if mapped["writer receipt"] != "domain-schema-published" {
		t.Fatalf("receipt must hang on domain-schema-published, got %q", mapped["writer receipt"])
	}
	if mapped["writer put"] != "knowledge-published" {
		t.Fatalf("put must hang on knowledge-published, got %q", mapped["writer put"])
	}
	if mapped["knowledge schema list"] != "schema-read-granted" {
		t.Fatalf("schema browse must hang on schema-read-granted, got %q", mapped["knowledge schema list"])
	}
	if mapped["knowledge read"] != "knowledge-read-granted" {
		t.Fatalf("knowledge read must hang on knowledge-read-granted, got %q", mapped["knowledge read"])
	}
	if mapped["catalog list"] != "catalog-read-granted" {
		t.Fatalf("catalog list must hang on catalog-read-granted, got %q", mapped["catalog list"])
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
	for _, command := range cli.CLICommandsForTest() {
		if _, ok := covered[command]; !ok {
			t.Errorf("public command %q never appears in a scene When I run", command)
		}
	}
}

func TestSceneFeaturesCoverHelpShortestPaths(t *testing.T) {
	needles := []string{
		"kc catalog repo register",
		"kc workspace define",
		"kc admin grant add",
		"kc pack",
		"kc writer commit",
		"kc writer put",
		"kc writer head",
		"kc knowledge read --repo",
		"kc knowledge search --as taihu:alice --workspace",
		"kc knowledge read --as agent:copilot --workspace",
		"kc workspace pin --workspace",
		"kc login --server",
		"kc catalog list",
		"kc catalog show",
		"kc knowledge schema list",
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
		"drafts-ingested":                "pack",
		"domain-schema-published":        "writer commit",
		"semantic-knowledge-constructed": "writer put",
		"knowledge-published":            "writer put",
		"permissions-aspect-published":   "writer put",
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
	mapped := map[string]string{}
	for _, item := range doc.Actions {
		if strings.TrimSpace(item.Action) == "" || strings.TrimSpace(item.State) == "" {
			t.Fatal("action needs action and state")
		}
		if _, dup := mapped[item.Action]; dup {
			t.Fatalf("duplicate permission action %q", item.Action)
		}
		if _, ok := ids[item.State]; !ok {
			t.Fatalf("action %q points at unknown state %s", item.Action, item.State)
		}
		mapped[item.Action] = item.State
	}
	want := map[string]string{
		"catalog.read":          "catalog-read-granted",
		"knowledge.search":      "knowledge-search-granted",
		"knowledge.read":        "knowledge-read-granted",
		"knowledge.schema.read": "schema-read-granted",
		"workspace.consume":     "workspace-consume-granted",
		"workspace.resolve":     "workspace-resolve-granted",
	}
	for action, state := range want {
		if mapped[action] != state {
			t.Errorf("PERMISSIONS action %s: state %q want %s", action, mapped[action], state)
		}
	}
	if processOn(loadState(doc, "knowledge-search-granted"), "delivery.chain").Source != "delivery.chain" {
		t.Fatal("AUTH-03 delivery chain must be a process on knowledge-search-granted")
	}
	if processOn(loadState(doc, "workspace-consume-granted"), "catalog.allow").Source != "catalog.allow" {
		t.Fatal("AUTH-02 consume isolation must be a process on workspace-consume-granted")
	}
}

func TestSceneCatalogFeaturesCoverProductPoints(t *testing.T) {
	doc := loadSceneCatalog(t)
	ids := map[string]sceneCatalogState{}
	for _, state := range doc.States {
		ids[state.ID] = state
	}
	dirs := sceneStateDirs(t, ids)
	seen := map[string]struct{}{}
	groups := map[string]int{}
	live := 0
	allowedGroups := map[string]struct{}{
		"host": {}, "write": {}, "schema": {}, "discover": {}, "auth": {}, "index": {},
		"consume": {}, "handle": {}, "files": {}, "govern": {}, "ops": {}, "frozen": {},
	}
	for _, feat := range doc.Features {
		if strings.TrimSpace(feat.ID) == "" || strings.TrimSpace(feat.Scene) == "" {
			t.Fatal("feature needs id and scene")
		}
		if _, dup := seen[feat.ID]; dup {
			t.Fatalf("duplicate feature %s", feat.ID)
		}
		seen[feat.ID] = struct{}{}
		if _, ok := ids[feat.Scene]; !ok {
			t.Fatalf("feature %s unknown scene %s", feat.ID, feat.Scene)
		}
		if _, ok := allowedGroups[feat.Group]; !ok {
			t.Fatalf("feature %s unknown group %q", feat.ID, feat.Group)
		}
		groups[feat.Group]++
		switch feat.Runner {
		case "scene":
			construct := filepath.Join(dirs[feat.Scene], "_build", "construct.feature")
			if _, err := os.Stat(construct); err != nil {
				t.Errorf("scene feature %s (%s) has no _build/construct.feature", feat.ID, feat.Scene)
			}
		case "go-test":
			hasGo := false
			for _, proc := range ids[feat.Scene].Processes {
				if proc.Surface == "go-test" && len(proc.Evidence) > 0 {
					hasGo = true
					break
				}
			}
			if !hasGo {
				t.Errorf("go-test feature %s has no go-test process evidence on %s", feat.ID, feat.Scene)
			}
		case "live":
			live++
			if feat.ID != "identity.taihu-live" {
				t.Errorf("only identity.taihu-live may be live, got %s", feat.ID)
			}
			if len(feat.Evidence) == 0 {
				t.Errorf("live feature %s needs evidence", feat.ID)
			}
		default:
			t.Fatalf("feature %s unknown runner %q", feat.ID, feat.Runner)
		}
	}
	if live != 1 {
		t.Fatalf("want exactly one live feature (real-user auth), got %d", live)
	}
	for group := range allowedGroups {
		if groups[group] == 0 {
			t.Errorf("product group %s has no feature", group)
		}
	}
	required := []string{
		"local.init", "local.repository", "local.store", "system.schema",
		"writer.ingest", "writer.commit", "knowledge.publish", "connector.preview",
		"schema.publish", "schema.browse-mechanics", "knowledge.schema.read",
		"catalog.read", "catalog.source-profile", "workspace.define", "workspace.resolve", "workspace.consume", "workspace.retire", "workspace.federated", "catalog.archive",
		"identity.local", "identity.taihu-live", "identity.principals", "grant.revoke", "permissions.aspect-not-gate",
		"index.declarative", "index.dynamic",
		"knowledge.search", "knowledge.read", "knowledge.provenance", "knowledge.relations", "delivery.strip", "retrieval.refine",
		"access.handle", "resource.access", "binding.observation",
		"workspace.files", "workspace.kcfs",
		"governance.proposal", "governance.merge", "operations.hook", "operations.gate",
		"access.evidence",
		"product.absence",
	}
	requiredSet := map[string]struct{}{}
	for _, id := range required {
		requiredSet[id] = struct{}{}
		if _, ok := seen[id]; !ok {
			t.Errorf("missing product feature %s", id)
		}
	}
	for id := range seen {
		if _, ok := requiredSet[id]; !ok {
			t.Errorf("feature %s is not in the product map", id)
		}
	}
	scenes := map[string]string{}
	for _, feat := range doc.Features {
		scenes[feat.ID] = feat.Scene
	}
	if scenes["schema.publish"] != "domain-schema-published" {
		t.Fatalf("schema.publish must hang on domain-schema-published, got %q", scenes["schema.publish"])
	}
	if scenes["writer.commit"] != "domain-schema-published" {
		t.Fatalf("writer.commit must hang on domain-schema-published, got %q", scenes["writer.commit"])
	}
	if scenes["writer.ingest"] != "drafts-ingested" {
		t.Fatalf("writer.ingest must hang on drafts-ingested, got %q", scenes["writer.ingest"])
	}
	if scenes["index.declarative"] != "projection-synced" {
		t.Fatalf("declarative index must hang on projection-synced, got %q", scenes["index.declarative"])
	}
	if scenes["index.dynamic"] != "observation-refreshed" {
		t.Fatalf("dynamic index must hang on observation-refreshed, got %q", scenes["index.dynamic"])
	}
	if scenes["delivery.strip"] != "knowledge-search-granted" {
		t.Fatalf("delivery strip must hang on knowledge-search-granted, got %q", scenes["delivery.strip"])
	}

	searchOnly := map[string]struct{}{
		"projection-synced":        {},
		"knowledge-search-granted": {},
		"knowledge-read-granted":   {},
		"principals-granted":       {},
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
	want := []string{"observation-refreshed", "workspace-consume-granted", "workspace-resolve-granted", "workspace-retired"}
	if strings.Join(skipped, ",") != strings.Join(want, ",") {
		t.Fatalf("skipped probes %v want %v", skipped, want)
	}
}

func TestSceneMutatingGrantProbesRunLast(t *testing.T) {
	for _, node := range discoverConstructableNodes(t) {
		if len(node.Probes) < 2 {
			continue
		}
		mutating := false
		for _, probe := range node.Probes {
			raw, err := os.ReadFile(probe)
			if err != nil {
				t.Fatal(err)
			}
			writesGrant := bytes.Contains(raw, []byte("admin grant add")) || bytes.Contains(raw, []byte("admin grant remove"))
			if mutating && !writesGrant {
				t.Errorf("%s: observational probe %s runs after a probe that mutates grants", node.ID, filepath.Base(probe))
			}
			if writesGrant {
				mutating = true
			}
		}
	}
}

func TestSceneCatalogDoesNotRegisterMaterials(t *testing.T) {
	raw, err := os.ReadFile(sceneCatalogPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "\nmaterials:") {
		t.Fatal("knowledge fixtures are not a Catalog registry; they enter through node construct steps")
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

func nodeNeedsState(node sceneTreeNode) bool {
	chain := append(append([]string{}, node.Ancestors...), node.ID)
	for _, id := range chain {
		if id == "observation-refreshed" {
			return true
		}
	}
	raw, err := os.ReadFile(node.Construct)
	if err != nil {
		return false
	}
	return strings.Contains(string(raw), "operations projection notice")
}

func shouldRunSceneProbes(doc sceneCatalogFile, stateID string) bool {
	found := false
	scene := false
	for _, feat := range doc.Features {
		if feat.Scene != stateID {
			continue
		}
		found = true
		if feat.Runner == "scene" {
			scene = true
		}
	}
	if scene {
		return true
	}
	if found {
		return false
	}
	return true
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

func sceneCatalogPath() string {
	return filepath.Join(scenesRoot(), "catalog.yaml")
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

func loadSceneCatalog(t *testing.T) sceneCatalogFile {
	t.Helper()
	raw, err := os.ReadFile(sceneCatalogPath())
	if err != nil {
		t.Fatal(err)
	}
	var doc sceneCatalogFile
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}
