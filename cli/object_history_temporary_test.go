package cli_test

import (
	"context"
	"fmt"
	"kc/client"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
	"testing"
)

func TestTemporaryHistoryContinuationDoesNotBorrowPublishedLabelGrant(t *testing.T) {
	_, url, repo := objectReferenceServer(t)
	admin := func(args ...string) kcRunResult { return kcRemote(t, url, "operator", args...) }
	for i, value := range []string{"one", "two"} {
		body(t, admin("writer", "put", "urn:kc:object:note/history", "--repo", repo, "--command-id", fmt.Sprintf("temporary-history-%d", i), "--value", fmt.Sprintf(`{"body":%q}`, value)))
	}
	body(t, admin("dataset", "define", "--dataset", "same-label", "--revision", "1", "--source", repo))
	body(t, admin("grant", "add", "--principal", "temporary-history", "--repo", repo, "--action", "object.history.read"))
	sourceGrant := asMap(t, body(t, admin("grant", "add", "--principal", "temporary-history", "--repo", repo, "--action", "file.read")))
	definition := dataset.Dataset{SetID: "same-label", Revision: 7, Sources: []dataset.DatasetSource{{Repository: kernel.RepositoryID(repo), Selector: snapshot.DefaultRef, Path: dataset.MountPath("source")}}}
	remote := productClient(t, url, "temporary-history")
	var first map[string]any
	if err := remote.Objects().Log(context.Background(), client.ObjectRequest{ObjectScope: client.ObjectScope{Definition: &definition}, Object: "note/history", Limit: 1}, client.RequestOptions{}, &first); err != nil {
		t.Fatal(err)
	}
	cursor, ok := first["continuation"].(string)
	if !ok || cursor == "" {
		t.Fatalf("temporary history has no continuation: %#v", first)
	}
	var next map[string]any
	if err := remote.Objects().Log(context.Background(), client.ObjectRequest{Continuation: cursor, Limit: 1}, client.RequestOptions{}, &next); err != nil {
		t.Fatalf("temporary continuation became named v7: %v", err)
	}
	if len(next["logs"].([]any)) != 1 {
		t.Fatalf("temporary continuation lost history: %#v", next)
	}
	body(t, admin("grant", "remove", "--id", sourceGrant["id"].(string)))
	body(t, admin("grant", "add", "--principal", "temporary-history", "--dataset", "same-label", "--action", "file.read"))
	if err := remote.Objects().Log(context.Background(), client.ObjectRequest{Continuation: cursor, Limit: 1}, client.RequestOptions{}, &next); kernel.CodeOf(err) != kernel.ErrForbidden {
		t.Fatalf("temporary cursor borrowed same-label publication grant: %v", err)
	}
}
