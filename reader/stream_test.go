package reader_test

import (
	"testing"

	"kc/internal/testkit"
	"kc/kernel"
	"kc/reader"
	"kc/repository"
)

func TestQueryStreamContinueLookupWindow(t *testing.T) {
	s := testkit.NewSetup(t, "")
	for i, id := range []string{"a", "b", "c"} {
		if _, err := s.Writer.Append("e-"+id, repository.AppendEntries{
			TargetRepository: s.RepositoryID,
			StreamRef:        "runs",
			Entries: []repository.AppendEntry{{
				EventID:    id,
				Payload:    map[string]any{"n": i},
				ObservedAt: "2026-01-0" + itoa(i+1) + "T00:00:00Z",
			}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	all, err := s.Reader.QueryStream(s.RepositoryID, reader.StreamReadRequest{StreamRef: "runs"})
	if err != nil {
		t.Fatal(err)
	}
	if all.Face != reader.StreamContinue || all.Completeness != reader.StreamDurable {
		t.Fatalf("%#v", all)
	}
	if all.Cursor != s.Stream.StreamCursor("runs") || all.HasMore || len(all.Records) != 3 {
		t.Fatalf("%#v", all)
	}

	page, err := s.Reader.QueryStream(s.RepositoryID, reader.StreamReadRequest{
		StreamRef: "runs", Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Face != reader.StreamContinue || len(page.Records) != 1 || page.Records[0].EventID != "a" {
		t.Fatalf("%#v", page)
	}
	if page.NextCursor == "" || !page.HasMore {
		t.Fatalf("next %#v", page)
	}
	rest, err := s.Reader.QueryStream(s.RepositoryID, reader.StreamReadRequest{
		StreamRef: "runs", FromCursor: page.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rest.Records) != 2 || rest.Records[0].EventID != "b" || rest.Records[1].EventID != "c" || rest.HasMore {
		t.Fatalf("%#v", rest)
	}

	hit, err := s.Reader.QueryStream(s.RepositoryID, reader.StreamReadRequest{
		StreamRef: "runs", EventID: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if hit.Face != reader.StreamLookup || len(hit.Records) != 1 || hit.Records[0].EventID != "b" {
		t.Fatalf("%#v", hit)
	}
	_, err = s.Reader.QueryStream(s.RepositoryID, reader.StreamReadRequest{
		StreamRef: "runs", EventID: "missing",
	})
	testkit.ExpectCode(t, err, kernel.ErrKnowledgeRefUnresolved)

	win, err := s.Reader.QueryStream(s.RepositoryID, reader.StreamReadRequest{
		StreamRef: "runs", FromRecordedAt: "2026-01-02T00:00:00Z", ToRecordedAt: "2026-01-02T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if win.Face != reader.StreamWindow || len(win.Records) != 1 || win.Records[0].EventID != "b" {
		t.Fatalf("%#v", win)
	}

	_, err = s.Reader.QueryStream(s.RepositoryID, reader.StreamReadRequest{
		StreamRef: "runs", EventID: "b", Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Reader.QueryStream(s.RepositoryID, reader.StreamReadRequest{
		StreamRef: "runs", EventID: "b", FromRecordedAt: "2026-01-01T00:00:00Z",
	})
	testkit.ExpectCode(t, err, kernel.ErrPreconditionFailed)

	_, err = s.Reader.QueryStream(s.RepositoryID, reader.StreamReadRequest{
		StreamRef: "runs", Cut: "2",
	})
	testkit.ExpectCode(t, err, kernel.ErrCapabilityUnsatisfied)
	_, err = s.Reader.QueryStream(s.RepositoryID, reader.StreamReadRequest{
		StreamRef: "runs", Clauses: []reader.SearchClause{reader.SearchEQ("n", "1")},
	})
	testkit.ExpectCode(t, err, kernel.ErrCapabilityUnsatisfied)
}

func itoa(n int) string {
	return string(rune('0' + n))
}
