package service

import (
	"fmt"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
	"reflect"
	"testing"
)

func TestSnapshotDeliveredDerivedCursorAndDefaultLimit(t *testing.T) {
	items := make([]dataset.DatasetItem, 501)
	for i := range items {
		items[i] = dataset.DatasetItem{Kind: dataset.DatasetItemFile, Target: fmt.Sprintf("docs/file-%03d", i), Repository: "repo", Commit: "c1", File: "source"}
	}
	pin := dataset.ResolvedKnowledgeSet{SetID: "files", Revision: 1, PinID: "fixed"}
	view := NewFileView(ViewOptions{Delivered: true, Pin: pin, Items: items})
	first, err := view.List(FileDirectoryRequest{Path: "docs"})
	if err != nil || len(first.Entries) != 500 || first.Exhausted || first.Continuation == "" {
		t.Fatalf("derived default did not page: %#v %v", first, err)
	}
	next, err := view.List(FileDirectoryRequest{Path: "docs", Limit: 1, Continuation: first.Continuation})
	if err != nil || !next.Exhausted || !reflect.DeepEqual(next.Entries, []snapshot.DirectoryEntry{{Name: "file-500", Kind: "file"}}) {
		t.Fatalf("derived continuation lost its remaining child: %#v %v", next, err)
	}
	for _, test := range []struct {
		name string
		pin  dataset.ResolvedKnowledgeSet
		path string
	}{
		{"dataset", dataset.ResolvedKnowledgeSet{SetID: "other", Revision: 1, PinID: "fixed"}, "docs"},
		{"revision", dataset.ResolvedKnowledgeSet{SetID: "files", Revision: 2, PinID: "fixed"}, "docs"},
		{"pin", dataset.ResolvedKnowledgeSet{SetID: "files", Revision: 1, PinID: "other"}, "docs"},
		{"directory", pin, "other"},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := NewFileView(ViewOptions{Delivered: true, Pin: test.pin, Items: items})
			_, err := changed.List(FileDirectoryRequest{Path: test.path, Limit: 1, Continuation: first.Continuation})
			if kernel.CodeOf(err) != kernel.ErrUsageInvalid {
				t.Fatalf("cursor accepted a different basis: %v", err)
			}
		})
	}
	changed := NewFileView(ViewOptions{Delivered: true, Pin: pin, Items: items[:1]})
	if _, err := changed.List(FileDirectoryRequest{Path: "docs", Continuation: first.Continuation}); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("changed visibility silently reset the position: %v", err)
	}
}

func TestSnapshotDeliveredPagingChecksProviderProgress(t *testing.T) {
	for _, test := range []struct {
		name string
		page snapshot.DirectoryPage
		want kernel.ErrorCode
	}{
		{"empty advancing", snapshot.DirectoryPage{Entries: []snapshot.DirectoryEntry{}, Continuation: "next", Generation: "c1"}, ""},
		{"missing continuation", snapshot.DirectoryPage{Generation: "c1"}, kernel.ErrPreconditionFailed},
		{"oversized", snapshot.DirectoryPage{Entries: []snapshot.DirectoryEntry{{Name: "a", Kind: "file"}, {Name: "b", Kind: "file"}}, Exhausted: true, Generation: "c1"}, kernel.ErrPreconditionFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			view := NewFileView(ViewOptions{Delivered: true, Pin: dataset.ResolvedKnowledgeSet{SetID: "files", Revision: 1, PinID: "fixed"}, Mounts: []dataset.VirtualMount{{Path: "docs", Repository: "repo", Commit: "c1"}}, Directory: func(_ dataset.VirtualMount, _ string, limit int, continuation string) (snapshot.DirectoryPage, error) {
				calls++
				if limit != 1 {
					t.Fatalf("provider received an unbounded limit %d", limit)
				}
				if continuation == "next" {
					return snapshot.DirectoryPage{Entries: []snapshot.DirectoryEntry{{Name: "a", Kind: "file"}}, Exhausted: true, Generation: "c1"}, nil
				}
				return test.page, nil
			}})
			page, err := view.List(FileDirectoryRequest{Path: "docs", Limit: 1})
			if kernel.CodeOf(err) != test.want {
				t.Fatalf("provider progress: page=%#v err=%v want=%s", page, err, test.want)
			}
			if test.want == "" {
				if page.Exhausted || page.Continuation == "" || len(page.Entries) != 0 {
					t.Fatalf("advancing empty page lost its continuation: %#v", page)
				}
				next, err := view.List(FileDirectoryRequest{Path: "docs", Limit: 1, Continuation: page.Continuation})
				if err != nil || !next.Exhausted || len(next.Entries) != 1 || calls != 2 {
					t.Fatalf("empty-page continuation did not reach the child: %#v %v calls=%d", next, err, calls)
				}
			}
		})
	}
}

func TestSnapshotDeliveredPagingRejectsRepeatedProviderPosition(t *testing.T) {
	view := NewFileView(ViewOptions{Delivered: true, Pin: dataset.ResolvedKnowledgeSet{SetID: "files", Revision: 1, PinID: "fixed"}, Mounts: []dataset.VirtualMount{{Path: "docs", Repository: "repo", Commit: "c1"}}, Directory: func(dataset.VirtualMount, string, int, string) (snapshot.DirectoryPage, error) {
		return snapshot.DirectoryPage{Entries: []snapshot.DirectoryEntry{{Name: "a", Kind: "file"}}, Continuation: "stuck", Generation: "c1"}, nil
	}})
	first, err := view.List(FileDirectoryRequest{Path: "docs", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = view.List(FileDirectoryRequest{Path: "docs", Limit: 1, Continuation: first.Continuation})
	if kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("repeated source position was accepted: %v", err)
	}
}
