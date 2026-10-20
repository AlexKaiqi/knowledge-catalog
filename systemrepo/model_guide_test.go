package systemrepo_test

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	objectmodel "kc/object"
	"kc/systemrepo"
)

const modelBoundaryHeading = "一个对象，还是相关的两个对象"
const modelGuidePath = "guides/model-from-materials/guide.md"

func TestModelGuideBoundaryMatchesDesignAuthority(t *testing.T) {
	design := modelGuideDesign(t)
	guide := modelGuideBody(t, modelGuideOperation(t, modelGuidePath))
	if err := modelBoundaryAgreement(design, guide); err != nil {
		t.Fatal(err)
	}
}

// Compare only the shared criterion. Examples and the rest of the guide may
// evolve independently without requiring a snapshot of the whole publication.
func modelBoundaryAgreement(design, guide string) error {
	authority, found := markdownSection(design, modelBoundaryHeading)
	if !found {
		return fmt.Errorf("design lacks the object-boundary section")
	}
	published, found := markdownSection(guide, modelBoundaryHeading)
	if !found {
		return fmt.Errorf("published modeling guide lacks the object-boundary section")
	}
	heading := "## " + modelBoundaryHeading
	if strings.TrimSpace(strings.TrimPrefix(authority, heading)) == "" ||
		strings.TrimSpace(strings.TrimPrefix(published, heading)) == "" {
		return fmt.Errorf("object-boundary criteria must not be empty")
	}
	if strings.TrimSpace(authority) != strings.TrimSpace(published) {
		return fmt.Errorf("published object-boundary criteria differ from doc/design/object.md")
	}
	return nil
}

func TestModelGuideBoundaryGuardRejectsDivergence(t *testing.T) {
	design := modelGuideDesign(t)
	guide := modelGuideBody(t, modelGuideOperation(t, modelGuidePath))
	if err := modelBoundaryAgreement(design, guide); err != nil {
		t.Fatalf("unchanged publication must pass: %v", err)
	}
	diverge := func(body string) string {
		changed := strings.Replace(body, "两者是否一一对应", "两者名称是否相同", 1)
		if changed == body {
			t.Fatal("counterexample no longer changes an actual published criterion")
		}
		return changed
	}
	empty := "## " + modelBoundaryHeading + "\n"
	for _, example := range []struct {
		name, design, guide string
	}{
		{"guide changes criterion", design, diverge(guide)},
		{"authority changes criterion", diverge(design), guide},
		{"both sections empty", empty, empty},
	} {
		t.Run(example.name, func(t *testing.T) {
			if err := modelBoundaryAgreement(example.design, example.guide); err == nil {
				t.Fatal("guard accepted divergent or empty object-boundary criteria")
			}
		})
	}
}

func TestModelGuidePublicationRetainsIdentityAndNavigationRelations(t *testing.T) {
	guide := modelGuideOperation(t, modelGuidePath)
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "kc/guide/model-from-materials", AspectName: "guide"}
	if guide.Op != objectmodel.OpPut || guide.Address != address || guide.SchemaRef != "schema/system/usage-guide/v1" {
		t.Fatalf("modeling classification changed the published guide contract: %#v", guide)
	}
	if !strings.HasPrefix(modelGuideBody(t, guide), "# 从资料到模型（建模方法）\n") {
		t.Fatal("published guide title must identify it as a modeling method")
	}
	for _, capability := range []string{"publish-object", "schemas"} {
		t.Run(capability, func(t *testing.T) {
			name := "model-from-materials.uses." + capability
			operation := modelGuideOperation(t, "relations/"+name+".yaml")
			id := objectmodel.ObjectID("kc/relation/" + name)
			if operation.Op != objectmodel.OpPut ||
				operation.Address != (objectmodel.Address{Kind: objectmodel.KindRelation, ObjectID: id}) ||
				operation.SchemaRef != "schema/core/relation/v1" {
				t.Fatalf("modeling guide relation changed its contract: %#v", operation)
			}
			relation, err := objectmodel.DecodeRelation(operation.Address, operation.Value)
			if err != nil {
				t.Fatal(err)
			}
			want := objectmodel.CanonicalRelation{
				RelationID: id, RelationType: "uses", Direction: objectmodel.RelationDirected,
				Endpoints: []objectmodel.RelationEndpoint{
					{Role: "from", Object: address.ObjectID},
					{Role: "to", Object: objectmodel.ObjectID("kc/capability/" + capability)},
				},
			}
			if !reflect.DeepEqual(relation, want) {
				t.Fatalf("modeling guide relation changed: got %#v, want %#v", relation, want)
			}
		})
	}
}

