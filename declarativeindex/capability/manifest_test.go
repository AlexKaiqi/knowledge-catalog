package capability

import (
	"encoding/json"
	"strings"
	"testing"

	retrieval "kc/discovery"
	objectmodel "kc/object"
)

func capabilityDeclaration(t *testing.T, kind string, options map[string]any) objectmodel.IndexDeclaration {
	t.Helper()
	build := map[string]any{"kind": kind}
	for key, value := range options {
		build[key] = value
	}
	declaration, err := objectmodel.ParseIndexDeclaration(objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "ops/search", AspectName: "index", MemberKey: "name"}, map[string]any{
		"catalog": ManifestName + "@" + ManifestVersion,
		"inputs":  []any{map[string]any{"from": "schema/service.definition#/fields/name"}},
		"build":   build, "uses": []any{"text"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return declaration
}

func TestPublishedManifestRoundTripKeepsInstanceChecks(t *testing.T) {
	manifest := DefaultManifest("embed/test-hash@1")
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var published Manifest
	if err := json.Unmarshal(data, &published); err != nil {
		t.Fatal(err)
	}
	declaration := capabilityDeclaration(t, "text", map[string]any{"analyzer": "standard"})
	inputs := []retrieval.DeclarationInput{{Type: "string"}}
	if reason := published.Check(declaration, inputs); reason != "" {
		t.Fatalf("published capability rejects its own admitted declaration: %s", reason)
	}
	declaration.Build.Options["analyzer"] = "not-installed"
	if reason := published.Check(declaration, inputs); !strings.Contains(reason, "analyzer") || !strings.Contains(reason, "not-installed") {
		t.Fatalf("unregistered analyzer has no precise reason: %s", reason)
	}
	before := published.Digest
	published.Builds[0].Options[0].Values = []string{"simple"}
	declaration.Build.Options["analyzer"] = "standard"
	if reason := published.Check(declaration, inputs); !strings.Contains(reason, "standard") {
		t.Fatalf("instance checks stale options after manifest changed: %s", reason)
	}
	if manifest.Digest != before {
		t.Fatal("reading a published manifest mutated the original instance")
	}
}

func TestManifestRebuildFollowsDeclaredOptionContract(t *testing.T) {
	manifest := DefaultManifest()
	previous := capabilityDeclaration(t, "text", map[string]any{"analyzer": "standard"})
	next := capabilityDeclaration(t, "text", map[string]any{"analyzer": "standard"})
	next.Inputs[0].Boost = 3
	if manifest.Rebuilds(previous, next) {
		t.Fatal("a scoring-only weight change rebuilds the retrieval field")
	}
	next.Inputs[0].MaxTokens = 100
	if !manifest.Rebuilds(previous, next) {
		t.Fatal("changed input truncation did not rebuild")
	}
	next = capabilityDeclaration(t, "text", map[string]any{"analyzer": "simple"})
	if !manifest.Rebuilds(previous, next) {
		t.Fatal("changed analyzer did not rebuild")
	}
	manifest.Builds[0].Options = append(manifest.Builds[0].Options, BuildOption{Name: "scoreWeight", Type: "number", Rebuild: false})
	previous.Build.Options["scoreWeight"], next.Build.Options["scoreWeight"] = 1, 2
	next.Build.Options["analyzer"] = "standard"
	if manifest.Rebuilds(previous, next) {
		t.Fatal("an instance-declared scoring-only build option ignores its rebuild contract")
	}
}

func TestIndexAffordanceExplainsDeclarationsAndCapabilityWorkflow(t *testing.T) {
	usage := Affordance().Usage
	for _, term := range []string{"kc/index-declaration", "能力清单", "describe", "trial", "不写入"} {
		if !strings.Contains(usage, term) {
			t.Errorf("plugin explanation omits %q", term)
		}
	}
	if strings.Contains(usage, "Schema 字段通过 text/filter/sort") || strings.Contains(Affordance().Summary, "字段 access") {
		t.Fatal("plugin still assigns index intent to structural Schema")
	}
}

func TestIndexAffordanceDistinguishesDatasetRecoveryAndCurrentObservation(t *testing.T) {
	usage := Affordance().Usage
	if strings.Contains(usage, "由对账恢复") || !strings.Contains(usage, "显式同步恢复") {
		t.Fatal("Dataset corpus help promises unavailable automatic recovery")
	}
	if !strings.Contains(usage, "新观察复核") || !strings.Contains(usage, "索引原观察") {
		t.Fatal("dynamic candidate help conflates indexed and consumer observations")
	}
}

func TestManifestDoesNotAdmitControlsAbsentFromPhysicalCompiler(t *testing.T) {
	manifest := DefaultManifest("embed/test-hash@1")
	for _, test := range []struct {
		name   string
		change func(*objectmodel.IndexDeclaration)
	}{
		{"analyzer", func(d *objectmodel.IndexDeclaration) { d.Build.Options["analyzer"] = "simple" }},
		{"multiple keyword inputs", func(d *objectmodel.IndexDeclaration) {
			d.Build.Kind = "keyword"
			d.Build.Options = map[string]any{}
			d.Uses = []string{"filter"}
			d.Inputs = append(d.Inputs, d.Inputs[0])
		}},
		{"text inputs above the limit", func(d *objectmodel.IndexDeclaration) {
			for len(d.Inputs) <= 8 {
				d.Inputs = append(d.Inputs, d.Inputs[0])
			}
		}},
		{"synonyms", func(d *objectmodel.IndexDeclaration) { d.Build.Options["synonyms"] = "lexicon/service@v1" }},
		{"prefix", func(d *objectmodel.IndexDeclaration) { d.Build.Options["prefix"] = true }},
		{"boost", func(d *objectmodel.IndexDeclaration) { d.Inputs[0].Boost = 3 }},
		{"truncation", func(d *objectmodel.IndexDeclaration) { d.Inputs[0].MaxTokens = 100 }},
		{"vector", func(d *objectmodel.IndexDeclaration) {
			d.Build.Kind = "vector"
			d.Build.Options = map[string]any{"model": "embed/not-installed@1"}
			d.Uses = []string{"semantic"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			declaration := capabilityDeclaration(t, "text", map[string]any{"analyzer": "standard"})
			test.change(&declaration)
			inputs := make([]retrieval.DeclarationInput, len(declaration.Inputs))
			for i := range inputs {
				inputs[i].Type = "string"
			}
			if manifest.Check(declaration, inputs) == "" {
				t.Fatal("instance admitted an option that never reaches physical construction")
			}
		})
	}
	data, err := json.Marshal(manifest)
	if err != nil || !strings.Contains(string(data), `"unavailableReason"`) {
		t.Fatal("unavailable construction controls are hidden from the public manifest")
	}
}

// Each text input is extracted as its own text cell and bare MATCH fans out
// over every text field, so several inputs reach physical construction.
func TestManifestAdmitsSeveralTextInputs(t *testing.T) {
	manifest := DefaultManifest()
	declaration := capabilityDeclaration(t, "text", map[string]any{"analyzer": "standard"})
	for len(declaration.Inputs) < 8 {
		declaration.Inputs = append(declaration.Inputs, declaration.Inputs[0])
	}
	inputs := make([]retrieval.DeclarationInput, len(declaration.Inputs))
	for i := range inputs {
		inputs[i].Type = "string"
	}
	if reason := manifest.Check(declaration, inputs); reason != "" {
		t.Fatalf("text declaration with 8 inputs rejected: %s", reason)
	}
}
