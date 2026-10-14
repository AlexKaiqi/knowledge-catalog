package writer_test

import (
	"errors"
	"kc/systemrepo"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	objectmaintenance "kc/object/maintenance"
	"kc/object/writer"
	"kc/snapshot"
)

type nativeSchemaRepository struct {
	*testkit.ObjectRepository
}

func (*nativeSchemaRepository) NativeObjectRepository() {}
func (*nativeSchemaRepository) ReadFile(string, kernel.CommitID) ([]byte, error) {
	return nil, errors.New("native schema validation must not read the compatibility tree")
}
func (*nativeSchemaRepository) ListFiles(kernel.CommitID) ([]string, error) {
	return nil, errors.New("native schema validation must not list the compatibility tree")
}
func (r *nativeSchemaRepository) ApplyObjectChange(_ string, change objectmodel.ChangeSet) (kernel.CommitID, error) {
	return r.ApplyObjectCommit(change)
}

// schemaDoc is an entity-level Domain Schema. These fixtures exercise
// schema_ref resolution, so the schema deliberately constrains the Entity blob
// Address the instances below write to.
func schemaDoc() map[string]any {
	return map[string]any{
		"entity":  "Policy",
		"pattern": "record",
	}
}

func strictPolicySchema() map[string]any {
	return map[string]any{
		"metaSchema": string(objectmodel.MetaSchemaV1),
		"entity":     "Policy", "aspect": "structure", "pattern": "record",
		"additionalProperties": false,
		"fields": map[string]any{
			"name":     map[string]any{"type": "string", "required": true, "access": []any{"text"}},
			"priority": map[string]any{"type": "integer"},
		},
	}
}

func TestWriterRejectsInstancePutAgainstBoundSchema(t *testing.T) {
	s := testkit.NewSetup(t, "")
	_, err := s.Writer.Commit("bound-instance", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/table.stats"}, Value: map[string]any{
				"entity": "Table", "aspect": "stats", "origin": "https://stats.example",
				"fields": map[string]any{"rowCount": map[string]any{"type": "number"}},
			}},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "stats"},
				SchemaRef: "schema/table.stats", Value: map[string]any{"rowCount": 1}},
		},
	})
	testkit.ExpectCode(t, err, kernel.ErrUsageInvalid)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != s.RootCommitID {
		t.Fatalf("bound instance moved HEAD to %s", got)
	}
}

func TestSchemaDefinitionMustConformToSystemMetaSchema(t *testing.T) {
	s := testkit.NewSetup(t, "")
	_, err := s.Writer.Commit("bad-schema-shape", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{
			Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"},
			Value: map[string]any{
				"metaSchema": string(objectmodel.MetaSchemaV1), "entity": "Policy", "aspect": "structure",
				"fields": map[string]any{"name": map[string]any{"type": "sql_varchar", "access": []any{"stored"}}},
			},
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaUnsupported)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != s.RootCommitID {
		t.Fatalf("invalid schema moved HEAD to %s", got)
	}
}

func TestSchemaInstanceValidationUsesSameChangesetDraft(t *testing.T) {
	s := testkit.NewSetup(t, "")
	operations := []objectmodel.Operation{
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: strictPolicySchema()},
		{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "structure"},
			SchemaRef: "schema/policy/structure/v1", Value: map[string]any{"name": "freeze", "priority": "high"}},
	}
	_, err := s.Writer.Commit("bad-instance", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: operations,
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaInstanceInvalid)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != s.RootCommitID {
		t.Fatalf("invalid instance moved HEAD to %s", got)
	}

	operations[1].Value = map[string]any{"name": "freeze", "priority": 1}
	committed, err := s.Writer.Commit("valid-instance", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: operations,
	})
	if err != nil {
		t.Fatal(err)
	}
	at := committed.Result.NewCommit
	if at == "" || at == s.RootCommitID || testkit.MustHead(t, s.Repo, snapshot.DefaultRef) != at {
		t.Fatalf("legal Schema/instance batch did not publish its receipt version: %#v", committed)
	}
	assertSchemaLifecycleValue(t, s, "schema/policy/structure/v1", at, strictPolicySchema())
	assertSchemaLifecycleValue(t, s, "policy/A", at, map[string]any{"structure": map[string]any{"name": "freeze", "priority": 1}})
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "structure"}
	instance, err := s.Reader.ReadAddress(s.RepositoryID, address, at)
	if err != nil || instance.Commit != at || instance.Address != address || len(instance.Declarations) != 1 || instance.Declarations[0].SchemaRef != "schema/policy/structure/v1" {
		t.Fatalf("published instance lost its same-batch Schema basis: %#v, %v", instance, err)
	}
	report, err := s.Reader.DescribeSchema(s.RepositoryID, at, "policy/A")
	if err != nil || report.Commit != at || report.Repository != s.RepositoryID || len(report.Schemas) != 1 || report.Schemas[0].ObjectID != "schema/policy/structure/v1" {
		t.Fatalf("instance Schema did not resolve at the published batch version: %#v, %v", report, err)
	}
}

