package writer_test

import (
	"maps"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
	"kc/systemrepo"
)

func TestWriterProtocolSchemaCopiesRejectCompatibleDrift(t *testing.T) {
	trusted := objectmodel.TrustedSchemaOperations()
	if len(trusted) == 0 {
		t.Fatal("protocol Schema copy checks need the published trust roots")
	}
	for _, published := range trusted {
		t.Run(string(published.Address.ObjectID), func(t *testing.T) {
			drifted := descriptionOnlySchemaChange(t, published)
			t.Run("first-publication", func(t *testing.T) {
				s := testkit.NewSetup(t, "")
				assertIndexWriterRejected(t, s, "drifted-first-copy", kernel.ErrSchemaIncompatible, drifted)
				head := testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
				_, err := s.Reader.ReadAddress(s.RepositoryID, published.Address, head)
				testkit.ExpectCode(t, err, kernel.ErrObjectRefUnresolved)
			})
			t.Run("published-copy", func(t *testing.T) {
				s := testkit.NewSetup(t, "")
				copied := commitIndexWriterDrafts(t, s, s.Repo, "exact-copy", published)
				assertIndexWriterStored(t, s, s.RepositoryID, copied, published)

				assertIndexWriterRejected(t, s, "drifted-update", kernel.ErrSchemaIncompatible, drifted)
				head := testkit.MustHead(t, s.Repo, snapshot.DefaultRef)
				assertIndexWriterStored(t, s, s.RepositoryID, head, published)
			})
		})
	}
}

func TestWriterOrdinarySystemSchemaCopiesAllowCompatibleEvolution(t *testing.T) {
	published := map[objectmodel.ObjectID]objectmodel.Operation{}
	for _, operation := range systemrepo.SystemSchemaOperations() {
		published[operation.Address.ObjectID] = operation
	}
	for _, id := range []objectmodel.ObjectID{"schema/core/readme/v1", objectmodel.CoreRelationSchemaV1} {
		t.Run(string(id), func(t *testing.T) {
			original, ok := published[id]
			if !ok {
				t.Fatalf("ordinary Schema %s is missing from the system publication", id)
			}
			changed := descriptionOnlySchemaChange(t, original)
			s := testkit.NewSetup(t, "")
			first := commitIndexWriterDrafts(t, s, s.Repo, "ordinary-copy", original)
			next := commitIndexWriterDrafts(t, s, s.Repo, "ordinary-evolution", changed)
			assertIndexWriterStored(t, s, s.RepositoryID, next, changed)
			assertIndexWriterStored(t, s, s.RepositoryID, first, original)
		})
	}
}

// Description-only edits are structurally compatible. Rejecting these roots
// must come from publication immutability, not ordinary Schema evolution rules.
func descriptionOnlySchemaChange(t *testing.T, published objectmodel.Operation) objectmodel.Operation {
	t.Helper()
	value, ok := published.Value.(map[string]any)
	if !ok {
		t.Fatalf("Schema %s must have a structured definition, got %T", published.Address.ObjectID, published.Value)
	}
	changed := published
	next := maps.Clone(value)
	description, _ := value["description"].(string)
	next["description"] = description + " Repository-local description."
	changed.Value = next
	before, err := objectmodel.ParseSchemaDefinition(published.Address.ObjectID, published.Value)
	if err != nil {
		t.Fatal(err)
	}
	after, err := objectmodel.ParseSchemaDefinition(changed.Address.ObjectID, changed.Value)
	if err != nil {
		t.Fatal(err)
	}
	if breaks := objectmodel.BreakingSchemaChanges(before, after); len(breaks) != 0 {
		t.Fatalf("the copy drift must be structurally compatible: %#v", breaks)
	}
	if kernel.CanonicalDigest(changed.Value) == kernel.CanonicalDigest(published.Value) {
		t.Fatal("the copy drift must change the published document")
	}
	return changed
}
