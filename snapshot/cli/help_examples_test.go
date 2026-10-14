package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	snapshotcli "kc/snapshot/cli"
	"kc/snapshot/client"
	"kc/snapshot/dataset"
)

func TestSnapshotHelpRecipeExamplesAreCompletePortableInputs(t *testing.T) {
	define := exampleHelp(t, "dataset define")
	overlay := exampleHelp(t, "dataset overlay")
	raw := exampleYAML(t, define, "recipe.yaml")
	if !bytes.Equal(raw, exampleYAML(t, overlay, "recipe.yaml")) {
		t.Fatal("define and overlay describe different base recipes")
	}
	recipe, err := dataset.ParseKnowledgeSetRecipe(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := dataset.KnowledgeSet{
		SetID: "delivery", Revision: 1,
		Metadata: map[string]string{"description": "项目材料", "audience": "分析团队"},
		Sources: []dataset.KnowledgeSetSource{
			{Repository: "docs", Selector: snapshot.DefaultRef, Path: dataset.MountPath("reference"), SubPath: "handbook"},
			{Repository: "archive", Selector: snapshot.DefaultRef, Path: dataset.MountPath("legacy"), SubPath: "handouts"},
			{Repository: "metrics", Selector: snapshot.DefaultRef, File: "exports/kpi.csv", Target: "data/kpi.csv"},
		},
	}
	definition := dataset.KnowledgeSet{SetID: recipe.Name, Revision: 1, Sources: recipe.Sources(), Metadata: recipe.Metadata}
	if !reflect.DeepEqual(definition, want) {
		t.Fatalf("the documented recipe changed its selected files, paths or metadata: %#v", definition)
	}
	if err := dataset.ValidateKnowledgeSet(definition); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "recipe.yaml")
	writeExample(t, file, raw)
	if err := snapshotcli.New(nil, snapshotcli.Context{}).Validate("dataset define", map[string]any{
		"dataset": "delivery", "revision": "1", "file": file,
	}); err != nil {
		t.Fatalf("the published help's file cannot reach define preflight: %v", err)
	}
	assertExampleUnchanged(t, file, raw)
}

