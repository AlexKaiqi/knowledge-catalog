package writer_test

import (
	"errors"
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	knowledgemaintenance "kc/knowledge/maintenance"
	"kc/knowledge/writer"
	"kc/snapshot"
)

type nativeSchemaRepository struct {
	*testkit.KnowledgeRepository
}

func (*nativeSchemaRepository) NativeKnowledgeRepository() {}
func (*nativeSchemaRepository) ReadFile(string, kernel.CommitID) ([]byte, error) {
	return nil, errors.New("native schema validation must not read the compatibility tree")
}
func (*nativeSchemaRepository) ListFiles(kernel.CommitID) ([]string, error) {
	return nil, errors.New("native schema validation must not list the compatibility tree")
}
func (r *nativeSchemaRepository) ApplyKnowledgeChange(_ string, change knowledge.ChangeSet) (kernel.CommitID, error) {
	return r.ApplyKnowledgeCommit(change)
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
		"metaSchema": string(knowledge.MetaSchemaV1),
		"entity":     "Policy", "aspect": "structure", "pattern": "record",
		"additionalProperties": false,
		"fields": map[string]any{
			"name":     map[string]any{"type": "string", "required": true, "access": []any{"text"}},
			"priority": map[string]any{"type": "integer"},
		},
	}
}

func TestSchemaDefinitionMustConformToSystemMetaSchema(t *testing.T) {
	s := testkit.NewSetup(t, "")
	_, err := s.Writer.Commit("bad-schema-shape", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy/structure/v1"},
			Value: map[string]any{
				"metaSchema": string(knowledge.MetaSchemaV1), "entity": "Policy", "aspect": "structure",
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
	operations := []knowledge.Operation{
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: strictPolicySchema()},
		{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "policy/A", AspectName: "structure"},
			SchemaRef: "schema/policy/structure/v1", Value: map[string]any{"name": "freeze", "priority": "high"}},
	}
	_, err := s.Writer.Commit("bad-instance", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: operations,
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaInstanceInvalid)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != s.RootCommitID {
		t.Fatalf("invalid instance moved HEAD to %s", got)
	}

	operations[1].Value = map[string]any{"name": "freeze", "priority": 1}
	if _, err := s.Writer.Commit("valid-instance", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID, Operations: operations,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaInstanceValidationRejectsMissingAndUnknownFields(t *testing.T) {
	s := testkit.NewSetup(t, "")
	schema, err := s.Writer.Commit("strict-schema", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: strictPolicySchema(),
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
			_, err := s.Writer.Commit("bad-"+tc.name, knowledge.CommitChangeSet{
				TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
				BaseCommit: schema.Result.CommitID, ExpectedTargetCommit: schema.Result.CommitID,
				Operations: []knowledge.Operation{{
					Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: knowledge.ObjectID("policy/" + tc.name), AspectName: "structure"},
					SchemaRef: "schema/policy/structure/v1", Value: tc.value,
				}},
			})
			testkit.ExpectCode(t, err, kernel.ErrSchemaInstanceInvalid)
		})
	}
}

