package writer_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
	"kc/writer"
)

func schemaDoc() map[string]any {
	return map[string]any{
		"entity":  "Policy",
		"aspect":  "structure",
		"pattern": "record",
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
	listed, err := pub.Reader.List(pub.RepositoryID, pubHead)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range listed {
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
