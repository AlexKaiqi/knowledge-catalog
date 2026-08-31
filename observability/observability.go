// Package observability defines non-canonical evidence produced when principals
// access versioned knowledge.
package observability

type Recorder interface {
	RecordAccess(AccessEvent) error
	RecordRetrieval(RetrievalEvent) error
	RecordRefine(RefineEvent) error
	RecordFeedback(FeedbackEvent) error
}
