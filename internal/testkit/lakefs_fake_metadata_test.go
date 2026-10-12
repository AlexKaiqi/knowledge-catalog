package testkit

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"kc/kernel"
	"kc/snapshot"
)

func TestLakeFSFakePreservesImmutableCommitMetadataAcrossFork(t *testing.T) {
	fake := NewLakeFSFake(t)
	const repositoryID kernel.RepositoryID = "kr://fixture/audit"
	dsn := fake.DSN(fake.NewRepo())
	source, err := snapshot.OpenLakeFS(repositoryID, dsn, fake.Credential())
	if err != nil {
		t.Fatal(err)
	}
	base, err := source.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	tree, ok := snapshot.TreeStoreOf(source)
	if !ok {
		t.Fatal("lakeFS fixture lost literal write capability")
	}
	commit, err := tree.ApplyTreeCommit(snapshot.TreeChangeSet{
		TargetRepository: repositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: base, ExpectedTargetCommit: base,
		Author: "agent:audit", RequestID: "request-audit-42", RuleID: "rule-audit-42",
		Message: "catalog audit event", Changes: []snapshot.TreeChange{{Path: "dataset.txt", Content: []byte("accepted catalog state")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pointer := filepath.Join(t.TempDir(), "source")
	if err := snapshot.WriteLakeFSStamp(pointer, string(repositoryID), dsn); err != nil {
		t.Fatal(err)
	}
	if err := fake.ForkStamps(filepath.Dir(pointer)); err != nil {
		t.Fatal(err)
	}
	_, forkDSN, err := snapshot.ReadLakeFSStamp(pointer)
	if err != nil {
		t.Fatal(err)
	}
	if forkDSN == dsn {
		t.Fatal("fork did not isolate its physical authority")
	}
	for _, coordinate := range []string{dsn, forkDSN} {
		opened, err := snapshot.OpenLakeFS(repositoryID, coordinate, fake.Credential())
		if err != nil {
			t.Fatal(err)
		}
		reader, ok := opened.(snapshot.CommitMetadataReader)
		if !ok {
			t.Fatal("lakeFS fixture lost immutable commit metadata capability")
		}
		metadata, err := reader.ReadCommitMetadata(commit)
		if err != nil {
			t.Fatal(err)
		}
		if metadata.Commit != commit || metadata.Author != "agent:audit" || metadata.Message != "catalog audit event" || metadata.RequestID != "request-audit-42" || metadata.RuleID != "rule-audit-42" {
			t.Fatalf("immutable Snapshot provenance changed: %#v", metadata)
		}
	}
	// The native history endpoint and point-read port must expose the same
	// accepted record rather than fabricating generic history labels.
	response, err := http.Get(fake.Origin() + "/api/v1/repositories/" + lakefsRepoFromDSN(dsn) + "/refs/main/commits?amount=1")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var history struct {
		Results []struct {
			ID       string            `json:"id"`
			Message  string            `json:"message"`
			Metadata map[string]string `json:"metadata"`
		} `json:"results"`
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("native history HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(&history); err != nil {
		t.Fatal(err)
	}
	if len(history.Results) != 1 || history.Results[0].ID != string(commit) || history.Results[0].Message != "catalog audit event" || history.Results[0].Metadata["kc.author"] != "agent:audit" || history.Results[0].Metadata["kc.command_id"] != "request-audit-42" || history.Results[0].Metadata["kc.rule_id"] != "rule-audit-42" {
		t.Fatalf("native history dropped accepted metadata: %#v", history)
	}
}
