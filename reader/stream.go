package reader

import (
	"strconv"
	"strings"

	"kc/kernel"
	"kc/repository"
)

const (
	StreamContinue StreamFace = "continue"
	StreamWindow   StreamFace = "window"
	StreamLookup   StreamFace = "lookup"
	StreamSearch   StreamFace = "search"

	StreamDurable StreamCompleteness = "durable"
	StreamIndexed StreamCompleteness = "indexed"
)

// StreamFace is the question a caller is asking of a stream.
// Callers name a face (or enough fields to infer one). They do not name a store.
type StreamFace string

// StreamCompleteness says what a zero or partial result is allowed to mean.
// durable: holes are forbidden in the returned range; cache miss is invisible.
// indexed: candidate set at a stated basis; lag is allowed and must be reported.
type StreamCompleteness string

// StreamReadRequest is the user-facing read of an Append stream.
// Callers do not name a store.
//
// Need first:
//
//	continue  fromCursor + limit → next complete records (contiguous, durable)
//	lookup    eventId → one record or UNRESOLVED
//
// Later: window (audit), search (indexed), cut (pinned generation).
// Cursor tokens are opaque: pass NextCursor back; do not arithmetic.
type StreamReadRequest struct {
	StreamRef      string         `json:"streamRef"`
	Face           StreamFace     `json:"face,omitempty"`
	Cut            string         `json:"cut,omitempty"`
	FromCursor     string         `json:"fromCursor,omitempty"`
	Limit          int            `json:"limit,omitempty"`
	EventID        string         `json:"eventId,omitempty"`
	FromRecordedAt string         `json:"fromRecordedAt,omitempty"`
	ToRecordedAt   string         `json:"toRecordedAt,omitempty"`
	Clauses        []SearchClause `json:"clauses,omitempty"`
}

// StreamPage is one answer. HeadCursor is the live durable end (writer CAS).
// NextCursor is the continue token. Completeness is what "empty" may claim.
type StreamPage struct {
	Repository   kernel.RepositoryID       `json:"repository"`
	StreamRef    string                    `json:"streamRef"`
	Face         StreamFace                `json:"face"`
	Cut          string                    `json:"cut,omitempty"`
	HeadCursor   string                    `json:"headCursor"`
	NextCursor   string                    `json:"nextCursor"`
	HasMore      bool                      `json:"hasMore"`
	Completeness StreamCompleteness        `json:"completeness"`
	Cursor       string                    `json:"cursor"`
	Records      []repository.StreamRecord `json:"records"`
}

// ReadStream is unbounded continue from the start of the live stream.
// Prefer QueryStream. This keeps T5 / existing CLI dumps working.
func (r *Reader) ReadStream(repositoryID kernel.RepositoryID, streamRef string) (slice repository.StreamSlice, err error) {
	page, err := r.QueryStream(repositoryID, StreamReadRequest{StreamRef: streamRef})
	if err != nil {
		return repository.StreamSlice{}, err
	}
	return repository.StreamSlice{
		Repository: page.Repository,
		StreamRef:  page.StreamRef,
		Cursor:     page.Cursor,
		Records:    page.Records,
	}, nil
}

