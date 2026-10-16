package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"kc/kernel"
)

func TestAcceptanceValueSourceInputsPinThePublicParameterDiagnostic(t *testing.T) {
	root := filepath.Join("..", "..", ".data", "scenes", "service-initialized", "named-repositories-created", "qinghe-object-published")
	raw, err := os.ReadFile(filepath.Join(root, "_probes", "acceptance", "probes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var probes []struct {
		ID    string `json:"id"`
		Steps []struct {
			Writer string `json:"writer"`
			Expect struct {
				Contains map[string][]string `json:"contains"`
			} `json:"expect"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(raw, &probes); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ file, probe string }{
		{"unknown-value-source-kind.yaml", "accept-live-reject-unknown-source-kind"},
		{"inline-value-source.yaml", "accept-live-writer-rejects-declarations"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, "_materials", "acceptance", "variants", "rejected", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			header := strings.SplitN(strings.TrimPrefix(string(raw), "---\n"), "\n---\n", 2)[0]
			var meta map[string]any
			if err := yaml.Unmarshal([]byte(header), &meta); err != nil {
				t.Fatal(err)
			}
			source, err := json.Marshal(meta["valueSource"])
			if err != nil {
				t.Fatal(err)
			}
			_, diagnostic := valueSource(map[string]any{"value-source": string(source)})
			if kernel.CodeOf(diagnostic) != kernel.ErrUsageInvalid || !strings.Contains(diagnostic.Error(), "--value-source") {
				t.Fatalf("the original rejected source must identify the public parameter: %v", diagnostic)
			}
			checked := 0
			for _, probe := range probes {
				if probe.ID != tc.probe {
					continue
				}
				for _, step := range probe.Steps {
					if !strings.HasSuffix(step.Writer, "/"+tc.file) {
						continue
					}
					for _, fragment := range step.Expect.Contains["error.message"] {
						if !strings.Contains(diagnostic.Error(), fragment) {
							t.Errorf("probe requires unselected wording %q instead of the original public diagnostic: %v", fragment, diagnostic)
						}
					}
					checked++
				}
			}
			if checked != 1 {
				t.Fatalf("expected the exact original rejection probe, checked %d", checked)
			}
		})
	}
}
