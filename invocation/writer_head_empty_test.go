package invocation

import (
	"testing"

	"kc/home"
	"kc/invoke"
	"kc/kernel"
	"kc/snapshot"
)

type writerHeadProbeStore struct {
	snapshot.Store
	page  snapshot.DirectoryPage
	err   error
	heads int
	reads []snapshot.DirectoryRequest
}

func (s *writerHeadProbeStore) Head(string) (kernel.CommitID, error) {
	s.heads++
	if s.heads > 1 {
		return "advanced", nil
	}
	return "fixed", nil
}

func (s *writerHeadProbeStore) ReadDirectory(req snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
	s.reads = append(s.reads, req)
	return s.page, s.err
}

func invokeWriterHead(t *testing.T, store snapshot.Store) (any, error) {
	t.Helper()
	registry := snapshot.NewRegistry()
	if err := registry.Add(store); err != nil {
		t.Fatal(err)
	}
	return VerbWriterHead(&Invocation{WS: &home.Home{Store: registry}, Flags: map[string]invoke.FlagValue{
		"repo": string(store.ID()), "ref": "refs/heads/import",
	}})
}

func TestWriterHeadProvesOnlyItsFixedCommitEmpty(t *testing.T) {
	for _, nonempty := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "hidden-content"}[nonempty], func(t *testing.T) {
			store := &writerHeadProbeStore{
				Store: &registryLikeStore{id: "kr://acme/head"},
				page:  snapshot.DirectoryPage{Generation: "fixed", Exhausted: true},
			}
			if nonempty {
				store.page.Entries = []snapshot.DirectoryEntry{{Name: ".kc", Kind: "directory"}}
			}
			result, err := invokeWriterHead(t, store)
			if err != nil {
				t.Fatal(err)
			}
			row := result.(map[string]any)
			if row["commit"] != kernel.CommitID("fixed") || row["empty"] != !nonempty || store.heads != 1 || len(store.reads) != 1 {
				t.Fatalf("head proof: result=%#v heads=%d reads=%#v", row, store.heads, store.reads)
			}
			request := store.reads[0]
			if request.Commit != "fixed" || request.Directory != "" || request.Limit != 1 {
				t.Fatalf("emptiness must use one fixed root entry: %#v", request)
			}
		})
	}
}

func TestWriterHeadOmitsUnknownEmptyState(t *testing.T) {
	result, err := invokeWriterHead(t, &registryLikeStore{id: "kr://acme/head"})
	if err != nil {
		t.Fatal(err)
	}
	row := result.(map[string]any)
	if _, ok := row["empty"]; ok || row["commit"] != kernel.CommitID("root") {
		t.Fatalf("store without directory reads invented an empty state: %#v", row)
	}
}

func TestWriterHeadFailsClosedOnEmptyProbeFailure(t *testing.T) {
	store := &writerHeadProbeStore{Store: &registryLikeStore{id: "kr://acme/head"},
		err: kernel.Fail(kernel.ErrForbidden, "root denied")}
	result, err := invokeWriterHead(t, store)
	if kernel.CodeOf(err) != kernel.ErrForbidden || result != nil {
		t.Fatalf("failed root read must not produce a head proof: result=%#v error=%v", result, err)
	}
}
