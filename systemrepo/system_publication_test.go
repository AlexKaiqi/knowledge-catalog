package systemrepo_test

import (
	"kc/internal/testkit"
	interfaces "kc/liveaccess/handler"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
	"kc/systemrepo"
)

func TestSystemPublicationMatchesCanonicalFilesAndSchemas(t *testing.T) {
	_, sourceFile, _, _ := runtime.Caller(0)
	sourceDir := filepath.Join(filepath.Dir(sourceFile), "content")
	repo := testkit.MakeSystemRepository(t)
	commit, err := repo.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	operations := systemrepo.SystemPublicationOperations()
	schemas := map[objectmodel.ObjectID]objectmodel.SchemaDefinition{}
	for _, operation := range systemrepo.SystemSchemaOperations() {
		definition, err := objectmodel.ParseSchemaDefinition(operation.Address.ObjectID, operation.Value)
		if err != nil {
			t.Fatal(err)
		}
		schemas[operation.Address.ObjectID] = definition
	}
	seen := map[string]bool{}
	byObject := map[objectmodel.ObjectID][]repofile.Unit{}
	for _, operation := range operations {
		key := objectmodel.AddressKey(operation.Address)
		if seen[key] {
			t.Fatalf("duplicate publication Address %s", key)
		}
		seen[key] = true
		raw, err := repo.ReadFile(operation.PathHint, commit)
		if err != nil {
			t.Fatal(err)
		}
		sourcePath := operation.PathHint
		if sourcePath == "README.md" {
			sourcePath = "readme.md"
		}
		authored := filepath.Join(sourceDir, sourcePath)
		if sourcePath == "_schemas/schema-definition.v1.aspect.yaml" || sourcePath == "_schemas/readme.v1.aspect.yaml" {
			authored = filepath.Join(filepath.Dir(sourceDir), "..", "object", "trusted", filepath.Base(sourcePath))
		}
		if sourcePath == "object-interfaces/live-access/definition.yaml" || sourcePath == "_schemas/implements.live-access.v1.aspect.yaml" || sourcePath == "_schemas/resource-descriptor.v1.aspect.yaml" {
			authored = filepath.Join(filepath.Dir(sourceDir), "..", "liveaccess", "contract", "trusted", filepath.Base(sourcePath))
		}
		source, err := os.ReadFile(authored)
		if err != nil {
			t.Fatal(err)
		}
		for _, content := range [][]byte{source, raw} {
			unit := repofile.Parse(string(content))
			if unit == nil || unit.DeclarationError() != nil || unit.Address != operation.Address || unit.SchemaRef != operation.SchemaRef || unit.PathHint != operation.PathHint || kernel.CanonicalDigest(unit.Value) != kernel.CanonicalDigest(operation.Value) {
				t.Fatalf("%s differs from the shared Canonical codec: %#v", operation.PathHint, unit)
			}
		}
		unit := repofile.Parse(string(raw))
		byObject[operation.Address.ObjectID] = append(byObject[operation.Address.ObjectID], *unit)
		if !objectmodel.IsSchemaObject(operation.Address.ObjectID) {
			definition, exists := schemas[objectmodel.ObjectID(operation.SchemaRef)]
			if !exists {
				t.Fatalf("%s lacks a same-publication Schema", operation.PathHint)
			}
			if err := objectmodel.ValidateSchemaInstance(operation.Address, operation.Value, definition); err != nil {
				t.Fatalf("%s: %v", operation.PathHint, err)
			}
		}
	}
	for id, units := range byObject {
		assembled, err := repofile.Assemble(units)
		if err != nil {
			t.Fatal(err)
		}
		read, err := repo.Read(id, commit)
		if err != nil || kernel.CanonicalDigest(read.Value) != kernel.CanonicalDigest(assembled) {
			t.Fatalf("object %s differs from shared assembly: %v", id, err)
		}
	}
	for _, id := range []objectmodel.ObjectID{"kc/concept/object", "kc/concept/aspect", "kc/concept/relation", "kc/interface/writer", "kc/interface/cli", "kc/mechanism/governance", "kc/mechanism/hook", "kc/capability/read-object", "kc/capability/publish-object", "kc/guide/first-publication", "kc/guide/share-a-dataset", "kc/concept/index-declaration", "kc/mechanism/dynamic-access", "interface/live-access/v1"} {
		if _, exists := byObject[id]; !exists {
			t.Errorf("required self-description object %s is missing", id)
		}
	}
}

