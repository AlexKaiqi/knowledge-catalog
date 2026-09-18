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
		{"schema/table.properties", "_schemas/table.properties.aspect.yaml"},
		{"schema/runbook.body", "_schemas/runbook.body.aspect.yaml"},
		{"schema/semantic-model.definition", "_schemas/semantic-model.definition.aspect.yaml"},
		{"schema/meta/schema-definition/v1", "_schemas/schema-definition.v1.aspect.yaml"},
		{"schema/core/resource-descriptor/v1", "_schemas/resource-descriptor.v1.aspect.yaml"},
		{"schema/core/relation/v1", "_schemas/relation.v1.aspect.yaml"},
		{"schema/core/readme/v1", "_schemas/readme.v1.aspect.yaml"},
		{"schema/table/structure/v1", "_schemas/structure.v1.aspect.yaml"},
		{"schema/policy", "_schemas/policy.aspect.yaml"},
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
	readme := DefaultPath(knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "payments", AspectName: knowledge.ReadmeAspect}, string(knowledge.CoreReadmeSchemaV1))
	if readme != knowledge.RepositoryReadmePath {
		t.Fatalf("readme: got %s", readme)
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
		"schema/core/readme/v1":                    "readmes",
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
		if got != "_schemas/metric.definition.aspect.yaml" {
			t.Fatalf("rel %s: got %s", rel, got)
		}
	}
}

func TestPathHintForIngestPlacesReadmeAtRepositoryRoot(t *testing.T) {
	address := knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "payments", AspectName: knowledge.ReadmeAspect}
	got := PathHintForIngest(address, "notes/about.md", "notes/about.md")
	if got != knowledge.RepositoryReadmePath {
		t.Fatalf("got %s", got)
	}
}
