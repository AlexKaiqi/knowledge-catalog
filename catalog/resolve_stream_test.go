package catalog_test

import (
	"testing"

	"kc/catalog"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/repository"
)

func TestResolveViewPinsStreamCutsWithoutReadingPayload(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://acme/personal/kai")
	store := repository.NewStore()
	if err := store.Add(repo); err != nil {
		t.Fatal(err)
	}
	testkit.BindJSONL(t, store, repo)
	stream, ok := store.GetStream(repo.ID())
	if !ok {
		t.Fatal("bound stream")
	}
	if _, err := stream.Append("runs", []repository.AppendEntry{{
		EventID: "r1", Payload: map[string]any{"secret": "payload-must-not-enter-catalog"},
	}}, stream.StreamCursor("runs")); err != nil {
		t.Fatal(err)
	}
	cat := testkit.OpenCatalog(t, store)
	if _, err := cat.DefineWorkspace("desk", 1, []catalog.WorkspaceSource{
		{Repository: repo.ID(), Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}

	first, err := cat.ResolveWorkspace("desk")
	if err != nil {
		t.Fatal(err)
	}
	cut := first.AppendCuts[repo.ID()]["runs"]
	if cut == "" || cut != stream.StreamCursor("runs") {
		t.Fatalf("cut %#v live %s", first.AppendCuts, stream.StreamCursor("runs"))
	}

	if _, err := stream.Append("runs", []repository.AppendEntry{{
		EventID: "r2", Payload: map[string]any{"n": 2},
	}}, stream.StreamCursor("runs")); err != nil {
		t.Fatal(err)
	}
	if first.AppendCuts[repo.ID()]["runs"] == stream.StreamCursor("runs") {
		t.Fatal("frozen cut must not follow live append")
	}
	next, err := cat.ResolveWorkspace("desk")
	if err != nil {
		t.Fatal(err)
	}
	if next.AppendCuts[repo.ID()]["runs"] != stream.StreamCursor("runs") {
		t.Fatal(next.AppendCuts)
	}
	if first.PinID == "" || first.PinID == next.PinID {
		t.Fatalf("PinID must change when AppendCuts change: %s -> %s", first.PinID, next.PinID)
	}
	check := cat.CheckResolved(first)
	if check.Outcome != "PASSED" {
		t.Fatal(check)
	}
}

func TestCheckResolvedReportsUnmountedStream(t *testing.T) {
	s := setupFed(t)
	if _, err := s.catalog.DefineWorkspace("v", 1, []catalog.WorkspaceSource{
		{Repository: "kr://acme/public/core", Selector: "refs/heads/main"},
	}); err != nil {
		t.Fatal(err)
	}
	resolved, err := s.catalog.ResolveWorkspace("v")
	if err != nil {
		t.Fatal(err)
	}
	resolved.AppendCuts = map[kernel.RepositoryID]map[string]string{
		"kr://acme/public/core": {"runs": "1"},
	}
	check := s.catalog.CheckResolved(resolved)
	if check.Outcome != "FAILED" || check.Issues[0].Code != kernel.ErrUsageInvalid {
		t.Fatal(check)
	}
}
