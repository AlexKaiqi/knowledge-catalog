package cli_test

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	objectmodel "kc/object"
	"kc/object/format"
)

type modelGuideCoordinate struct {
	dataset  string
	revision int
	path     string
}

type modelGuideExamples map[modelGuideCoordinate]*format.Unit

// The guide supplies the coordinates; the public download supplies the evidence.
// Source materials are used only by the existing publication fixture.
func TestPublicModelGuideExamplesSupportDecisions(t *testing.T) {
	world := readExampleWorld(t)
	d := openExampleDeployment(t, world)
	d.publish(t, world, true)
	const reader = "agent:model-learner"
	guide := asMap(t, body(t, d.as(t, reader, "read",
		"urn:kc:object:kc/guide/model-from-materials#/guide", "--repo", "kr://kc/system")))
	text := asMap(t, guide["value"])["body"].(string)
	examples := downloadModelGuideExamples(t, d, reader, text)

	t.Run("one-concept-explains-different-contexts", func(t *testing.T) {
		concept := examples.unit(t, "qinghe-analysis", 2, "concepts/order/definition.yaml")
		requireModelAspect(t, concept, "concept/qinghe.order", "definition")
		value := asMap(t, concept.Value)
		contexts, ok := value["contexts"].([]any)
		if !ok || len(contexts) != 3 || value["name"] != "订单" {
			t.Fatalf("the example must explain three contexts in one concept definition: %#v", value)
		}
		meanings := map[string]string{}
		for _, raw := range contexts {
			context := asMap(t, raw)
			name, _ := context["context"].(string)
			meaning, _ := context["meaning"].(string)
			meanings[name] = meaning
		}
		for context, referent := range map[string]string{
			"门店收银": "小票",
			"数仓":   "table/shop.orders",
			"下单服务": "service/qinghe.order-service",
		} {
			if !strings.Contains(meanings[context], referent) {
				t.Errorf("%s must distinguish its referent %s: %#v", context, referent, meanings)
			}
		}
	})
	t.Run("product-and-table-have-separate-identities", func(t *testing.T) {
		product := examples.unit(t, "qinghe-analysis", 2, "products/osmanthus-oolong/definition.yaml")
		table := examples.unit(t, "qinghe-analysis", 2, "tables/products.properties.yaml")
		requireModelAspect(t, product, "product/qinghe.osmanthus-oolong", "definition")
		requireModelAspect(t, table, "table/shop.products", "properties")
		relation := examples.unit(t, "qinghe-analysis", 2, "relations-product/recorded-in-osmanthus-oolong-products.yaml")
		requireModelRelation(t, relation, "rel/recorded-in/osmanthus-oolong-products", "recorded-in",
			map[string]objectmodel.ObjectID{"product": product.ObjectID, "table": table.ObjectID})
	})
	t.Run("members-have-keys-ordinary-fields-stay-in-the-aspect", func(t *testing.T) {
		definition := examples.unit(t, "qinghe-analysis", 2, "semantic-models/shop.sales/definition.yaml")
		requireModelAspect(t, definition, "semantic-model/shop.sales", "definition")
		value := asMap(t, definition.Value)
		if value["baseTableRef"] != "table/shop.sales_mart" || value["defaultTimeDimensionKey"] != "order-date" {
			t.Fatalf("ordinary fields must remain values in the definition aspect: %#v", value)
		}
		for _, member := range []struct{ aspect, key string }{{"dimensions", "category"}, {"measures", "gmv"}} {
			path := "semantic-models/shop.sales/" + member.aspect + "/" + member.key + ".yaml"
			unit := examples.unit(t, "qinghe-analysis", 2, path)
			want := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: definition.ObjectID,
				AspectName: member.aspect, MemberKey: member.key}
			if unit.Address != want || asMap(t, unit.Value)["columnRef"] == nil {
				t.Fatalf("member must have its own stable address and field reference: %#v", unit)
			}
		}
	})
	t.Run("service-aspects-differ-from-a-relation-to-a-campaign", func(t *testing.T) {
		for _, aspect := range []string{"definition", "interface", "runbook"} {
			unit := examples.unit(t, "store-oncall", 2, "services/order-service/"+aspect+".yaml")
			requireModelAspect(t, unit, "service/qinghe.order-service", aspect)
		}
		runbook := examples.unit(t, "store-oncall", 2, "services/order-service/runbook.yaml")
		if asMap(t, runbook.Value)["revision"] != "2" {
			t.Fatalf("the fixed Dataset revision must deliver the updated runbook: %#v", runbook.Value)
		}
		campaign := examples.unit(t, "store-oncall", 2, "campaigns/autumn-2026/definition.yaml")
		requireModelAspect(t, campaign, "campaign/qinghe.autumn-2026", "definition")
		relation := examples.unit(t, "store-oncall", 2, "relations-store-ops/affected-by-order-service-autumn-2026.yaml")
		requireModelRelation(t, relation, "rel/affected-by/order-service-autumn-2026", "affected-by",
			map[string]objectmodel.ObjectID{"service": runbook.ObjectID, "campaign": campaign.ObjectID})
	})
	t.Run("reuse-a-contract-and-add-a-different-contract", func(t *testing.T) {
		checkModelGuideSchemaExamples(t, examples)
	})
}

