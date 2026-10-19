package projection

import (
	"strings"
	"testing"

	retrieval "kc/discovery"
	"kc/internal/testkit"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/snapshot"
)

func TestSystemPublicationSchemaFieldsSupplyDiscoveryContract(t *testing.T) {
	repo := testkit.MakeSystemRepository(t)
	basis := testkit.MustHead(t, repo, snapshot.DefaultRef)
	spec, err := (&Index{}).specAtCommit(repo, basis)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []objectmodel.ObjectID{objectmodel.ObjectID("kc/system"), "kc/concept/object"} {
		value, err := repo.Read(id, basis)
		if err != nil {
			t.Fatal(err)
		}
		doc, include, err := compileValue(repo, value, spec)
		if err != nil || !include || strings.TrimSpace(doc.Text) == "" {
			t.Fatalf("published discovery text for %s lost: include=%v text=%q err=%v", id, include, doc.Text, err)
		}
	}
	field, err := spec.ResolveField(retrieval.FieldRef{Schema: objectmodel.ObjectID("schema/core/readme/v1"), Aspect: "readme", Path: "body"})
	if err != nil || !field.Has(reader.HintText) {
		t.Fatalf("readme text contract missing: %#v %v", field, err)
	}
}
