package capability

import (
	"testing"

	retrieval "kc/discovery"
	objectmodel "kc/object"
)

func TestDefaultManifestSupportsDeclaredTextTemplateAndKeywordNormalizer(t *testing.T) {
	manifest := DefaultManifest()
	for _, test := range []struct {
		name, kind, use string
		options         map[string]any
		missing         string
	}{
		{"text template", "text", "text", map[string]any{"template": "{name}"}, ""},
		{"text missing skip", "text", "text", nil, objectmodel.IndexMissingSkip},
		{"keyword lowercase", "keyword", "filter", map[string]any{"normalizer": "lowercase"}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			declaration := capabilityDeclaration(t, test.kind, test.options)
			declaration.Uses = []string{test.use}
			declaration.Inputs[0].Missing = test.missing
			if reason := manifest.Check(declaration, []retrieval.DeclarationInput{{Type: "string"}}); reason != "" {
				t.Fatalf("default build is unsupported: %s", reason)
			}
		})
	}
}

func TestTemplateCheckRejectsUnknownMalformedAndAmbiguousPlaceholders(t *testing.T) {
	manifest := DefaultManifest()
	for _, template := range []string{"{unknown}", "{name", "name}", "{}"} {
		declaration := capabilityDeclaration(t, "text", map[string]any{"template": template})
		if manifest.Check(declaration, []retrieval.DeclarationInput{{Type: "string"}}) == "" {
			t.Fatalf("unsupported template was admitted: %q", template)
		}
	}
	declaration := capabilityDeclaration(t, "text", map[string]any{"template": "{name}"})
	declaration.Inputs = append(declaration.Inputs, declaration.Inputs[0])
	if manifest.Check(declaration, []retrieval.DeclarationInput{{Type: "string"}, {Type: "string"}}) == "" {
		t.Fatal("ambiguous template input was admitted")
	}
}

func TestVectorManifestAdmitsOnlyRegisteredModelsAndImplementedDistance(t *testing.T) {
	declaration := capabilityDeclaration(t, "vector", map[string]any{"model": "embed/test-hash@1", "distance": "cosine"})
	declaration.Uses = []string{objectmodel.IndexUseSemantic}
	inputs := []retrieval.DeclarationInput{{Type: "string"}}
	if reason := DefaultManifest().Check(declaration, inputs); reason == "" {
		t.Fatal("no-model instance admitted a vector member")
	}
	if reason := DefaultManifest("embed/test-hash@1").Check(declaration, inputs); reason != "" {
		t.Fatalf("registered model was refused: %s", reason)
	}
	declaration.Build.Options["distance"] = "l2"
	if reason := DefaultManifest("embed/test-hash@1").Check(declaration, inputs); reason == "" {
		t.Fatal("unimplemented vector distance was admitted")
	}
	declaration.Build.Options["distance"] = "cosine"
	declaration.Build.Options["model"] = "embed/unregistered@1"
	if reason := DefaultManifest("embed/test-hash@1").Check(declaration, inputs); reason == "" {
		t.Fatal("unregistered model was admitted")
	}
}
