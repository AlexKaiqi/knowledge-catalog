package catalog_test

import (
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/knowledge"
)

func TestOpenKnowledgeSetDoesNotFollowLaterCommitUntilRepublish(t *testing.T) {
	s := setupFed(t)
	defined, err := s.catalog.DefineKnowledgeSet("v", 1, []catalog.KnowledgeSetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
		{Repository: "kr://acme/groups/payments", Selector: "refs/heads/main"},
	})
	if err != nil {
		t.Fatal(err)
	}
	published := defined.Sources[0].Commit
	if published == "" {
		t.Fatal("publish must freeze source commits")
	}
	head := testkit.MustHead(t, s.publicRepo, "refs/heads/main")
	later, err := s.publicRepo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: "kr://acme/public/core", TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("policy/P-103", map[string]any{"statement": "later"}, ""),
	})
	if err != nil {
		t.Fatal(err)
	}

	serving, err := testkit.OpenKnowledgeSet(s.catalog, "v")
	if err != nil {
		t.Fatal(err)
	}
	if serving.Pin().SetID != "v" {
		t.Fatal(serving.Pin().SetID)
	}
	reads, err := serving.Read("policy/P-103", nil)
	if err != nil || len(reads) != 2 {
		t.Fatal(reads, err)
	}
	byRepo := map[kernel.RepositoryID]any{}
	for _, item := range reads {
		byRepo[item.Repository] = item.Value
		if item.Commit == "" {
			t.Fatal("consumer result still carries the resolved commit")
		}
	}
	if byRepo["kr://acme/public/core"].(map[string]any)["statement"] != "public v1" {
		t.Fatal("published dataset must stay on the frozen commit", byRepo)
	}
	if byRepo["kr://acme/groups/payments"].(map[string]any)["statement"] != "group qualification" {
		t.Fatal(byRepo)
	}
	if serving.Pin().Repositories["kr://acme/public/core"] != published {
		t.Fatal(serving.Pin().Repositories["kr://acme/public/core"], published, later)
	}

	traces, err := serving.GetProvenance("policy/P-103")
	if err != nil || len(traces) != 2 {
		t.Fatal(traces, err)
	}
	resolved, err := serving.Resolve("policy/P-103")
	if err != nil || len(resolved) != 2 {
		t.Fatal(resolved, err)
	}
	logs, err := serving.Log("policy/P-103", 0)
	if err != nil || len(logs) != 2 {
		t.Fatal(logs, err)
	}
	pin := serving.Pin().Repositories["kr://acme/public/core"]
	if pin != published {
		t.Fatal(pin, published, later)
	}
	for _, item := range logs {
		if item.Commit == "" || item.ObjectID != "policy/P-103" {
			t.Fatal(item)
		}
		if item.Repository == "kr://acme/public/core" && item.Commit != published {
			t.Fatal(item.Commit, published)
		}
	}
	if _, err := s.catalog.DefineKnowledgeSet("v", 2, []catalog.KnowledgeSetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
		{Repository: "kr://acme/groups/payments", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	nextServing, err := testkit.OpenKnowledgeSet(s.catalog, "v")
	if err != nil {
		t.Fatal(err)
	}
	republished, err := nextServing.Read("policy/P-103", nil)
	if err != nil {
		t.Fatal(err)
	}
	var laterBody any
	for _, item := range republished {
		if item.Repository == "kr://acme/public/core" {
			laterBody = item.Value.(map[string]any)["statement"]
		}
	}
	if laterBody != "later" {
		t.Fatal(republished)
	}
	if nextServing.Pin().Repositories["kr://acme/public/core"] != later {
		t.Fatal(nextServing.Pin().Repositories["kr://acme/public/core"], later)
	}
	missing, err := serving.Read("absent", nil)
	if err != nil || len(missing) != 0 {
		t.Fatal(missing, err)
	}
	_, err = testkit.OpenKnowledgeSet(s.catalog, "missing")
	testkit.ExpectCode(t, err, kernel.ErrKnowledgeSetInvalid)
}

func TestOpenedKnowledgeSetPinDoesNotMoveWithLaterCommit(t *testing.T) {
	s := setupFed(t)
	if _, err := s.catalog.DefineKnowledgeSet("v", 1, []catalog.KnowledgeSetSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	serving, err := testkit.OpenKnowledgeSet(s.catalog, "v")
	if err != nil {
		t.Fatal(err)
	}
	opened := serving.Pin().Repositories["kr://acme/public/core"]
	head := testkit.MustHead(t, s.publicRepo, "refs/heads/main")
	if _, err := s.publicRepo.ApplyKnowledgeCommit(knowledge.CommitChangeSet{
		TargetRepository: "kr://acme/public/core", TargetRef: "refs/heads/main",
		BaseCommit: head, ExpectedTargetCommit: head,
		Operations: testkit.PutEntity("policy/P-103", map[string]any{"statement": "after-open"}, ""),
	}); err != nil {
		t.Fatal(err)
	}
	reads, err := serving.Read("policy/P-103", nil)
	if err != nil || reads[0].Value.(map[string]any)["statement"] != "public v1" {
		t.Fatal(reads, err)
	}
	if reads[0].Commit != opened {
		t.Fatal(reads[0].Commit, opened)
	}
	next, err := testkit.OpenKnowledgeSet(s.catalog, "v")
	if err != nil {
		t.Fatal(err)
	}
	later, err := next.Read("policy/P-103", nil)
	if err != nil || later[0].Value.(map[string]any)["statement"] != "public v1" {
		t.Fatal("re-resolve must stay on the published commit until republish", later, err)
	}
}