func TestSnapshotHelpOverlayExamplesRunOfflineWithoutPublishing(t *testing.T) {
	help := exampleHelp(t, "dataset overlay")
	recipeRaw, overlayRaw := exampleYAML(t, help, "recipe.yaml"), exampleYAML(t, help, "overlay.yaml")
	if _, err := dataset.ParseKnowledgeSetOverlay(overlayRaw); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	recipeFile, overlayFile, outputFile := filepath.Join(dir, "recipe.yaml"), filepath.Join(dir, "overlay.yaml"), filepath.Join(dir, "merged.yaml")
	writeExample(t, recipeFile, recipeRaw)
	writeExample(t, overlayFile, overlayRaw)
	transport := &helpRejectingTransport{}
	sdk, err := client.New(client.Config{BaseURL: "http://unused.invalid", Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	executor := snapshotcli.New(sdk, snapshotcli.Context{})
	flags := map[string]any{"file": recipeFile, "overlay": overlayFile}
	value, err := executor.Execute(context.Background(), "dataset overlay", flags, client.RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := exampleMergedDefinition()
	if !reflect.DeepEqual(value, want) {
		t.Fatalf("offline stdout changed the documented range/paths/metadata or froze an unpublished source: %#v", value)
	}
	if err := dataset.ValidateKnowledgeSet(want); err != nil {
		t.Fatal(err)
	}
	flags["out"] = outputFile
	value, err = executor.Execute(context.Background(), "dataset overlay", flags, client.RequestOptions{})
	if err != nil || !reflect.DeepEqual(value, map[string]any{"setId": "delivery", "out": outputFile}) {
		t.Fatalf("offline file output did not return its local receipt: %#v %v", value, err)
	}
	output, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := dataset.ParseKnowledgeSetRecipe(output)
	if err != nil {
		t.Fatal(err)
	}
	definition := dataset.KnowledgeSet{SetID: merged.Name, Revision: 1, Sources: merged.Sources(), Metadata: merged.Metadata}
	// Portable recipes list mounts before files; the selected coordinates and
	// target paths must survive that representation change exactly.
	wantPortable := dataset.KnowledgeSet{
		SetID: want.SetID, Revision: want.Revision, Metadata: want.Metadata,
		Sources: []dataset.KnowledgeSetSource{want.Sources[0], want.Sources[2], want.Sources[1], want.Sources[3]},
	}
	if !reflect.DeepEqual(definition, wantPortable) {
		t.Fatalf("portable output changed the verified candidate: %#v", definition)
	}
	assertExampleUnchanged(t, recipeFile, recipeRaw)
	assertExampleUnchanged(t, overlayFile, overlayRaw)
	if transport.calls != 0 {
		t.Fatalf("offline help examples made %d remote calls or attempted publication", transport.calls)
	}
}

func exampleMergedDefinition() dataset.KnowledgeSet {
	return dataset.KnowledgeSet{
		SetID: "delivery", Revision: 1,
		Metadata: map[string]string{"description": "项目材料", "audience": "分析团队"},
		Sources: []dataset.KnowledgeSetSource{
			{Repository: "docs", Selector: snapshot.DefaultRef, Path: dataset.MountPath("reference"), SubPath: "guides"},
			{Repository: "metrics", Selector: snapshot.DefaultRef, File: "exports/latest.csv", Target: "data/kpi.csv"},
			{Repository: "assets", Selector: snapshot.DefaultRef, Path: dataset.MountPath("media"), SubPath: "images"},
			{Repository: "docs", Selector: snapshot.DefaultRef, File: "notes/release.txt", Target: "notes/release.txt"},
		},
	}
}

func TestSnapshotHelpBaseRevExampleKeepsAHeadCheckRatherThanACommitSelector(t *testing.T) {
	help := exampleHelp(t, "dataset overlay")
	raw := exampleYAML(t, help, "recipe.yaml")
	const commit = "c46e887d91abf0da61b7c549be733e07a0d2c1f08464e0d0c3ae97989c85ad50"
	const optional = `    # 可选：baseRev: "来源管理者确认的完整 commit ID"`
	if !bytes.Contains(raw, []byte(optional)) {
		t.Fatal("the recipe no longer shows where the optional head check belongs")
	}
	raw = bytes.Replace(raw, []byte(optional), []byte(`    baseRev: "`+commit+`"`), 1)
	recipe, err := dataset.ParseKnowledgeSetRecipe(raw)
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := dataset.ParseKnowledgeSetOverlay(exampleYAML(t, help, "overlay.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	definition, err := dataset.MergeOverlay(dataset.KnowledgeSet{
		SetID: recipe.Name, Revision: 1, Sources: recipe.Sources(), Metadata: recipe.Metadata,
	}, overlay)
	if err != nil {
		t.Fatal(err)
	}
	want := exampleMergedDefinition()
	want.Sources[0].BaseRev, want.Sources[3].BaseRev = commit, commit
	if !reflect.DeepEqual(definition, want) {
		t.Fatalf("overlay changed the declared head check or made it a frozen commit: %#v", definition)
	}
	for _, claim := range []string{"只核对该引用当前 tip", "不选任意旧 commit", "不能在本地合成时冻结远端版本"} {
		if !strings.Contains(help, claim) {
			t.Errorf("help no longer distinguishes recipe checks from accepted commits: %s", claim)
		}
	}
}

func TestSnapshotHelpYAMLSelectorsMustBeExplicit(t *testing.T) {
	help := exampleHelp(t, "dataset overlay")
	for _, file := range []string{"recipe.yaml", "overlay.yaml"} {
		t.Run(file, func(t *testing.T) {
			raw := bytes.Replace(exampleYAML(t, help, file), []byte("    selector: refs/heads/main\n"), nil, 1)
			var err error
			if file == "recipe.yaml" {
				_, err = dataset.ParseKnowledgeSetRecipe(raw)
			} else {
				_, err = dataset.ParseKnowledgeSetOverlay(raw)
			}
			if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
				t.Fatalf("omitting YAML selector unexpectedly behaved like omitted argv selector: %v", err)
			}
		})
	}
}

func TestSnapshotHelpArgvMappingsKeepDefaultSelectorAndSourceTargetOrder(t *testing.T) {
	for _, row := range []struct {
		input string
		want  dataset.KnowledgeSetSource
	}{
		{"docs", dataset.KnowledgeSetSource{Repository: "docs", Selector: snapshot.DefaultRef}},
		{"docs=@", dataset.KnowledgeSetSource{Repository: "docs", Selector: snapshot.DefaultRef, Path: dataset.MountPath("")}},
		{"kr://scene/knowledge@", dataset.KnowledgeSetSource{Repository: "kr://scene/knowledge", Selector: snapshot.DefaultRef, Path: dataset.MountPath("")}},
		{"docs@reference@handbook", dataset.KnowledgeSetSource{Repository: "docs", Selector: snapshot.DefaultRef, Path: dataset.MountPath("reference"), SubPath: "handbook"}},
		{"docs=HEAD@reference@handbook", dataset.KnowledgeSetSource{Repository: "docs", Selector: "HEAD", Path: dataset.MountPath("reference"), SubPath: "handbook"}},
		{"docs=refs/heads/release@reference@handbook", dataset.KnowledgeSetSource{Repository: "docs", Selector: "refs/heads/release", Path: dataset.MountPath("reference"), SubPath: "handbook"}},
	} {
		sources, err := dataset.ParseSourceMappings([]string{row.input})
		if err != nil || !reflect.DeepEqual(sources, []dataset.KnowledgeSetSource{row.want}) {
			t.Errorf("source example changed: %s → %#v (%v)", row.input, sources, err)
		}
	}
	files, err := dataset.ParseFileMappings([]string{"docs@handbook/start.md@reference/index.md"})
	want := []dataset.KnowledgeSetSource{{Repository: "docs", Selector: snapshot.DefaultRef, File: "handbook/start.md", Target: "reference/index.md"}}
	if err != nil || !reflect.DeepEqual(files, want) {
		t.Fatalf("file-source example swapped source/target or omitted its default selector: %#v %v", files, err)
	}
}

func TestSnapshotHelpOverlayKeepsUnsupportedSelectorsBeforeSourceResolution(t *testing.T) {
	help := exampleHelp(t, "dataset overlay")
	replace := func(raw []byte) []byte {
		return bytes.ReplaceAll(raw, []byte("selector: refs/heads/main"), []byte("selector: main"))
	}
	definition := executeOfflineExample(t, replace(exampleYAML(t, help, "recipe.yaml")), replace(exampleYAML(t, help, "overlay.yaml")))
	want := exampleMergedDefinition()
	for i := range want.Sources {
		want.Sources[i].Selector = "main"
	}
	if !reflect.DeepEqual(definition, want) {
		t.Fatalf("offline overlay must preserve nonempty, consistent unresolved selectors: %#v", definition)
	}
	if err := dataset.ValidateKnowledgeSet(definition); err != nil {
		t.Fatalf("shape validation unexpectedly resolved a source reference: %v", err)
	}
	for _, claim := range []string{"发布时由连接的 lakeFS 解析", "selector: main", "本机成功不证明输入全合法"} {
		if !strings.Contains(help, claim) {
			t.Errorf("help implies offline acceptance proves a source selector is supported: missing %s", claim)
		}
	}
}

func TestSnapshotHelpOverlayKeepsUncompiledFileTargetsUntilFileListValidation(t *testing.T) {
	help := exampleHelp(t, "dataset overlay")
	replace := func(raw []byte) []byte {
		return bytes.ReplaceAll(raw, []byte("target: data/kpi.csv"), []byte("target: ../outside.csv"))
	}
	definition := executeOfflineExample(t, replace(exampleYAML(t, help, "recipe.yaml")), replace(exampleYAML(t, help, "overlay.yaml")))
	want := exampleMergedDefinition()
	want.Sources[1].Target = "../outside.csv"
	if !reflect.DeepEqual(definition, want) {
		t.Fatalf("offline overlay did not preserve the uncompiled file target: %#v", definition)
	}
	if err := dataset.ValidateKnowledgeSet(definition); err != nil {
		t.Fatalf("shape validation unexpectedly compiled per-file paths: %v", err)
	}
	// ReplayDefinition uses the same file-list compiler as PrepareKnowledgeSet,
	// without opening sources or accepting a Dataset publication.
	pin, err := dataset.ReplayDefinition(definition, dataset.ResolvedKnowledgeSet{
		SetID: definition.SetID, Revision: definition.Revision,
		Repositories: map[kernel.RepositoryID]kernel.CommitID{"docs": "test-fixed", "metrics": "test-fixed", "assets": "test-fixed"},
	})
	if kernel.CodeOf(err) != kernel.ErrKnowledgeSetInvalid || len(pin.Items) != 0 {
		t.Fatalf("an escaping candidate became a compiled delivery list: %#v %v", pin, err)
	}
	for _, claim := range []string{"target: ../outside.csv", "发布前编译文件清单时", "不能成为合法交付"} {
		if !strings.Contains(help, claim) {
			t.Errorf("help conflates local composition and per-file path validation: missing %s", claim)
		}
	}
}

func TestSnapshotHelpOverlayEmptyPatchReturnsBaseWithoutCombinationValidation(t *testing.T) {
	help := exampleHelp(t, "dataset overlay")
	raw := bytes.Replace(exampleYAML(t, help, "recipe.yaml"), []byte("path: reference"), []byte("path: ../outside"), 1)
	recipe, err := dataset.ParseKnowledgeSetRecipe(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := dataset.KnowledgeSet{SetID: recipe.Name, Revision: 1, Sources: recipe.Sources(), Metadata: recipe.Metadata}
	definition := executeOfflineExample(t, raw, []byte("{}\n"))
	if !reflect.DeepEqual(definition, want) {
		t.Fatalf("empty patch changed or reinterpreted its base recipe: %#v", definition)
	}
	if kernel.CodeOf(dataset.ValidateKnowledgeSet(definition)) != kernel.ErrKnowledgeSetInvalid {
		t.Fatal("the directory traversal fixture must fail the separate combination validator")
	}
	if !strings.Contains(help, "空补丁只返回底稿、不补做组合校验") {
		t.Fatal("help promises combination validation for a no-op overlay")
	}
}

func executeOfflineExample(t *testing.T, recipeRaw, overlayRaw []byte) dataset.KnowledgeSet {
	t.Helper()
	dir := t.TempDir()
	recipeFile, overlayFile := filepath.Join(dir, "recipe.yaml"), filepath.Join(dir, "overlay.yaml")
	writeExample(t, recipeFile, recipeRaw)
	writeExample(t, overlayFile, overlayRaw)
	value, err := snapshotcli.New(nil, snapshotcli.Context{}).Execute(context.Background(), "dataset overlay", map[string]any{
		"file": recipeFile, "overlay": overlayFile,
	}, client.RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := value.(dataset.KnowledgeSet)
	if !ok {
		t.Fatalf("offline overlay returned an accepted or remote result instead of a candidate: %#v", value)
	}
	assertExampleUnchanged(t, recipeFile, recipeRaw)
	assertExampleUnchanged(t, overlayFile, overlayRaw)
	return definition
}

func exampleHelp(t *testing.T, path string) string {
	t.Helper()
	for _, command := range snapshotcli.Commands() {
		if command.Path == path {
			return command.Usage
		}
	}
	t.Fatalf("no published help for %s", path)
	return ""
}

func exampleYAML(t *testing.T, help, file string) []byte {
	t.Helper()
	marker := "cat > " + file + " <<'YAML'\n"
	start := strings.Index(help, marker)
	if start < 0 {
		t.Fatalf("%s is not a complete copyable quoted heredoc", file)
	}
	body := help[start+len(marker):]
	end := strings.Index(body, "\nYAML\n")
	if end < 0 {
		t.Fatalf("%s heredoc closing delimiter is indented or missing", file)
	}
	return []byte(body[:end] + "\n")
}

func writeExample(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
}

func assertExampleUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	if body, err := os.ReadFile(path); err != nil || !bytes.Equal(body, want) {
		t.Fatalf("offline example rewrote its input %s: %q %v", path, body, err)
	}
}