func downloadModelGuideExamples(t *testing.T, d *exampleDeployment, principal, guide string) modelGuideExamples {
	t.Helper()
	examples := modelGuideExamples{}
	for _, line := range strings.Split(guide, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "kc files read") {
			continue
		}
		args := strings.Fields(line)
		if len(args) != 9 || args[2] != "read" || args[3] != "--dataset" || args[5] != "--revision" || args[7] != "--path" {
			t.Fatalf("guide downloads must name an explicit Dataset, fixed revision and path: %s", line)
		}
		revision, err := strconv.Atoi(args[6])
		if err != nil || revision < 1 {
			t.Fatalf("guide download must use a positive fixed revision: %s", line)
		}
		coordinate := modelGuideCoordinate{args[4], revision, args[8]}
		raw := d.readDelivered(t, principal, coordinate.dataset, coordinate.revision, coordinate.path)
		unit := format.ParseAt(string(raw), coordinate.path)
		if unit == nil || len(asMap(t, unit.Value)) == 0 {
			t.Fatalf("guide example must deliver an object identity and structured body: %s", line)
		}
		examples[coordinate] = unit
		t.Logf("read %s revision %d path %s (%d bytes)", coordinate.dataset, coordinate.revision, coordinate.path, len(raw))
	}
	if len(examples) == 0 {
		t.Fatal("the published model guide has no executable fixed-revision examples")
	}
	return examples
}

func (examples modelGuideExamples) unit(t *testing.T, dataset string, revision int, path string) *format.Unit {
	t.Helper()
	unit, ok := examples[modelGuideCoordinate{dataset, revision, path}]
	if !ok {
		t.Fatalf("the published guide must provide its decision evidence: %s revision %d path %s", dataset, revision, path)
	}
	return unit
}

func requireModelAspect(t *testing.T, unit *format.Unit, objectID objectmodel.ObjectID, aspect string) {
	t.Helper()
	want := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: objectID, AspectName: aspect}
	if unit.ObjectID != objectID || unit.Address != want {
		t.Fatalf("expected aspect address %#v, got %#v", want, unit)
	}
}

func requireModelRelation(t *testing.T, unit *format.Unit, id objectmodel.ObjectID, relationType string, endpoints map[string]objectmodel.ObjectID) {
	t.Helper()
	relation, err := objectmodel.DecodeRelation(unit.Address, unit.Value)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]objectmodel.ObjectID{}
	for _, endpoint := range relation.Endpoints {
		got[endpoint.Role] = endpoint.Object
	}
	if relation.RelationID != id || relation.RelationType != relationType || relation.Direction != objectmodel.RelationDirected ||
		len(relation.Endpoints) != len(endpoints) || !reflect.DeepEqual(got, endpoints) {
		t.Fatalf("relation must connect the independently delivered objects: %#v; want %s %s %#v", relation, id, relationType, endpoints)
	}
}

