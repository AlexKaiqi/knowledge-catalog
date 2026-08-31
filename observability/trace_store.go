package observability

import (
	"sort"
	"time"
)

func (s *FileStore) Trace(traceID string) (TraceView, error) {
	access, err := s.Access(AccessQuery{TraceID: traceID})
	if err != nil {
		return TraceView{}, err
	}
	feedback, err := readJSONL[FeedbackEvent](s.FeedbackPath)
	if err != nil {
		return TraceView{}, err
	}
	refines, err := s.Refine(RefineQuery{TraceID: traceID})
	if err != nil {
		return TraceView{}, err
	}
	retrievals, err := s.Retrieval(RetrievalQuery{TraceID: traceID})
	if err != nil {
		return TraceView{}, err
	}
	view := TraceView{TraceID: traceID, Entries: []TraceEntry{}}
	for i := range access {
		event := access[i]
		view.Entries = append(view.Entries, TraceEntry{Kind: "access", OccurredAt: event.OccurredAt, Access: &event})
	}
	for i := range feedback {
		event := feedback[i]
		if event.Trace.TraceID != traceID && (event.SubmissionTrace == nil || event.SubmissionTrace.TraceID != traceID) {
			continue
		}
		view.Entries = append(view.Entries, TraceEntry{Kind: "feedback", OccurredAt: event.OccurredAt, Feedback: &event})
	}
	for i := range refines {
		event := refines[i]
		view.Entries = append(view.Entries, TraceEntry{Kind: "refine", OccurredAt: event.OccurredAt, Refine: &event})
	}
	for i := range retrievals {
		event := retrievals[i]
		view.Entries = append(view.Entries, TraceEntry{Kind: "retrieval", OccurredAt: event.OccurredAt, Retrieval: &event})
	}
	sort.SliceStable(view.Entries, func(i, j int) bool {
		return occurredBefore(view.Entries[i].OccurredAt, view.Entries[j].OccurredAt)
	})
	return view, nil
}

func occurredBefore(left, right string) bool {
	leftTime, leftErr := time.Parse(time.RFC3339Nano, left)
	rightTime, rightErr := time.Parse(time.RFC3339Nano, right)
	if leftErr == nil && rightErr == nil {
		return leftTime.Before(rightTime)
	}
	return left < right
}
