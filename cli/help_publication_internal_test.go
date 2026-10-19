package cli

import (
	"kc/internal/testkit"
	"strings"
	"testing"

	objectmodel "kc/object"
	objectformat "kc/object/format"
)

func TestPublicationHelpNavigatesExistingDatasetBeforeWriting(t *testing.T) {
	result := Run([]string{"help", "write"})
	if result.Status != 0 {
		t.Fatal(result)
	}
	last := -1
	for _, step := range []string{"kc show", "kc dataset show", "kc files list", "kc files read", "kc writer commit"} {
		next := strings.Index(result.Stdout, step)
		if next <= last {
			t.Errorf("write help must reach %q after the preceding publication step", step)
		}
		last = next
	}
	for _, fact := range []string{"--revision", "Schema", "已有仓", "kc writer put", "kc help writer commit"} {
		if !strings.Contains(result.Stdout, fact) {
			t.Errorf("publication help omits %q", fact)
		}
	}
}

func TestWriterCommitHelpContainsValidCompleteReadmeFile(t *testing.T) {
	result := Run([]string{"help", "writer", "commit"})
	if result.Status != 0 {
		t.Fatal(result)
	}
	start := strings.Index(result.Stdout, "    ---\n")
	if start < 0 {
		t.Fatal("writer commit help has no complete frontmatter file")
	}
	var lines []string
	for _, line := range strings.Split(result.Stdout[start:], "\n") {
		if !strings.HasPrefix(line, "    ") {
			break
		}
		lines = append(lines, strings.TrimPrefix(line, "    "))
	}
	unit := objectformat.Parse(strings.Join(lines, "\n"))
	if unit == nil || unit.Address.Kind != objectmodel.KindAspect || unit.Address.AspectName != "readme" || unit.SchemaRef != string(objectmodel.ObjectID("schema/core/readme/v1")) {
		t.Fatalf("help file does not describe a complete README Aspect: %#v", unit)
	}
	if body, ok := testkit.MarkdownBody(unit.Value); !ok || !strings.HasPrefix(body, "# ") {
		t.Fatalf("help file must retain Markdown as the body, not a JSON envelope: %#v", unit.Value)
	}
	if !strings.Contains(Run([]string{"help", "diff"}).Stdout, "kc help writer commit") {
		t.Fatal("diff must share the writer commit directory file contract")
	}
}

func TestSchemaHelpDistinguishesInspectionFromPublicationInput(t *testing.T) {
	result := Run([]string{"help", "schema", "describe"})
	for _, fact := range []string{"规范化", "fields", "数组", "对象映射", "kc read", "value", "kc files read"} {
		if result.Status != 0 || !strings.Contains(result.Stdout, fact) {
			t.Errorf("schema help omits the distinction needed to reuse a published Schema: %q", fact)
		}
	}
	put := Run([]string{"help", "writer", "put"})
	if put.Status != 0 || !strings.Contains(put.Stdout, "frontmatter") || !strings.Contains(put.Stdout, "kc help writer commit") {
		t.Error("writer put must distinguish its JSON value file from a directory object file")
	}
}