func TestSystemPublicationNavigationIsClosed(t *testing.T) {
	operations := systemrepo.SystemPublicationOperations()
	objects := map[objectmodel.ObjectID]bool{}
	files := map[string]bool{}
	for _, operation := range operations {
		objects[operation.Address.ObjectID] = true
		files[operation.PathHint] = true
	}
	links := regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)
	for _, operation := range operations {
		if operation.Address.Kind == objectmodel.KindRelation {
			relation, err := objectmodel.DecodeRelation(operation.Address, operation.Value)
			if err != nil {
				t.Fatal(err)
			}
			for _, endpoint := range relation.Endpoints {
				if endpoint.ObjectRef.Repository != objectmodel.SystemRepositoryID || !objects[endpoint.ObjectRef.Object] {
					t.Errorf("%s has unresolved navigation endpoint %#v", operation.PathHint, endpoint)
				}
			}
		}
		if !strings.HasSuffix(operation.PathHint, ".md") {
			continue
		}
		body := operation.Value.(map[string]any)["body"].(string)
		for _, match := range links.FindAllStringSubmatch(body, -1) {
			target := path.Clean(path.Join(path.Dir(operation.PathHint), match[1]))
			if !files[target] {
				t.Errorf("%s links to missing published file %s", operation.PathHint, target)
			}
		}
	}
}

func TestSystemPublicationUsesSchemaAccessAsOnlyIndexDeclarationForm(t *testing.T) {
	operations := systemrepo.SystemPublicationOperations()
	schemas := map[objectmodel.ObjectID]objectmodel.SchemaDefinition{}
	var live interfaces.InterfaceDefinition
	for _, operation := range operations {
		id := operation.Address.ObjectID
		if strings.HasPrefix(string(id), "index/") || id == "schema/core/index-declaration/v1" {
			t.Errorf("%s introduces a second index declaration form", operation.PathHint)
		}
		var err error
		switch {
		case objectmodel.IsSchemaObject(id):
			schemas[id], err = objectmodel.ParseSchemaDefinition(id, operation.Value)
		case id == "interface/live-access/v1":
			live, err = interfaces.ParseInterfaceDefinition(id, operation.Value)
		}
		if err != nil {
			t.Fatalf("%s: %v", operation.PathHint, err)
		}
	}
	if live.Operations["read"].Effect != "read" || live.Operations["invoke"].Effect != "write" {
		t.Fatalf("LiveAccess must distinguish current-state reading from authorized operations: %#v", live)
	}
	if len(live.StandardImplementationSchemas) != 1 || live.StandardImplementationSchemas[0] != "schema/implements.live-access/v1" {
		t.Fatalf("LiveAccess lacks its independently published implementation Schema: %#v", live)
	}
	for _, id := range []objectmodel.ObjectID{"schema/core/readme/v1", "schema/system/explanation/v1", "schema/system/capability-definition/v1", "schema/system/capability-usage/v1", "schema/system/usage-guide/v1"} {
		definition, exists := schemas[id]
		if !exists {
			t.Fatalf("missing structure Schema %s", id)
		}
		found := false
		for _, field := range definition.Fields {
			if field.Path == "body" {
				found = true
				if field.Type != "string" || len(field.Access) != 1 || field.Access[0] != "text" {
					t.Errorf("%s body must retain string structure and explicitly declared text discovery: %#v", id, field)
				}
			}
		}
		if !found {
			t.Errorf("%s lacks body structure and access declaration", id)
		}
	}
}

func TestSystemPublicationValuesAreIndependent(t *testing.T) {
	operations := systemrepo.SystemPublicationOperations()
	operations[0].Value.(map[string]any)["body"] = "mutated"
	repo := testkit.MakeSystemRepository(t)
	commit, _ := repo.Head(snapshot.DefaultRef)
	read, err := repo.ReadAddress(operations[0].Address, commit)
	if err != nil || read.Value.(map[string]any)["body"] == "mutated" {
		t.Fatalf("caller mutated binary publication: %#v %v", read, err)
	}
	read.Value.(map[string]any)["body"] = "mutated"
	again, err := repo.ReadAddress(operations[0].Address, commit)
	if err != nil || again.Value.(map[string]any)["body"] == "mutated" {
		t.Fatalf("caller mutated Repository: %#v %v", again, err)
	}
}

func TestSystemRepositoryDirectoryPaginationKeepsBasis(t *testing.T) {
	repo := testkit.MakeSystemRepository(t)
	commit, _ := repo.Head(snapshot.DefaultRef)
	request := snapshot.DirectoryRequest{Commit: commit, Directory: "capabilities", Limit: 2}
	var names []string
	for {
		page, err := repo.ReadDirectory(request)
		if err != nil || len(page.Entries) > 2 || page.Generation != string(commit) {
			t.Fatalf("invalid directory page: %#v %v", page, err)
		}
		for _, entry := range page.Entries {
			if entry.Kind != "directory" {
				t.Fatalf("unexpected child: %#v", entry)
			}
			names = append(names, entry.Name)
		}
		if page.Exhausted {
			break
		}
		wrong := request
		wrong.Directory = "concepts"
		wrong.Continuation = page.Continuation
		if _, err := repo.ReadDirectory(wrong); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
			t.Fatalf("cross-directory cursor: %v", err)
		}
		request.Continuation = page.Continuation
	}
	if len(names) != 15 {
		t.Fatalf("capability navigation incomplete: %v", names)
	}
	request.Commit = "another-version"
	if _, err := repo.ReadDirectory(request); kernel.CodeOf(err) != kernel.ErrVersionUnresolved {
		t.Fatalf("unavailable fixed basis: %v", err)
	}
}