// QueryStream answers one stream face. It does not name JSONL, Redis, or Iceberg.
func (r *Reader) QueryStream(repositoryID kernel.RepositoryID, req StreamReadRequest) (page StreamPage, err error) {
	defer func() {
		err = r.note("stream", map[string]any{
			"repositoryId": string(repositoryID),
			"stream":       req.StreamRef,
			"face":         string(page.Face),
		}, err)
	}()
	if strings.TrimSpace(req.StreamRef) == "" {
		return StreamPage{}, kernel.Fail(kernel.ErrPreconditionFailed, "streamRef is required")
	}
	face, err := resolveStreamFace(req)
	if err != nil {
		return StreamPage{}, err
	}
	req.Face = face
	if req.Cut != "" {
		return StreamPage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "stream cut is not assembled; continue against the live head or omit --cut")
	}
	if face == StreamSearch {
		return StreamPage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "stream search is not assembled; continue, window, or lookup read durable records")
	}
	stream, err := r.store.RequireStream(repositoryID, kernel.ErrKnowledgeRefUnresolved)
	if err != nil {
		return StreamPage{}, err
	}
	all := stream.ReadStream(req.StreamRef)
	page = StreamPage{
		Repository:   all.Repository,
		StreamRef:    req.StreamRef,
		Face:         face,
		HeadCursor:   all.Cursor,
		Cursor:       all.Cursor,
		Completeness: StreamDurable,
	}
	switch face {
	case StreamLookup:
		return lookupStream(page, all.Records, req.EventID)
	case StreamWindow:
		return windowStream(page, all.Records, req.FromRecordedAt, req.ToRecordedAt)
	default:
		return continueStream(page, all.Records, req.FromCursor, req.Limit)
	}
}

func resolveStreamFace(req StreamReadRequest) (StreamFace, error) {
	var found []StreamFace
	if strings.TrimSpace(req.EventID) != "" {
		found = append(found, StreamLookup)
	}
	if req.FromRecordedAt != "" || req.ToRecordedAt != "" {
		found = append(found, StreamWindow)
	}
	if len(req.Clauses) > 0 {
		found = append(found, StreamSearch)
	}
	if req.Face != "" {
		switch req.Face {
		case StreamContinue, StreamWindow, StreamLookup, StreamSearch:
		default:
			return "", kernel.Fail(kernel.ErrPreconditionFailed, "unknown stream face %s", req.Face)
		}
		if len(found) > 0 && found[0] != req.Face {
			return "", kernel.Fail(kernel.ErrPreconditionFailed, "stream face %s does not match the given fields", req.Face)
		}
		if len(found) > 1 {
			return "", kernel.Fail(kernel.ErrPreconditionFailed, "stream read names more than one face")
		}
		return req.Face, nil
	}
	if len(found) > 1 {
		return "", kernel.Fail(kernel.ErrPreconditionFailed, "stream read names more than one face; use continue, window, or lookup")
	}
	if len(found) == 1 {
		return found[0], nil
	}
	return StreamContinue, nil
}

func lookupStream(page StreamPage, records []repository.StreamRecord, eventID string) (StreamPage, error) {
	for _, rec := range records {
		if rec.EventID == eventID {
			page.Records = []repository.StreamRecord{rec}
			page.NextCursor = page.HeadCursor
			return page, nil
		}
	}
	return StreamPage{}, kernel.Fail(kernel.ErrKnowledgeRefUnresolved, "event %s is unresolved in stream %s", eventID, page.StreamRef)
}

func windowStream(page StreamPage, records []repository.StreamRecord, from, to string) (StreamPage, error) {
	var out []repository.StreamRecord
	for _, rec := range records {
		if from != "" && rec.RecordedAt < from {
			continue
		}
		if to != "" && rec.RecordedAt > to {
			continue
		}
		out = append(out, rec)
	}
	page.Records = out
	page.NextCursor = page.HeadCursor
	return page, nil
}

func continueStream(page StreamPage, records []repository.StreamRecord, fromCursor string, limit int) (StreamPage, error) {
	skip, err := cursorSkip(fromCursor)
	if err != nil {
		return StreamPage{}, err
	}
	if skip > len(records) {
		skip = len(records)
	}
	rest := records[skip:]
	if limit > 0 && len(rest) > limit {
		rest = rest[:limit]
	}
	page.Records = rest
	page.NextCursor = cursorAfter(skip + len(rest))
	page.HasMore = page.NextCursor != page.HeadCursor
	return page, nil
}

func cursorSkip(cursor string) (int, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(cursor)
	if err != nil || n < 0 {
		return 0, kernel.Fail(kernel.ErrPreconditionFailed, "fromCursor is not a valid stream cursor")
	}
	return n, nil
}

func cursorAfter(n int) string {
	return strconv.Itoa(n)
}