func TestSchemaInstanceValidationRejectsMissingAndUnknownFields(t *testing.T) {
	s := testkit.NewSetup(t, "")
	schema, err := s.Writer.Commit("strict-schema", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{
			Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: strictPolicySchema(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		value map[string]any
	}{
		{name: "missing", value: map[string]any{"priority": 1}},
		{name: "unknown", value: map[string]any{"name": "freeze", "owner": "ops"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.Writer.Commit("bad-"+tc.name, objectmodel.CommitChangeSet{
				TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
				BaseCommit: schema.Result.CommitID, ExpectedTargetCommit: schema.Result.CommitID,
				Operations: []objectmodel.Operation{{
					Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: objectmodel.ObjectID("policy/" + tc.name), AspectName: "structure"},
					SchemaRef: "schema/policy/structure/v1", Value: tc.value,
				}},
			})
			testkit.ExpectCode(t, err, kernel.ErrSchemaInstanceInvalid)
		})
	}
}

func TestSchemaEvolutionRejectsBreakingReuseAndAllowsOptionalAddition(t *testing.T) {
	s := testkit.NewSetup(t, "")
	first, err := s.Writer.Commit("schema-v1", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{
			Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: strictPolicySchema(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	breaking := strictPolicySchema()
	breaking["fields"].(map[string]any)["priority"] = map[string]any{"type": "string"}
	_, err = s.Writer.Commit("breaking-v1", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: first.Result.CommitID, ExpectedTargetCommit: first.Result.CommitID,
		Operations: []objectmodel.Operation{{
			Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: breaking,
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaIncompatible)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != first.Result.CommitID {
		t.Fatalf("incompatible schema moved HEAD to %s", got)
	}

	compatible := strictPolicySchema()
	compatible["fields"].(map[string]any)["description"] = map[string]any{"type": "string", "access": []any{"text"}}
	updated, err := s.Writer.Commit("compatible-v1", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: first.Result.CommitID, ExpectedTargetCommit: first.Result.CommitID,
		Operations: []objectmodel.Operation{{
			Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: compatible,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := strictPolicySchema()
	want["fields"].(map[string]any)["description"] = map[string]any{"type": "string", "access": []any{"text"}}
	assertSchemaLifecycleValue(t, s, "schema/policy/structure/v1", updated.Result.NewCommit, want)
	assertSchemaLifecycleValue(t, s, "schema/policy/structure/v1", first.Result.NewCommit, strictPolicySchema())
}

// An operation that omits schema_ref inherits the declaration already stored at
// that Address, so it must be held to the same Domain Schema contract. Skipping
// validation here would let any published instance drift silently.
func TestSchemaValidationCoversInheritedSchemaRef(t *testing.T) {
	s := testkit.NewSetup(t, "")
	published, err := s.Writer.Commit("publish", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: strictPolicySchema()},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "structure"},
				SchemaRef: "schema/policy/structure/v1", Value: map[string]any{"name": "freeze"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// No SchemaRef on the operation: the stored declaration is inherited.
	_, err = s.Writer.Commit("inherit-invalid", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: published.Result.CommitID, ExpectedTargetCommit: published.Result.CommitID,
		Operations: []objectmodel.Operation{{
			Op:      objectmodel.OpPut,
			Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "structure"},
			Value:   map[string]any{"name": "freeze", "owner": "ops"},
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaInstanceInvalid)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != published.Result.CommitID {
		t.Fatalf("inherited-schema violation moved HEAD to %s", got)
	}

	if _, err := s.Writer.Commit("inherit-valid", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: published.Result.CommitID, ExpectedTargetCommit: published.Result.CommitID,
		Operations: []objectmodel.Operation{{
			Op:      objectmodel.OpPut,
			Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "structure"},
			Value:   map[string]any{"name": "thaw", "priority": 2},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

// Address/pattern matching is a property of the Meta Schema contract, not an
// opt-in that a provider can skip by omitting the metaSchema field.
func TestSchemaAddressMatchingAppliesWithoutExplicitMetaSchema(t *testing.T) {
	s := testkit.NewSetup(t, "")
	aspectOnly := map[string]any{"entity": "Policy", "aspect": "structure", "pattern": "record"}
	_, err := s.Writer.Commit("aspect-schema-entity-instance", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: aspectOnly},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"},
				SchemaRef: "schema/policy/structure/v1", Value: map[string]any{"name": "freeze"}},
		},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaInstanceInvalid)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != s.RootCommitID {
		t.Fatalf("address mismatch moved HEAD to %s", got)
	}
}

// A compatible schema document change is not automatically safe: the instances
// already published against that identity must still satisfy the new contract.
func TestSchemaUpdateValidatesAlreadyPublishedInstances(t *testing.T) {
	s := testkit.NewSetup(t, "")
	permissive := map[string]any{
		"metaSchema": string(objectmodel.MetaSchemaV1),
		"entity":     "Policy", "aspect": "structure", "pattern": "record",
		"additionalProperties": true,
		"fields": map[string]any{
			"name": map[string]any{"type": "string", "required": true, "access": []any{"text"}},
		},
	}
	published, err := s.Writer.Commit("publish", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: permissive},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "structure"},
				SchemaRef: "schema/policy/structure/v1", Value: map[string]any{"name": "freeze", "tier": "gold"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Adding an optional field is a compatible document change, but declaring a
	// type for the already-stored "tier" value invalidates the published instance.
	retyped := map[string]any{
		"metaSchema": string(objectmodel.MetaSchemaV1),
		"entity":     "Policy", "aspect": "structure", "pattern": "record",
		"additionalProperties": true,
		"fields": map[string]any{
			"name": map[string]any{"type": "string", "required": true, "access": []any{"text"}},
			"tier": map[string]any{"type": "integer"},
		},
	}
	_, err = s.Writer.Commit("retype", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: published.Result.CommitID, ExpectedTargetCommit: published.Result.CommitID,
		Operations: []objectmodel.Operation{{
			Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: retyped,
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaInstanceInvalid)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != published.Result.CommitID {
		t.Fatalf("unsafe in-place schema update moved HEAD to %s", got)
	}

	// Migrating the instance in the same ChangeSet makes the update provable.
	migrated, err := s.Writer.Commit("retype-with-migration", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: published.Result.CommitID, ExpectedTargetCommit: published.Result.CommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: retyped},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "structure"},
				SchemaRef: "schema/policy/structure/v1", Value: map[string]any{"name": "freeze", "tier": 1}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSchema := map[string]any{
		"metaSchema": string(objectmodel.MetaSchemaV1),
		"entity":     "Policy", "aspect": "structure", "pattern": "record", "additionalProperties": true,
		"fields": map[string]any{
			"name": map[string]any{"type": "string", "required": true, "access": []any{"text"}},
			"tier": map[string]any{"type": "integer"},
		},
	}
	assertSchemaLifecycleValue(t, s, "schema/policy/structure/v1", migrated.Result.NewCommit, wantSchema)
	assertSchemaLifecycleValue(t, s, "policy/A", migrated.Result.NewCommit, map[string]any{"structure": map[string]any{"name": "freeze", "tier": 1}})
	delete(wantSchema["fields"].(map[string]any), "tier")
	assertSchemaLifecycleValue(t, s, "schema/policy/structure/v1", published.Result.NewCommit, wantSchema)
	assertSchemaLifecycleValue(t, s, "policy/A", published.Result.NewCommit, map[string]any{"structure": map[string]any{"name": "freeze", "tier": "gold"}})
}

// Deleting a Domain Schema requires proof that nothing still references it.
func TestSchemaRemovalRequiresNoRemainingReferrers(t *testing.T) {
	s := testkit.NewSetup(t, "")
	published, err := s.Writer.Commit("publish", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: strictPolicySchema()},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "structure"},
				SchemaRef: "schema/policy/structure/v1", Value: map[string]any{"name": "freeze"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.Writer.Commit("drop-referenced-schema", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: published.Result.CommitID, ExpectedTargetCommit: published.Result.CommitID,
		Operations: []objectmodel.Operation{{
			Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"},
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaIncompatible)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != published.Result.CommitID {
		t.Fatalf("referenced schema removal moved HEAD to %s", got)
	}

	// Removing the referrer in the same ChangeSet discharges the obligation.
	removed, err := s.Writer.Commit("drop-both", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: published.Result.CommitID, ExpectedTargetCommit: published.Result.CommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "structure"}},
			{Op: objectmodel.OpRemove, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range []objectmodel.Address{
		{Kind: objectmodel.KindEntity, ObjectID: "schema/policy/structure/v1"},
		{Kind: objectmodel.KindAspect, ObjectID: "policy/A", AspectName: "structure"},
	} {
		_, err := s.Reader.ReadAddress(s.RepositoryID, address, removed.Result.NewCommit)
		testkit.ExpectCode(t, err, kernel.ErrObjectRefUnresolved)
	}
	assertSchemaLifecycleValue(t, s, "schema/policy/structure/v1", published.Result.NewCommit, strictPolicySchema())
	assertSchemaLifecycleValue(t, s, "policy/A", published.Result.NewCommit, map[string]any{"structure": map[string]any{"name": "freeze"}})
}

func assertSchemaLifecycleValue(t *testing.T, s testkit.Setup, id objectmodel.ObjectID, commit kernel.CommitID, want any) {
	t.Helper()
	value, err := s.Reader.Read(objectmodel.ObjectRef{Repository: s.RepositoryID, Object: id}, commit, nil)
	if err != nil {
		t.Fatal(err)
	}
	if value.Commit != commit || value.ObjectRef.Object != id || kernel.CanonicalDigest(value.Value) != kernel.CanonicalDigest(want) {
		t.Fatalf("Schema lifecycle lost fixed value: got=%#v want object=%s commit=%s value=%#v", value, id, commit, want)
	}
}

func TestSchemaRefMissingRejected(t *testing.T) {
	s := testkit.NewSetup(t, "")
	_, err := s.Writer.Commit("bad-ref", objectmodel.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{
			Op:        objectmodel.OpPut,
			Address:   objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"},
			Value:     map[string]any{"v": 1},
			SchemaRef: "schema/policy",
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaRevisionUnresolved)
}

func TestSchemaRefSameChangesetAccepted(t *testing.T) {
	s := testkit.NewSetup(t, "")
	_, err := s.Writer.Commit("boot", objectmodel.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{
				Op:      objectmodel.OpPut,
				Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy"},
				Value:   schemaDoc(),
			},
			{
				Op:        objectmodel.OpPut,
				Address:   objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"},
				Value:     map[string]any{"v": 1},
				SchemaRef: "schema/policy",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSchemaRefPriorCommitAccepted(t *testing.T) {
	s := testkit.NewSetup(t, "")
	first, err := s.Writer.Commit("schema", objectmodel.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{
			Op:      objectmodel.OpPut,
			Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy"},
			Value:   schemaDoc(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Writer.Commit("use", objectmodel.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           first.Result.CommitID,
		ExpectedTargetCommit: first.Result.CommitID,
		Operations: []objectmodel.Operation{{
			Op:        objectmodel.OpPut,
			Address:   objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"},
			Value:     map[string]any{"v": 1},
			SchemaRef: "schema/policy@" + string(first.Result.CommitID),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSchemaRefPriorCommitUsesNativeObjectLookup(t *testing.T) {
	base := testkit.MakeRepository(t, "kr://acme/public/native")
	root, err := base.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	first, err := base.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: base.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []objectmodel.Operation{{
			Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy"}, Value: schemaDoc(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	native := &nativeSchemaRepository{ObjectRepository: base}
	store := snapshot.NewRegistry()
	if err := store.Add(native); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("native-use", objectmodel.CommitChangeSet{
		TargetRepository: base.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: first, ExpectedTargetCommit: first,
		Operations: []objectmodel.Operation{{
			Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"},
			Value: map[string]any{"v": 1}, SchemaRef: "schema/policy",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := base.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	// Republishing the schema now needs the reverse referrer index and the
	// inherited-declaration lookup. Both must stay native point reads: this
	// fixture fails the whole commit if either falls back to the tree codec.
	if _, err := w.Commit("native-reverse", objectmodel.CommitChangeSet{
		TargetRepository: base.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: second, ExpectedTargetCommit: second,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy"}, Value: schemaDoc()},
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"},
				Value: map[string]any{"v": 2}},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaRefForeignRepoRejected(t *testing.T) {
	s := testkit.NewSetup(t, "")
	_, err := s.Writer.Commit("foreign", objectmodel.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{
			Op:        objectmodel.OpPut,
			Address:   objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"},
			Value:     map[string]any{"v": 1},
			SchemaRef: "kc://acme/other/core@deadbeef/schema/policy",
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaRevisionUnresolved)
}

func TestSchemaRefPinExistsButUnresolved(t *testing.T) {
	s := testkit.NewSetup(t, "")
	_, err := s.Writer.Commit("pin-miss", objectmodel.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{
			Op:        objectmodel.OpPut,
			Address:   objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"},
			Value:     map[string]any{"v": 1},
			SchemaRef: "schema/policy@" + string(s.RootCommitID),
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaRevisionUnresolved)
}

func TestSchemaRefSameRepoKcAccepted(t *testing.T) {
	s := testkit.NewSetup(t, "")
	first, err := s.Writer.Commit("schema", objectmodel.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{
			Op:      objectmodel.OpPut,
			Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy"},
			Value:   schemaDoc(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Writer.Commit("use", objectmodel.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           first.Result.CommitID,
		ExpectedTargetCommit: first.Result.CommitID,
		Operations: []objectmodel.Operation{{
			Op:        objectmodel.OpPut,
			Address:   objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"},
			Value:     map[string]any{"v": 1},
			SchemaRef: "kc://acme/public/core@" + string(first.Result.CommitID) + "/schema/policy",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func reviewBranch(t *testing.T, s testkit.Setup, repository kernel.RepositoryID, ref string) string {
	t.Helper()
	if _, err := s.Writer.CreateBranch(writer.BranchIntent{TargetRepository: repository, Ref: ref}); err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestSchemaRefBranchCommitRejectedAndAccepted(t *testing.T) {
	s := testkit.NewSetup(t, "")
	main := testkit.MustHead(t, s.Repo, "refs/heads/main")
	_, err := s.Writer.CommitIntent("pr-bad", writer.CommitIntent{
		TargetRepository: s.RepositoryID,
		TargetRef:        reviewBranch(t, s, s.RepositoryID, "refs/heads/review/PR-bad"),
		Operations: []objectmodel.Operation{{
			Op:        objectmodel.OpPut,
			Address:   objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"},
			Value:     map[string]any{"v": 1},
			SchemaRef: "schema/policy",
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaRevisionUnresolved)
	if testkit.MustHead(t, s.Repo, "refs/heads/main") != main {
		t.Fatal("branch commit moved main")
	}

	schema, err := s.Writer.Commit("schema", objectmodel.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{
			Op:      objectmodel.OpPut,
			Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/policy"},
			Value:   schemaDoc(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Writer.CommitIntent("pr-ok", writer.CommitIntent{
		TargetRepository: s.RepositoryID,
		TargetRef:        reviewBranch(t, s, s.RepositoryID, "refs/heads/review/PR-ok"),
		Operations: []objectmodel.Operation{{
			Op:        objectmodel.OpPut,
			Address:   objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "policy/A"},
			Value:     map[string]any{"v": 1},
			SchemaRef: "schema/policy",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if testkit.MustHead(t, s.Repo, "refs/heads/main") != schema.Result.CommitID {
		t.Fatal("branch commit moved main")
	}
}

func TestForkPublishOnBranchCreatesNewObject(t *testing.T) {
	pub := testkit.NewSetup(t, "kr://acme/public/semantic")
	personal := testkit.MakeRepository(t, "kr://acme/personals/alice")
	if err := pub.Store.Add(personal); err != nil {
		t.Fatal(err)
	}
	perHead := testkit.MustHead(t, personal, "")
	draft, err := pub.Writer.Commit("draft", objectmodel.CommitChangeSet{
		TargetRepository:     personal.ID(),
		TargetRef:            "refs/heads/main",
		BaseCommit:           perHead,
		ExpectedTargetCommit: perHead,
		Operations: []objectmodel.Operation{{
			Op:      objectmodel.OpPut,
			Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "drafts/metric-x"},
			Value:   map[string]any{"text": "alice draft"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	source := "kc://acme/personals/alice@" + string(draft.Result.CommitID) + "/drafts/metric-x"
	pubHead := testkit.MustHead(t, pub.Repo, "refs/heads/main")
	proposed, err := pub.Writer.CommitIntent("fork-1", writer.CommitIntent{
		TargetRepository: pub.RepositoryID,
		TargetRef:        reviewBranch(t, pub, pub.RepositoryID, "refs/heads/review/FORK-1"),
		Operations: []objectmodel.Operation{{
			Op:      objectmodel.OpPut,
			Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "metrics/x"},
			Value:   map[string]any{"text": "published"},
		}},
		Provenance: &objectmodel.ProvenanceEnvelope{
			OriginKind: objectmodel.OriginAssertion,
			SourceRefs: []string{source},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if testkit.MustHead(t, pub.Repo, "refs/heads/main") != pubHead {
		t.Fatal("branch commit moved public main")
	}
	if _, err := pub.Reader.Read(objectmodel.ObjectRef{Repository: personal.ID(), Object: "drafts/metric-x"}, draft.Result.CommitID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := pub.Reader.Read(objectmodel.ObjectRef{Repository: pub.RepositoryID, Object: "metrics/x"}, pubHead, nil); err == nil {
		t.Fatal("new object leaked onto public main")
	} else {
		testkit.ExpectCode(t, err, kernel.ErrObjectRefUnresolved)
	}
	repoAtHead, err := pub.Reader.Require(pub.RepositoryID, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	scanner, err := objectmaintenance.RequireScanner(repoAtHead)
	if err != nil {
		t.Fatal(err)
	}
	page, err := scanner.ScanSnapshotPage(pubHead, objectmaintenance.ScanRequest{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Values {
		if item.Address.ObjectID == "drafts/metric-x" {
			t.Fatal("personal object copied into public")
		}
	}
	trace, err := pub.Reader.GetProvenance(objectmodel.ObjectRef{Repository: pub.RepositoryID, Object: "metrics/x"}, proposed.Result.CommitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Chain) == 0 || len(trace.Chain[0].SourceRefs) == 0 || trace.Chain[0].SourceRefs[0] != source {
		t.Fatalf("%#v", trace)
	}
}

func readmeSchemaValue(t *testing.T) map[string]any {
	t.Helper()
	for _, operation := range systemrepo.SystemSchemaOperations() {
		if operation.Address.ObjectID == objectmodel.CoreReadmeSchemaV1 {
			value, ok := operation.Value.(map[string]any)
			if !ok {
				t.Fatalf("readme schema value %T", operation.Value)
			}
			return value
		}
	}
	t.Fatal("readme schema is not published")
	return nil
}

func TestReadmeAspectIsMarkdownBody(t *testing.T) {
	s := testkit.NewSetup(t, "")
	schemaValue := readmeSchemaValue(t)
	schemaOp := objectmodel.Operation{
		Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.CoreReadmeSchemaV1},
		Value: schemaValue,
	}
	instanceAddr := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "payments", AspectName: objectmodel.ReadmeAspect}
	valid := map[string]any{"body": "# Payments warehouse\n\nPublished metrics and tables for payments."}

	drifted := readmeSchemaValue(t)
	drifted["entity"] = "Other"
	_, err := s.Writer.Commit("drift-readme-schema", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{
			Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.CoreReadmeSchemaV1},
			Value: drifted,
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaIncompatible)

	_, err = s.Writer.Commit("readme-wrong-aspect", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{schemaOp, {
			Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "payments"},
			SchemaRef: string(objectmodel.CoreReadmeSchemaV1), Value: valid,
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaInstanceInvalid)

	_, err = s.Writer.Commit("readme-missing-ref", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{{
			Op: objectmodel.OpPut, Address: instanceAddr, Value: valid,
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaInstanceInvalid)

	_, err = s.Writer.Commit("readme-unknown-field", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{schemaOp, {
			Op: objectmodel.OpPut, Address: instanceAddr,
			SchemaRef: string(objectmodel.CoreReadmeSchemaV1),
			Value:     map[string]any{"body": "ok", "owner": "payments"},
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaInstanceInvalid)

	receipt, err := s.Writer.Commit("readme-ok", objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{schemaOp, {
			Op: objectmodel.OpPut, Address: instanceAddr,
			SchemaRef: string(objectmodel.CoreReadmeSchemaV1), Value: valid,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Repo.Read("payments", receipt.Result.CommitID)
	if err != nil {
		t.Fatal(err)
	}
	body, ok := objectmodel.ReadmeBody(got.Value)
	if !ok || body != valid["body"] {
		t.Fatalf("read back %#v", got.Value)
	}
}
