package writer_test

import (
	"testing"

	"kc/internal/testkit"
	objectmodel "kc/object"
	"kc/snapshot"
)

// readme is an ordinary Aspect: the system gives neither the name, the
// schema, nor the file README.md any meaning of its own.
func TestReadmeAspectIsOrdinary(t *testing.T) {
	s := testkit.NewSetup(t, "")
	const readmeSchema = objectmodel.ObjectID("schema/core/readme/v1")
	const otherSchema = objectmodel.ObjectID("schema/biz/domain-page/v1")
	schemaValue := readmeSchemaValue(t)
	other := readmeSchemaValue(t)
	other["entity"] = "DomainPage"
	schemaPut := func(id objectmodel.ObjectID, value map[string]any) objectmodel.Operation {
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: id}, Value: value}
	}
	readmePut := func(id objectmodel.ObjectID, path string, schema objectmodel.ObjectID) objectmodel.Operation {
		return objectmodel.Operation{
			Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: id, AspectName: "readme"},
			PathHint: path, SchemaRef: string(schema), Value: map[string]any{"body": "# " + string(id)},
		}
	}
	placed := map[objectmodel.ObjectID]string{
		"biz/repo":      "README.md",
		"biz/domain-ad": "domains/ad/README.md",
		"biz/domain-xp": "domains/xp/README.md",
		"biz/plain":     "",
	}
	receipt, err := s.Writer.Commit("ordinary-readme", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			schemaPut(readmeSchema, schemaValue),
			schemaPut(otherSchema, other),
			readmePut("biz/repo", placed["biz/repo"], readmeSchema),
			readmePut("biz/domain-ad", placed["biz/domain-ad"], readmeSchema),
			readmePut("biz/domain-xp", placed["biz/domain-xp"], otherSchema),
			readmePut("biz/plain", placed["biz/plain"], readmeSchema),
		},
	})
	if err != nil {
		t.Fatalf("several objects with a readme Aspect must commit like any Aspect: %v", err)
	}
	commit := receipt.Result.CommitID
	for id, path := range placed {
		resolved, err := s.Repo.Resolve(id, commit)
		if err != nil {
			t.Fatal(err)
		}
		if path == "" {
			path = "readmes/biz/plain/readme.json"
		}
		if resolved.Status != objectmodel.StatusResolved || resolved.PathHint != path {
			t.Fatalf("%s stored at %q, want its declared path %q", id, resolved.PathHint, path)
		}
		got, err := s.Repo.Read(id, commit)
		if err != nil {
			t.Fatal(err)
		}
		aspect, _ := got.Value.(map[string]any)["readme"].(map[string]any)
		if aspect["body"] != "# "+string(id) {
			t.Fatalf("%s read back %#v", id, got.Value)
		}
	}
}
