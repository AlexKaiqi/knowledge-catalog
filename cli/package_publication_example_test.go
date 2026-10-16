package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	objectmodel "kc/object"
)

// Execute the authored package examples through the product Server; accepting
// argv alone would not prove that the Schema and its index inputs are writable.
func TestWriterPackageExamplesPublishStructureAndIndexSeparately(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "object", "writer", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	world := exampleWorld{}
	world.Repositories = append(world.Repositories, struct {
		ID        string `json:"id"`
		Provision string `json:"provision"`
	}{"kr://acme/public/core", "managed"})
	d := openExampleDeployment(t, world)
	text := strings.ReplaceAll(string(raw), "\\\n", " ")
	var commit string
	count := 0
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "go run ./cmd/kc -- writer put ") {
			continue
		}
		args, err := splitSceneArgs(strings.TrimPrefix(line, "go run ./cmd/kc -- "))
		if err != nil {
			t.Fatal(err)
		}
		receipt := asMap(t, body(t, d.as(t, d.operator, args...)))
		commit = asMap(t, receipt["result"])["newCommit"].(string)
		if commit == "" {
			t.Fatalf("package example produced no accepted commit: %#v", receipt)
		}
		count++
	}
	if count != 2 {
		t.Fatalf("package guide must publish both Schema and index member; executed %d commands", count)
	}
	schema := asMap(t, body(t, d.as(t, d.operator, "read", "urn:kc:object:schema/service.health", "--repo", "kr://acme/public/core", "--commit", commit)))
	definition, err := objectmodel.ParseSchemaDefinition("schema/service.health", schema["value"])
	if err != nil || definition.Aspect != "health" || len(definition.Fields) != 1 || definition.Fields[0].Path != "status" {
		t.Fatalf("published example must retain its structural field: %#v %v", definition, err)
	}
	index := asMap(t, body(t, d.as(t, d.operator, "read", "urn:kc:object:index/service-search#/index/status", "--repo", "kr://acme/public/core", "--commit", commit)))
	address := objectmodel.Address{Kind: objectmodel.KindMember, ObjectID: "index/service-search", AspectName: "index", MemberKey: "status"}
	declaration, err := objectmodel.ParseIndexDeclaration(address, index["value"])
	if err != nil || len(declaration.Inputs) != 1 || declaration.Inputs[0].Locator.Schema.Object != "schema/service.health" || declaration.Inputs[0].Locator.Path != "status" {
		t.Fatalf("published example must reference the actual Schema field: %#v %v", declaration, err)
	}
}
