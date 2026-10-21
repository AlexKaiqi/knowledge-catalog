package cli_test

import (
	"strconv"
	"strings"
	"testing"
)

type exampleSchemaSource struct {
	repository string
	names      []string
}

func TestQingheExampleDatasetsDeliverReusableSchemas(t *testing.T) {
	world := readExampleWorld(t)
	d := openExampleDeployment(t, world)
	d.publish(t, world, true)
	reader := "agent:example-learner"
	for _, tc := range []struct {
		dataset  string
		revision int
		sources  []exampleSchemaSource
	}{
		{"autumn-launch-kit", 3, []exampleSchemaSource{
			{"media", []string{"media-asset.definition", "media-asset.usage", "media-asset.vision"}},
			{"campaign", []string{"campaign.definition", "relation.canonical"}},
			{"product", []string{"product.definition"}},
		}},
		{"qinghe-analysis", 2, []exampleSchemaSource{
			{"table-meta", []string{"table.properties", "table.schema", "table.stats", "column.properties", "data-job.definition", "data-job.properties"}},
			{"sales-semantic", []string{"metric.definition", "metric.properties", "metric.caliber", "semantic-model.definition", "semantic-model.properties", "semantic-model.dimensions", "semantic-model.measures", "relation.canonical"}},
			{"product", []string{"product.definition", "concept.definition"}},
		}},
		{"store-oncall", 2, []exampleSchemaSource{
			{"store-ops", []string{"service.definition", "service.interface", "service.runbook", "service.health", "tool.definition", "alert.definition", "relation.canonical"}},
			{"campaign", []string{"campaign.definition"}},
		}},
	} {
		t.Run(tc.dataset, func(t *testing.T) {
			listing := asMap(t, body(t, d.as(t, reader, "files", "list", "--dataset", tc.dataset, "--revision", strconv.Itoa(tc.revision), "--path", "_schemas")))
			repositories := asMap(t, listing["repositories"])
			entries, _ := listing["entries"].([]any)
			paths := make(map[string]bool, len(entries))
			for _, entry := range entries {
				paths[asMap(t, entry)["path"].(string)] = true
			}
			wantCount := 0
			for _, source := range tc.sources {
				wantCount += len(source.names)
				for _, name := range source.names {
					t.Run(name, func(t *testing.T) {
						path := "_schemas/" + name + ".aspect.yaml"
						if !paths[path] {
							t.Errorf("public directory must list reusable Schema %s; got %#v", path, paths)
						}
						commit := repositories[source.repository].(string)
						d.checkDeliveredSchema(t, reader, tc.dataset, tc.revision, source.repository, commit, name)
					})
				}
			}
			if listing["exhausted"] != true || len(paths) != wantCount {
				t.Fatalf("Schema directory must contain the selected contracts once each: got %d want %d; %#v", len(paths), wantCount, listing)
			}
		})
	}
	// The later vision contract is not retroactively added to the earlier release.
	expectCode(t, d.as(t, reader, "files", "read", "--dataset", "autumn-launch-kit", "--revision", "2", "--path", "_schemas/media-asset.vision.aspect.yaml"), "OBJECT_REF_UNRESOLVED")
	// A neighboring source Schema is not needed by this Dataset and stays private.
	expectCode(t, d.as(t, reader, "files", "read", "--dataset", "autumn-launch-kit", "--path", "_schemas/concept.definition.aspect.yaml"), "OBJECT_REF_UNRESOLVED")
	expectCode(t, d.as(t, reader, "read", "urn:kc:object:schema/concept.definition", "--dataset", "autumn-launch-kit"), "OBJECT_REF_UNRESOLVED")
}

func (d *exampleDeployment) checkDeliveredSchema(t *testing.T, principal, dataset string, revision int, repository, commit, name string) {
	t.Helper()
	id := "schema/" + name
	reference := "urn:kc:object:" + id
	path := "_schemas/" + name + ".aspect.yaml"
	pattern := "record"
	if name == "semantic-model.dimensions" || name == "semantic-model.measures" {
		pattern = "keyed_collection"
	}
	t.Run("file", func(t *testing.T) {
		raw := d.readDelivered(t, principal, dataset, revision, path)
		if !strings.Contains(string(raw), "object_id: "+id) || !strings.Contains(string(raw), `"fields"`) {
			t.Fatalf("delivered Schema must retain its identity and definition: %s", raw)
		}
	})
	t.Run("object", func(t *testing.T) {
		result := asMap(t, body(t, d.as(t, principal, "read", reference, "--dataset", dataset, "--revision", strconv.Itoa(revision), "--commit", commit)))
		definition := asMap(t, result["value"])
		fields := asMap(t, definition["fields"])
		if result["reference"] != reference || definition["pattern"] != pattern || len(fields) == 0 {
			t.Fatalf("public read must return the reusable Schema body: %#v", result)
		}
	})
	t.Run("describe", func(t *testing.T) {
		reports, ok := body(t, d.as(t, principal, "schema", "describe", "--dataset", dataset, "--revision", strconv.Itoa(revision), "--object", id)).([]any)
		if !ok || len(reports) != 1 {
			t.Fatalf("Dataset describe must select one Schema source: %#v", reports)
		}
		result := asMap(t, reports[0])
		schemas, _ := result["schemas"].([]any)
		if result["repository"] != repository || result["commit"] != commit || len(schemas) != 1 {
			t.Fatalf("public describe must identify one selected Schema: %#v", result)
		}
		schema := asMap(t, schemas[0])
		definition := asMap(t, schema["definition"])
		fields, _ := definition["fields"].([]any)
		if schema["objectId"] != id || definition["pattern"] != pattern || len(fields) == 0 {
			t.Fatalf("public describe must return structural constraints: %#v", result)
		}
	})
	// Public examples grant consumption of the selected files, not their source repos.
	expectCode(t, d.as(t, principal, "read", reference, "--repo", repository), "FORBIDDEN")
	expectCode(t, d.as(t, principal, "schema", "describe", "--repo", repository, "--object", id), "FORBIDDEN")
	expectCode(t, d.as(t, principal, "files", "read", "--repo", repository, "--commit", commit, "--path", path), "FORBIDDEN")
}
