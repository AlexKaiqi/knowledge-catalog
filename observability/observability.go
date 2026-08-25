// Package observability defines non-canonical evidence produced when principals
// access versioned knowledge.
package observability

type Recorder interface {
	RecordAccess(AccessEvent) error
	RecordFeedback(FeedbackEvent) error
}
