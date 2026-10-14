package semanticview_test

import (
	"strings"
	"testing"

	objectmodel "kc/object"
	"kc/object/semanticview"
)

func TestRenderUsesActualAssembledYAMLAndPreservesCoordinates(t *testing.T) {
	value := objectmodel.ObjectValue{
		Repository: "kr://acme/semantic", Commit: "c1",
		Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "metric/gmv"},
		Value: map[string]any{
			"definition": map[string]any{"name": "Gross Merchandise Value", "expression": "SUM(price)"},
			"properties": map[string]any{"unit": "CNY"},
		},
		Declarations: []objectmodel.UnitDeclaration{
			{Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "metric/gmv", AspectName: "definition"}, SchemaRef: "schema/metric/definition/v1"},
			{Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "metric/gmv", AspectName: "properties"}, SchemaRef: "schema/metric/properties/v1"},
		},
	}
	raw, err := semanticview.Render(value)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		"object_id: metric/gmv", "repository: kr://acme/semantic", "commit: c1",
		"definition:", "expression: SUM(price)", "properties:", "unit: CNY",
		"definition: schema/metric/definition/v1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("semantic YAML missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "object_id: metric/gmv\naspect_name:") || strings.Contains(text, "---\nobject_id") {
		t.Fatalf("consumer view leaked canonical unit envelope:\n%s", text)
	}
	if got := semanticview.Path(value, "Metric"); !strings.HasPrefix(got, "metrics/gross-merchandise-value--") || !strings.HasSuffix(got, ".yaml") {
		t.Fatalf("path = %s", got)
	}
}