func TestModelGuideReadmeSeparatesMethodFromOperations(t *testing.T) {
	readme := modelGuideBody(t, modelGuideOperation(t, "README.md"))
	guides, found := markdownSection(readme, "使用指南")
	if !found {
		t.Fatal("published README lacks its guide navigation")
	}
	_, categories, found := strings.Cut(guides, "### 建模方法\n")
	if !found {
		t.Fatal("published README does not identify modeling methods")
	}
	methods, operations, found := strings.Cut(categories, "\n### 操作指南\n")
	if !found {
		t.Fatal("published README must place modeling methods before operational guides")
	}
	entries := regexp.MustCompile(`(?m)^- \[[^\]]+\]\(([^)]+)\)`)
	methodEntries := entries.FindAllStringSubmatch(methods, -1)
	if len(methodEntries) != 1 || methodEntries[0][1] != modelGuidePath {
		t.Fatalf("method navigation must contain only the modeling guide: %v", methodEntries)
	}
	if strings.Contains(operations, modelGuidePath) ||
		!strings.Contains(operations, "(guides/first-publication/guide.md)") {
		t.Fatal("operational navigation must contain first-publication separately from the modeling method")
	}
	sequence := regexp.MustCompile(`先[^。\n]*再[^。\n]*\(guides/first-publication/guide\.md\)`)
	if !sequence.MatchString(methods) {
		t.Fatal("method navigation must tell readers to judge the model before entering first-publication")
	}
}

func TestObjectConceptPointsToPublishedModelGuide(t *testing.T) {
	operation := modelGuideOperation(t, "concepts/object/definition.md")
	boundary, found := markdownSection(modelGuideBody(t, operation), "划分对象边界")
	if !found {
		t.Fatal("object concept lacks its boundary guidance")
	}
	link := "../../" + modelGuidePath
	if !strings.Contains(boundary, "]("+link+")") {
		t.Fatal("object concept must point to the published modeling guide for boundary criteria")
	}
	target := path.Clean(path.Join(path.Dir(operation.PathHint), link))
	if modelGuideOperation(t, target).Address.ObjectID != "kc/guide/model-from-materials" {
		t.Fatal("object concept boundary link does not resolve to the modeling guide in this publication")
	}
}

func modelGuideOperation(t *testing.T, publicationPath string) objectmodel.Operation {
	t.Helper()
	for _, operation := range systemrepo.SystemPublicationOperations() {
		if operation.PathHint == publicationPath {
			return operation
		}
	}
	t.Fatalf("required published file %s is missing", publicationPath)
	return objectmodel.Operation{}
}

func modelGuideBody(t *testing.T, operation objectmodel.Operation) string {
	t.Helper()
	value, ok := operation.Value.(map[string]any)
	if !ok {
		t.Fatalf("%s has no object body", operation.PathHint)
	}
	body, ok := value["body"].(string)
	if !ok || strings.TrimSpace(body) == "" {
		t.Fatalf("%s has no nonempty Markdown body", operation.PathHint)
	}
	return body
}

func modelGuideDesign(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(sourceFile), "..", "doc", "design", "object.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
