package home_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	retrieval "kc/discovery"
	apphome "kc/home"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
)

func TestAcceptanceIndexProbesUseActualMaterialDiagnostics(t *testing.T) {
	root := filepath.Join("..", ".data", "scenes", "service-initialized", "named-repositories-created", "qinghe-object-published")
	materialRoot := filepath.Join(root, "_materials")
	data, err := os.ReadFile(filepath.Join(root, "_probes", "acceptance", "probes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var probes []struct {
		ID    string `json:"id"`
		Steps []struct {
			Trial  string `json:"trial"`
			Writer string `json:"writer"`
			Expect struct {
				Has      map[string]any      `json:"has"`
				Contains map[string][]string `json:"contains"`
			} `json:"expect"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(data, &probes); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		file       string
		code       kernel.ErrorCode
		probeCount int
	}{
		{"index-missing-field.yaml", kernel.ErrObjectRefUnresolved, 3},
		{"index-with-engine.yaml", kernel.ErrUsageInvalid, 3},
		{"index-raw-mapping.yaml", kernel.ErrUsageInvalid, 3},
		{"index-empty-inputs.yaml", kernel.ErrUsageInvalid, 1},
		{"index-bad-locator.yaml", kernel.ErrUsageInvalid, 1},
		{"index-with-value-source.yaml", kernel.ErrUsageInvalid, 1},
	} {
		t.Run(tc.file, func(t *testing.T) {
			s := testkit.NewSetup(t, "kr://acceptance/materials")
			_, err := s.Writer.Commit("seed-schema", objectmodel.ChangeSet{
				TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
				BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
				Operations: testkit.PutEntity("schema/table.properties", map[string]any{
					"entity": "Table", "aspect": "properties", "fields": map[string]any{"name": map[string]any{"type": "string"}},
				}, ""),
			})
			if err != nil {
				t.Fatal(err)
			}
			at := testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
			content, err := os.ReadFile(filepath.Join(materialRoot, "acceptance", "variants", "rejected", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			unit := repofile.Parse(string(content))
			if unit == nil || unit.DeclarationError() != nil {
				t.Fatalf("material is not a valid object file: %#v", unit)
			}
			ws := &apphome.Home{Store: s.Store, Reader: s.Reader, Writer: s.Writer}
			trial, err := ws.TrialIndexDeclarations("declarative-index", retrieval.DeclarationTrialRequest{
				Repository: s.RepositoryID, Commit: at,
				Documents: []retrieval.DeclarationDocument{{Path: tc.file, Content: string(content)}},
			}, allowAll)
			if err != nil || len(trial.Members) != 1 {
				t.Fatalf("trial did not judge the original file: %#v %v", trial, err)
			}
			_, writerErr := s.Writer.Commit("reject-original-material", objectmodel.ChangeSet{
				TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef, BaseCommit: at, ExpectedTargetCommit: at,
				Operations: []objectmodel.Operation{{Op: objectmodel.OpPut, Address: unit.Address, SchemaRef: unit.SchemaRef, ValueSource: unit.ValueSource, Value: unit.Value}},
			})
			if kernel.CodeOf(writerErr) != tc.code || trial.Members[0].Code != string(tc.code) || trial.Members[0].Status != retrieval.DeclarationInvalid || trial.Members[0].Reason != writerErr.Error() {
				t.Fatalf("trial/Writer material contract: trial=%#v Writer=%v want=%s", trial, writerErr, tc.code)
			}
			if trial.Members[0].Member != unit.Address {
				t.Fatalf("trial diagnostic does not identify the original member: %#v want %#v", trial.Members[0].Member, unit.Address)
			}
			if testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != at {
				t.Fatal("trial or rejected Writer input advanced HEAD")
			}
			checked := 0
			for _, probe := range probes {
				for _, step := range probe.Steps {
					if !strings.HasSuffix(step.Trial, "/"+tc.file) && !strings.HasSuffix(step.Writer, "/"+tc.file) {
						continue
					}
					key := "error.code"
					if step.Trial != "" {
						key = "members.0.code"
					}
					want, ok := step.Expect.Has[key].(string)
					if !ok || want != string(tc.code) && want != "$saved.trial.members.0.code" {
						t.Errorf("%s %s expects %v; original material actually returns %s", probe.ID, key, step.Expect.Has[key], tc.code)
					}
					for _, fragments := range step.Expect.Contains {
						for _, fragment := range fragments {
							if !strings.Contains(writerErr.Error(), fragment) {
								t.Errorf("%s diagnostic for %s omits required position/reason %q: %v", probe.ID, tc.file, fragment, writerErr)
							}
						}
					}
					checked++
				}
			}
			if checked != tc.probeCount {
				t.Fatalf("the original Writer and new trial probes were not both checked: %d", checked)
			}
		})
	}
}
