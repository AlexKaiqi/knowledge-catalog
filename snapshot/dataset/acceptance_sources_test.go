package dataset_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kc/kernel"
	"kc/snapshot"
	catalog "kc/snapshot/dataset"
)

func TestAcceptanceDatasetSourcesDeliverSchemasFromSelectedSubtrees(t *testing.T) {
	path := filepath.Join("..", "..", ".data", "scenes", "service-initialized", "named-repositories-created", "qinghe-object-published", "_probes", "acceptance", "probes.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var probes []struct {
		ID    string `json:"id"`
		Steps []struct {
			KC []string `json:"kc"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(content, &probes); err != nil {
		t.Fatal(err)
	}
	selected := map[string]bool{
		"accept-live-dataset-read-consume-only":     false,
		"accept-live-descriptor-deleted-at-head":    false,
		"accept-live-pinned-dataset-unit-changed":   false,
		"accept-cross-descriptor-repo-not-selected": false,
		"liveaccess-mixed-values":                   false,
		"accept-search-pinned-dataset-frozen":       false,
	}
	for _, probe := range probes {
		if _, ok := selected[probe.ID]; !ok {
			continue
		}
		for _, step := range probe.Steps {
			if len(step.KC) < 2 || step.KC[0] != "dataset" || step.KC[1] != "define" {
				continue
			}
			selected[probe.ID] = true
			t.Run(probe.ID, func(t *testing.T) {
				var mappings, files []string
				for i, arg := range step.KC[:len(step.KC)-1] {
					if arg == "--source" || arg == "--file-source" {
						mapping := strings.ReplaceAll(strings.ReplaceAll(step.KC[i+1], "$branch", snapshot.DefaultRef), "$prefix", "fixture-")
						if arg == "--source" {
							mappings = append(mappings, "kr://acceptance/"+mapping)
						} else {
							files = append(files, "kr://acceptance/"+mapping)
						}
					}
				}
				testAcceptanceSourceDelivery(t, mappings, files)
			})
		}
	}
	for id, found := range selected {
		if !found {
			t.Errorf("%s has no Dataset definition", id)
		}
	}
}

func testAcceptanceSourceDelivery(t *testing.T, mappings, files []string) {
	t.Helper()
	sources, err := catalog.ParseSourceMappings(mappings)
	if err != nil {
		t.Fatal(err)
	}
	fileSources, err := catalog.ParseFileMappings(files)
	if err != nil {
		t.Fatal(err)
	}
	sources = append(sources, fileSources...)
	tables := &publicationSource{catalogMem: newCatalogMem("kr://acceptance/table-meta")}
	semantic := &publicationSource{catalogMem: newCatalogMem("kr://acceptance/sales-semantic")}
	for _, store := range []*publicationSource{tables, semantic} {
		writePublicationFiles(t, store.catalogMem, map[string]string{
			"_schemas/definition.yaml": string(store.ID()), "_schemas/common.yaml": "shared-schema",
			"_schemas/fixture-metric.properties.aspect.yaml": string(store.ID()), "_schemas/fixture-metric.definition.aspect.yaml": string(store.ID()),
			"tables/one.yaml":  "table",
			"metrics/one.yaml": "metric", "private/hidden.yaml": "unselected",
		})
	}
	maintenance := &publicationSource{catalogMem: newCatalogMem("kr://acceptance/sales-analysis")}
	writePublicationFiles(t, maintenance.catalogMem, map[string]string{"indexes/member.yaml": "declaration", "private/hidden.yaml": "unselected"})
	service, _ := publicationService(t, tables, semantic, maintenance)
	def, err := service.DefineDataset("live-values", 1, sources)
	if err != nil {
		t.Fatal("real portable mappings do not publish together", err)
	}
	pin, err := service.ResolveDataset("live-values")
	if err != nil {
		t.Fatal(err)
	}
	schemaRepositories := map[kernel.RepositoryID]bool{}
	semanticSelected := false
	commonHolders := 0
	for _, source := range sources {
		semanticSelected = semanticSelected || source.Repository == semantic.ID()
		if source.File != "" {
			if filepath.Dir(source.File) != "_schemas" || !strings.HasPrefix(source.Target, "_schemas/") {
				t.Fatalf("single Schema delivery broadened or lost its target: %#v", source)
			}
			read, err := service.ReadVirtualFileAt(def, pin, source.Target)
			if err != nil || string(read.Content) != string(source.Repository) || read.Repository != source.Repository {
				t.Fatalf("single Schema delivery changed its source: %#v %v", read, err)
			}
			schemaRepositories[source.Repository] = true
			continue
		}
		if source.Path == nil || source.SubPath == "" || source.Repository != tables.ID() && source.Repository != semantic.ID() && source.Repository != maintenance.ID() {
			t.Fatalf("selected fragments were broadened: %#v", source)
		}
		if !strings.HasPrefix(*source.Path, "_schemas/") {
			continue
		}
		if _, err := service.ReadVirtualFileAt(def, pin, *source.Path+"/common.yaml"); err == nil {
			commonHolders++
		}
		if source.SubPath != "_schemas" {
			t.Fatalf("Schema source selects %q instead of the original _schemas directory", source.SubPath)
		}
		read, err := service.ReadVirtualFileAt(def, pin, *source.Path+"/definition.yaml")
		if err != nil || string(read.Content) != string(source.Repository) || read.Repository != source.Repository {
			t.Fatalf("Schema delivery did not resolve its original source file: %#v %v", read, err)
		}
		route, err := catalog.RouteDatasetFile(pin.Items, *source.Path+"/definition.yaml")
		if err != nil || route.File != "_schemas/definition.yaml" {
			t.Fatalf("Schema delivery did not retain the original source path: %#v %v", route, err)
		}
		schemaRepositories[source.Repository] = true
	}
	if commonHolders != 1 {
		t.Fatalf("Dataset selects %d copies of the common Schema; one holder must be selected", commonHolders)
	}
	if !schemaRepositories[tables.ID()] || semanticSelected && !schemaRepositories[semantic.ID()] {
		t.Fatal("a selected object repository lost its required Schema fragment")
	}
	if _, err := catalog.RouteDatasetFile(pin.Items, "private/hidden.yaml"); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatal("unselected adjacent files entered the Dataset scope", err)
	}
}