func TestSchemaEvolutionRejectsBreakingReuseAndAllowsOptionalAddition(t *testing.T) {
	s := testkit.NewSetup(t, "")
	first, err := s.Writer.Commit("schema-v1", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: strictPolicySchema(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	breaking := strictPolicySchema()
	breaking["fields"].(map[string]any)["priority"] = map[string]any{"type": "string"}
	_, err = s.Writer.Commit("breaking-v1", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: first.Result.CommitID, ExpectedTargetCommit: first.Result.CommitID,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: breaking,
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaIncompatible)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != first.Result.CommitID {
		t.Fatalf("incompatible schema moved HEAD to %s", got)
	}

	compatible := strictPolicySchema()
	compatible["fields"].(map[string]any)["description"] = map[string]any{"type": "string", "access": []any{"text"}}
	if _, err := s.Writer.Commit("compatible-v1", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: first.Result.CommitID, ExpectedTargetCommit: first.Result.CommitID,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: compatible,
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

// An operation that omits schema_ref inherits the declaration already stored at
// that Address, so it must be held to the same Domain Schema contract. Skipping
// validation here would let any published instance drift silently.
func TestSchemaValidationCoversInheritedSchemaRef(t *testing.T) {
	s := testkit.NewSetup(t, "")
	published, err := s.Writer.Commit("publish", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: strictPolicySchema()},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "policy/A", AspectName: "structure"},
				SchemaRef: "schema/policy/structure/v1", Value: map[string]any{"name": "freeze"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// No SchemaRef on the operation: the stored declaration is inherited.
	_, err = s.Writer.Commit("inherit-invalid", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: published.Result.CommitID, ExpectedTargetCommit: published.Result.CommitID,
		Operations: []knowledge.Operation{{
			Op:      knowledge.OpPut,
			Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "policy/A", AspectName: "structure"},
			Value:   map[string]any{"name": "freeze", "owner": "ops"},
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaInstanceInvalid)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != published.Result.CommitID {
		t.Fatalf("inherited-schema violation moved HEAD to %s", got)
	}

	if _, err := s.Writer.Commit("inherit-valid", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: published.Result.CommitID, ExpectedTargetCommit: published.Result.CommitID,
		Operations: []knowledge.Operation{{
			Op:      knowledge.OpPut,
			Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "policy/A", AspectName: "structure"},
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
	_, err := s.Writer.Commit("aspect-schema-entity-instance", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: aspectOnly},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/A"},
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
		"metaSchema": string(knowledge.MetaSchemaV1),
		"entity":     "Policy", "aspect": "structure", "pattern": "record",
		"additionalProperties": true,
		"fields": map[string]any{
			"name": map[string]any{"type": "string", "required": true, "access": []any{"text"}},
		},
	}
	published, err := s.Writer.Commit("publish", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: permissive},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "policy/A", AspectName: "structure"},
				SchemaRef: "schema/policy/structure/v1", Value: map[string]any{"name": "freeze", "tier": "gold"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Adding an optional field is a compatible document change, but declaring a
	// type for the already-stored "tier" value invalidates the published instance.
	retyped := map[string]any{
		"metaSchema": string(knowledge.MetaSchemaV1),
		"entity":     "Policy", "aspect": "structure", "pattern": "record",
		"additionalProperties": true,
		"fields": map[string]any{
			"name": map[string]any{"type": "string", "required": true, "access": []any{"text"}},
			"tier": map[string]any{"type": "integer"},
		},
	}
	_, err = s.Writer.Commit("retype", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: published.Result.CommitID, ExpectedTargetCommit: published.Result.CommitID,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: retyped,
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaInstanceInvalid)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != published.Result.CommitID {
		t.Fatalf("unsafe in-place schema update moved HEAD to %s", got)
	}

	// Migrating the instance in the same ChangeSet makes the update provable.
	if _, err := s.Writer.Commit("retype-with-migration", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: published.Result.CommitID, ExpectedTargetCommit: published.Result.CommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: retyped},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "policy/A", AspectName: "structure"},
				SchemaRef: "schema/policy/structure/v1", Value: map[string]any{"name": "freeze", "tier": 1}},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

// Deleting a Domain Schema requires proof that nothing still references it.
func TestSchemaRemovalRequiresNoRemainingReferrers(t *testing.T) {
	s := testkit.NewSetup(t, "")
	published, err := s.Writer.Commit("publish", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy/structure/v1"}, Value: strictPolicySchema()},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "policy/A", AspectName: "structure"},
				SchemaRef: "schema/policy/structure/v1", Value: map[string]any{"name": "freeze"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.Writer.Commit("drop-referenced-schema", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: published.Result.CommitID, ExpectedTargetCommit: published.Result.CommitID,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpRemove, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy/structure/v1"},
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaIncompatible)
	if got := testkit.MustHead(t, s.Repo, snapshot.DefaultRef); got != published.Result.CommitID {
		t.Fatalf("referenced schema removal moved HEAD to %s", got)
	}

	// Removing the referrer in the same ChangeSet discharges the obligation.
	if _, err := s.Writer.Commit("drop-both", knowledge.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: published.Result.CommitID, ExpectedTargetCommit: published.Result.CommitID,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpRemove, Address: knowledge.Address{Kind: knowledge.KindAspect, ObjectID: "policy/A", AspectName: "structure"}},
			{Op: knowledge.OpRemove, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy/structure/v1"}},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaRefMissingRejected(t *testing.T) {
	s := testkit.NewSetup(t, "")
	_, err := s.Writer.Commit("bad-ref", knowledge.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{{
			Op:        knowledge.OpPut,
			Address:   knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/A"},
			Value:     map[string]any{"v": 1},
			SchemaRef: "schema/policy",
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaRevisionUnresolved)
}

func TestSchemaRefSameChangesetAccepted(t *testing.T) {
	s := testkit.NewSetup(t, "")
	_, err := s.Writer.Commit("boot", knowledge.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{
			{
				Op:      knowledge.OpPut,
				Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy"},
				Value:   schemaDoc(),
			},
			{
				Op:        knowledge.OpPut,
				Address:   knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/A"},
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
	first, err := s.Writer.Commit("schema", knowledge.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{{
			Op:      knowledge.OpPut,
			Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy"},
			Value:   schemaDoc(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Writer.Commit("use", knowledge.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           first.Result.CommitID,
		ExpectedTargetCommit: first.Result.CommitID,
		Operations: []knowledge.Operation{{
			Op:        knowledge.OpPut,
			Address:   knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/A"},
			Value:     map[string]any{"v": 1},
			SchemaRef: "schema/policy@" + string(first.Result.CommitID),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSchemaRefPriorCommitUsesNativeKnowledgeLookup(t *testing.T) {
	base := testkit.MakeRepository(t, "kr://acme/public/native")
	root, err := base.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	first, err := base.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: base.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: root, ExpectedTargetCommit: root,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy"}, Value: schemaDoc(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	native := &nativeSchemaRepository{KnowledgeRepository: base}
	store := snapshot.NewRegistry()
	if err := store.Add(native); err != nil {
		t.Fatal(err)
	}
	w, err := writer.NewWriter(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.Commit("native-use", knowledge.CommitChangeSet{
		TargetRepository: base.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: first, ExpectedTargetCommit: first,
		Operations: []knowledge.Operation{{
			Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/A"},
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
	if _, err := w.Commit("native-reverse", knowledge.CommitChangeSet{
		TargetRepository: base.ID(), TargetRef: snapshot.DefaultRef,
		BaseCommit: second, ExpectedTargetCommit: second,
		Operations: []knowledge.Operation{
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy"}, Value: schemaDoc()},
			{Op: knowledge.OpPut, Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/A"},
				Value: map[string]any{"v": 2}},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaRefForeignRepoRejected(t *testing.T) {
	s := testkit.NewSetup(t, "")
	_, err := s.Writer.Commit("foreign", knowledge.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{{
			Op:        knowledge.OpPut,
			Address:   knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/A"},
			Value:     map[string]any{"v": 1},
			SchemaRef: "kc://acme/other/core@deadbeef/schema/policy",
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaRevisionUnresolved)
}

func TestSchemaRefPinExistsButUnresolved(t *testing.T) {
	s := testkit.NewSetup(t, "")
	_, err := s.Writer.Commit("pin-miss", knowledge.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{{
			Op:        knowledge.OpPut,
			Address:   knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/A"},
			Value:     map[string]any{"v": 1},
			SchemaRef: "schema/policy@" + string(s.RootCommitID),
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaRevisionUnresolved)
}

func TestSchemaRefSameRepoKcAccepted(t *testing.T) {
	s := testkit.NewSetup(t, "")
	first, err := s.Writer.Commit("schema", knowledge.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{{
			Op:      knowledge.OpPut,
			Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy"},
			Value:   schemaDoc(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Writer.Commit("use", knowledge.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           first.Result.CommitID,
		ExpectedTargetCommit: first.Result.CommitID,
		Operations: []knowledge.Operation{{
			Op:        knowledge.OpPut,
			Address:   knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/A"},
			Value:     map[string]any{"v": 1},
			SchemaRef: "kc://acme/public/core@" + string(first.Result.CommitID) + "/schema/policy",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSchemaRefProposeRejectedAndAccepted(t *testing.T) {
	s := testkit.NewSetup(t, "")
	main := testkit.MustHead(t, s.Repo, "refs/heads/main")
	_, err := s.Writer.Propose("pr-bad", writer.ProposeIntent{
		TargetRepository: s.RepositoryID,
		TargetRef:        "refs/heads/main",
		CandidateRef:     "refs/heads/candidates/PR-bad",
		Operations: []knowledge.Operation{{
			Op:        knowledge.OpPut,
			Address:   knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/A"},
			Value:     map[string]any{"v": 1},
			SchemaRef: "schema/policy",
		}},
	})
	testkit.ExpectCode(t, err, kernel.ErrSchemaRevisionUnresolved)
	if testkit.MustHead(t, s.Repo, "refs/heads/main") != main {
		t.Fatal("propose moved main")
	}

	schema, err := s.Writer.Commit("schema", knowledge.CommitChangeSet{
		TargetRepository:     s.RepositoryID,
		TargetRef:            "refs/heads/main",
		BaseCommit:           s.RootCommitID,
		ExpectedTargetCommit: s.RootCommitID,
		Operations: []knowledge.Operation{{
			Op:      knowledge.OpPut,
			Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "schema/policy"},
			Value:   schemaDoc(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Writer.Propose("pr-ok", writer.ProposeIntent{
		TargetRepository: s.RepositoryID,
		TargetRef:        "refs/heads/main",
		CandidateRef:     "refs/heads/candidates/PR-ok",
		Operations: []knowledge.Operation{{
			Op:        knowledge.OpPut,
			Address:   knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "policy/A"},
			Value:     map[string]any{"v": 1},
			SchemaRef: "schema/policy",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if testkit.MustHead(t, s.Repo, "refs/heads/main") != schema.Result.CommitID {
		t.Fatal("propose moved main")
	}
}

func TestForkPublishProposesNewObject(t *testing.T) {
	pub := testkit.NewSetup(t, "kr://acme/public/semantic")
	personal := testkit.MakeRepository(t, "kr://acme/personals/alice")
	if err := pub.Store.Add(personal); err != nil {
		t.Fatal(err)
	}
	perHead := testkit.MustHead(t, personal, "")
	draft, err := pub.Writer.Commit("draft", knowledge.CommitChangeSet{
		TargetRepository:     personal.ID(),
		TargetRef:            "refs/heads/main",
		BaseCommit:           perHead,
		ExpectedTargetCommit: perHead,
		Operations: []knowledge.Operation{{
			Op:      knowledge.OpPut,
			Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "drafts/metric-x"},
			Value:   map[string]any{"text": "alice draft"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	source := "kc://acme/personals/alice@" + string(draft.Result.CommitID) + "/drafts/metric-x"
	pubHead := testkit.MustHead(t, pub.Repo, "refs/heads/main")
	proposed, err := pub.Writer.Propose("fork-1", writer.ProposeIntent{
		TargetRepository: pub.RepositoryID,
		TargetRef:        "refs/heads/main",
		CandidateRef:     "refs/heads/candidates/FORK-1",
		Operations: []knowledge.Operation{{
			Op:      knowledge.OpPut,
			Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: "metrics/x"},
			Value:   map[string]any{"text": "published"},
		}},
		Provenance: &knowledge.ProvenanceEnvelope{
			OriginKind: knowledge.OriginAssertion,
			SourceRefs: []string{source},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if testkit.MustHead(t, pub.Repo, "refs/heads/main") != pubHead {
		t.Fatal("propose moved public main")
	}
	if _, err := pub.Reader.Read(knowledge.KnowledgeRef{Repository: personal.ID(), Object: "drafts/metric-x"}, draft.Result.CommitID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := pub.Reader.Read(knowledge.KnowledgeRef{Repository: pub.RepositoryID, Object: "metrics/x"}, pubHead, nil); err == nil {
		t.Fatal("new object leaked onto public main")
	} else {
		testkit.ExpectCode(t, err, kernel.ErrKnowledgeRefUnresolved)
	}
	repoAtHead, err := pub.Reader.Require(pub.RepositoryID, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		t.Fatal(err)
	}
	scanner, err := knowledgemaintenance.RequireScanner(repoAtHead)
	if err != nil {
		t.Fatal(err)
	}
	page, err := scanner.ScanSnapshotPage(pubHead, knowledgemaintenance.ScanRequest{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Values {
		if item.Address.ObjectID == "drafts/metric-x" {
			t.Fatal("personal object copied into public")
		}
	}
	trace, err := pub.Reader.GetProvenance(knowledge.KnowledgeRef{Repository: pub.RepositoryID, Object: "metrics/x"}, proposed.Result.CommitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Chain) == 0 || len(trace.Chain[0].SourceRefs) == 0 || trace.Chain[0].SourceRefs[0] != source {
		t.Fatalf("%#v", trace)
	}
}
