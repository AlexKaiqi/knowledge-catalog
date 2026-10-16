package snapshot_test

import (
	"errors"
	"testing"

	"kc/kernel"
	"kc/snapshot"
)

type emptyProbeStore struct {
	snapshot.Store
	read func(snapshot.DirectoryRequest) (snapshot.DirectoryPage, error)
}

func (s emptyProbeStore) ReadDirectory(request snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
	return s.read(request)
}

func TestProbeEmptyTreeUsesFixedBoundedDirectoryEvidence(t *testing.T) {
	for _, test := range []struct {
		name  string
		pages []snapshot.DirectoryPage
		empty bool
	}{
		{"empty", []snapshot.DirectoryPage{{Generation: "basis", Exhausted: true}}, true},
		{"file without locator", []snapshot.DirectoryPage{{Generation: "basis", Exhausted: true, Entries: []snapshot.DirectoryEntry{{Name: "raw.json", Kind: "file"}}}}, false},
		{"hidden metadata", []snapshot.DirectoryPage{{Generation: "basis", Exhausted: true, Entries: []snapshot.DirectoryEntry{{Name: ".kc", Kind: "directory"}}}}, false},
		{"directory", []snapshot.DirectoryPage{{Generation: "basis", Entries: []snapshot.DirectoryEntry{{Name: "objects", Kind: "directory"}}, Continuation: "more"}}, false},
		{"empty advancing pages", []snapshot.DirectoryPage{{Generation: "basis", Continuation: "next"}, {Generation: "basis", Exhausted: true}}, true},
		{"file after empty page", []snapshot.DirectoryPage{{Generation: "basis", Continuation: "next"}, {Generation: "basis", Exhausted: true, Entries: []snapshot.DirectoryEntry{{Name: "README.md", Kind: "file"}}}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			store := emptyProbeStore{read: func(request snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
				if request.Commit != "basis" || request.Directory != "" || request.Limit != 1 {
					t.Fatalf("probe did not fix and bound the root request: %#v", request)
				}
				if calls > 0 && request.Continuation != test.pages[calls-1].Continuation {
					t.Fatalf("probe lost source continuation: %#v", request)
				}
				page := test.pages[calls]
				calls++
				return page, nil
			}}
			empty, known, err := snapshot.ProbeEmptyTree(store, "basis")
			if err != nil || !known || empty != test.empty || calls != len(test.pages) {
				t.Fatalf("probe = empty:%t known:%t err:%v calls:%d", empty, known, err, calls)
			}
		})
	}
}

func TestProbeEmptyTreeRejectsInvalidProviderEvidence(t *testing.T) {
	for _, test := range []struct {
		name  string
		pages []snapshot.DirectoryPage
	}{
		{"missing generation", []snapshot.DirectoryPage{{Exhausted: true}}},
		{"wrong generation", []snapshot.DirectoryPage{{Generation: "other", Exhausted: true}}},
		{"missing continuation", []snapshot.DirectoryPage{{Generation: "basis"}}},
		{"exhausted with continuation", []snapshot.DirectoryPage{{Generation: "basis", Exhausted: true, Continuation: "next"}}},
		{"oversized page", []snapshot.DirectoryPage{{Generation: "basis", Exhausted: true, Entries: []snapshot.DirectoryEntry{{Name: "a", Kind: "file"}, {Name: "b", Kind: "file"}}}}},
		{"repeated continuation", []snapshot.DirectoryPage{{Generation: "basis", Continuation: "same"}, {Generation: "basis", Continuation: "same"}}},
		{"cyclic continuation", []snapshot.DirectoryPage{{Generation: "basis", Continuation: "a"}, {Generation: "basis", Continuation: "b"}, {Generation: "basis", Continuation: "a"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			store := emptyProbeStore{read: func(snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
				if calls >= len(test.pages) {
					t.Fatal("probe retried malformed evidence indefinitely")
				}
				page := test.pages[calls]
				calls++
				return page, nil
			}}
			empty, _, err := snapshot.ProbeEmptyTree(store, "basis")
			if empty || kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
				t.Fatalf("invalid evidence proved empty: empty=%t err=%v", empty, err)
			}
		})
	}
}

func TestProbeEmptyTreePreservesUnknownCapabilityAndErrors(t *testing.T) {
	// Embedding only Store intentionally hides any optional directory or tree
	// capability. Probing must not turn that absence into an unbounded ListFiles.
	withoutDirectory := struct{ snapshot.Store }{}
	if empty, known, err := snapshot.ProbeEmptyTree(withoutDirectory, "basis"); err != nil || empty || known {
		t.Fatalf("missing capability became empty: empty=%t known=%t err=%v", empty, known, err)
	}
	failure := kernel.Fail(kernel.ErrForbidden, "directory access denied")
	store := emptyProbeStore{read: func(snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
		return snapshot.DirectoryPage{}, failure
	}}
	if empty, _, err := snapshot.ProbeEmptyTree(store, "basis"); empty || !errors.Is(err, failure) {
		t.Fatalf("authority error became empty or changed identity: empty=%t err=%v", empty, err)
	}
	if empty, _, err := snapshot.ProbeEmptyTree(store, ""); empty || kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("unfixed probe accepted: empty=%t err=%v", empty, err)
	}
}