func checkModelGuideSchemaExamples(t *testing.T, examples modelGuideExamples) {
	t.Helper()
	tableSchema := examples.unit(t, "qinghe-analysis", 2, "_schemas/table.properties.aspect.yaml")
	tableContract := requireModelSchemaFields(t, tableSchema, "properties",
		map[string]string{"name": "string", "qualifiedName": "string", "nativeId": "string"})
	for _, name := range []string{"orders", "products"} {
		table := examples.unit(t, "qinghe-analysis", 2, "tables/"+name+".properties.yaml")
		requireModelAspect(t, table, objectmodel.ObjectID("table/shop."+name), "properties")
		if table.SchemaRef != "schema/table.properties" || table.SchemaRef != string(tableSchema.ObjectID) {
			t.Fatalf("different tables must reuse the delivered contract: %#v", table)
		}
		if asMap(t, table.Value)["qualifiedName"] != "shop."+name {
			t.Fatalf("a reused Schema must still describe the individual table: %#v", table.Value)
		}
		if err := objectmodel.ValidateSchemaInstance(table.Address, table.Value, tableContract); err != nil {
			t.Fatalf("the shared contract must explain table %s: %v", name, err)
		}
	}

	definition := examples.unit(t, "autumn-launch-kit", 3, "assets/autumn-launch/cut-15s.definition.yaml")
	vision := examples.unit(t, "autumn-launch-kit", 3, "assets/autumn-launch/cut-15s.vision.yaml")
	requireModelAspect(t, definition, "media-asset/autumn-launch.cut-15s", "definition")
	requireModelAspect(t, vision, definition.ObjectID, "vision")
	definitionSchema := examples.unit(t, "autumn-launch-kit", 3, "_schemas/media-asset.definition.aspect.yaml")
	visionSchema := examples.unit(t, "autumn-launch-kit", 3, "_schemas/media-asset.vision.aspect.yaml")
	if definition.SchemaRef != "schema/media-asset.definition" || vision.SchemaRef != "schema/media-asset.vision" ||
		string(definitionSchema.ObjectID) != definition.SchemaRef || string(visionSchema.ObjectID) != vision.SchemaRef {
		t.Fatalf("the two media aspects must use their distinct delivered contracts: %#v %#v", definition, vision)
	}
	definitionContract := requireModelSchemaFields(t, definitionSchema, "definition",
		map[string]string{"name": "string", "kind": "string", "file": "string", "format": "string"})
	visionContract := requireModelSchemaFields(t, visionSchema, "vision",
		map[string]string{"labels": "array", "producedBy": "string"})
	if err := objectmodel.ValidateSchemaInstance(definition.Address, definition.Value, definitionContract); err != nil {
		t.Fatalf("the definition contract must explain the file description: %v", err)
	}
	if err := objectmodel.ValidateSchemaInstance(vision.Address, vision.Value, visionContract); err != nil {
		t.Fatalf("the vision contract must explain the observations: %v", err)
	}
	visionValue := asMap(t, vision.Value)
	labels, ok := visionValue["labels"].([]any)
	if !ok || len(labels) == 0 || visionValue["producedBy"] == nil || visionValue["file"] != nil {
		t.Fatalf("vision must deliver observations, not the asset's file description: %#v", visionValue)
	}
	if asMap(t, definition.Value)["file"] != "assets/autumn-launch/cut-15s.mp4" {
		t.Fatalf("definition must retain the asset's file description: %#v", definition.Value)
	}
}

func requireModelSchemaFields(t *testing.T, schema *format.Unit, aspect string, required map[string]string) objectmodel.SchemaDefinition {
	t.Helper()
	definition, err := objectmodel.ParseSchemaDefinition(schema.ObjectID, schema.Value)
	if err != nil {
		t.Fatal(err)
	}
	if definition.Pattern != "record" || definition.Aspect != aspect {
		t.Fatalf("expected record contract for %s, got %#v", aspect, definition)
	}
	fields := map[string]objectmodel.SchemaFieldDefinition{}
	for _, field := range definition.Fields {
		fields[field.Path] = field
	}
	for name, fieldType := range required {
		if field := fields[name]; !field.Required || field.Type != fieldType {
			t.Errorf("contract %s must require %s as %s: %#v", schema.ObjectID, name, fieldType, fields)
		}
	}
	return definition
}
