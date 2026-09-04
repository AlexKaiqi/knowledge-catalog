package repofile

import (
	"testing"

	"kc/knowledge"
)

func TestDefaultPathPlacesSchemasUnderSchemasDirectory(t *testing.T) {
	cases := []struct {
		objectID knowledge.ObjectID
		want     string
	}{
		{"schema/table.properties", "schemas/table.properties.aspect.yaml"},
		{"schema/runbook.body", "schemas/runbook.body.aspect.yaml"},
		{"schema/semantic-model.definition", "schemas/semantic-model.definition.aspect.yaml"},
		{"schema/meta/schema-definition/v1", "schemas/schema-definition.v1.aspect.yaml"},
		{"schema/core/resource-descriptor/v1", "schemas/resource-descriptor.v1.aspect.yaml"},
		{"schema/core/relation/v1", "schemas/relation.v1.aspect.yaml"},
		{"schema/core/source-profile/v1", "schemas/source-profile.v1.aspect.yaml"},
		{"schema/table/structure/v1", "schemas/structure.v1.aspect.yaml"},
		{"schema/policy", "schemas/policy.aspect.yaml"},
	}
	for _, tc := range cases {
		got := DefaultPath(knowledge.Address{Kind: knowledge.KindEntity, ObjectID: tc.objectID}, "")
		if got != tc.want {
			t.Fatalf("%s: got %s want %s", tc.objectID, got, tc.want)
		}
	}
}

func TestDefaultPathPlacesInstancesUnderTypeDirectories(t *testing.T) {
	got := DefaultPath(knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "table/lineitem", AspectName: "properties"}, "")
	if got != "table/lineitem/properties.json" {
		t.Fatalf("without schema_ref: got %s", got)
	}
	typed := DefaultPath(knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "dw-metric-1", AspectName: "properties"}, "schema/metric.properties")
	if typed != "metrics/dw-metric-1/properties.json" {
		t.Fatalf("with schema_ref: got %s", typed)
	}
	resource := DefaultPath(knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "resource/mysql-tpch-sql"}, "schema/core/resource-descriptor/v1")
	if resource != "resources/resource/mysql-tpch-sql.json" {
		t.Fatalf("resource: got %s", resource)
	}
	profile := DefaultPath(knowledge.Address{Kind: knowledge.KindEntity, ObjectID: knowledge.SourceProfileObjectID}, string(knowledge.CoreSourceProfileSchemaV1))
	if profile != "source-profiles/core/source-profile.json" {
		t.Fatalf("source profile: got %s", profile)
	}
}

func TestInstanceTypeDir(t *testing.T) {
	cases := map[string]string{
		"schema/metric.definition":                 "metrics",
		"schema/semantic-model.properties":         "semantic-models",
		"schema/table.properties":                  "tables",
		"schema/database-schema.properties":        "database-schemas",
		"schema/data-job.definition":               "data-jobs",
		"schema/data-platform-instance.properties": "data-platform-instances",
		"schema/relation.canonical":                "relations",
		"schema/core/resource-descriptor/v1":       "resources",
		"schema/core/source-profile/v1":            "source-profiles",
		"":                                         "",
	}
	for ref, want := range cases {
		if got := InstanceTypeDir(ref); got != want {
			t.Fatalf("%s: got %s want %s", ref, got, want)
		}
	}
}

func TestPathHintForIngestPutsSchemaDraftsInOneDirectory(t *testing.T) {
	address := knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/metric.definition"}
	for _, rel := range []string{"metric.definition.aspect.yaml", "schemas/physical/metric.definition.aspect.yaml"} {
		got := PathHintForIngest(address, "", rel)
		if got != "schemas/metric.definition.aspect.yaml" {
			t.Fatalf("rel %s: got %s", rel, got)
		}
	}
}
