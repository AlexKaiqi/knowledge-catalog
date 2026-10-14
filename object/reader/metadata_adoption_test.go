package reader_test

import (
	"testing"

	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/object/reader"
)

func TestSchemaAdoptionMetadataCannotReadBodiesOrEscapePublishedScope(t *testing.T) {
	rd, guard, _, at := newReferenceReadFixture(t)
	for _, path := range []string{"private/description.yaml", "public/id.yaml", "private/status.yaml"} {
		guard.blocked[path] = true
	}
	repo, err := rd.Require(guard.ID(), kernel.ErrObjectRefUnresolved)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := repo.(objectmodel.SchemaAdoptionLocator).AdoptedSchemaRefs("table/orders", at)
	if err != nil || len(refs) != 0 {
		t.Fatalf("known empty Schema adoption required object bodies: %v %v", refs, err)
	}
	scope := reader.Open(func(kernel.RepositoryID) (objectmodel.Repository, error) { return repo, nil }, reader.DatasetPin{
		Repositories: map[kernel.RepositoryID]kernel.CommitID{guard.ID(): at},
		Items:        []reader.DatasetItem{{Repository: guard.ID(), Commit: at, Kind: "prefix", Prefix: "public"}},
	})
	member, err := scope.Member(guard.ID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := member.(objectmodel.SchemaAdoptionLocator).AdoptedSchemaRefs("table/orders", at); kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		t.Fatalf("partial object metadata became complete virtual type evidence: %v", err)
	}
	locations, err := repofile.ReadObjectUnitLocations(guard.TreeStore, "table/orders", at)
	if err != nil {
		t.Fatal(err)
	}
	for i := range locations {
		locations[i].SchemaRef = nil
	}
	guard.locatorOverride, err = repofile.EncodeObjectUnitLocations("table/orders", locations)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.(objectmodel.SchemaAdoptionLocator).AdoptedSchemaRefs("table/orders", at); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("old metadata guessed Schema adoption from sibling bodies: %v", err)
	}
	for path := range guard.blocked {
		if guard.reads[path] != 0 {
			t.Fatalf("metadata interpretation accessed a body: %s count=%d", path, guard.reads[path])
		}
	}
}
