package object

import (
	"context"
	"kc/kernel"
	"time"
)

const DefaultHistoryScanBudget = 256
const MaxHistoryScanBudget = 1024

const DefaultHistoryScanDuration = time.Second
const MaxHistoryScanDuration = 4 * time.Second
const HistoryScanRequestTimeout = 5 * time.Second

// HistoryScanProgress is an unfinished equal-state segment on a fixed
// first-parent chain. Service continuations must retain it as trusted metadata;
// it is neither a client-supplied proof nor an authorization capability.
type HistoryScanProgress struct {
	Algorithm  string              `json:"algorithm"`
	Object     ObjectID            `json:"object"`
	Repository kernel.RepositoryID `json:"repository"`
	Binding    kernel.Digest       `json:"binding"`
	Start      kernel.CommitID     `json:"start"`
	Position   kernel.CommitID     `json:"position"`
	Pending    ObjectRevision      `json:"pending"`
	State      kernel.Digest       `json:"state"`
	After      kernel.CommitID     `json:"after,omitempty"`
	Skipping   bool                `json:"skipping,omitempty"`
}

type HistoryScanQuery struct {
	Limit      int
	Budget     int
	After      kernel.CommitID
	Progress   *HistoryScanProgress
	TimeBudget time.Duration
}

type HistoryScanPage struct {
	Revisions []ObjectRevision     `json:"revisions"`
	Scanned   int                  `json:"scanned"`
	Complete  bool                 `json:"complete"`
	Progress  *HistoryScanProgress `json:"progress,omitempty"`
}

// HistoryScanner bounds examined commits independently of returned revisions.
// An empty unfinished page is valid only with an advanced scan position.
type HistoryScanner interface {
	ScanHistory(context.Context, ObjectID, kernel.CommitID, HistoryScanQuery) (HistoryScanPage, error)
}

// Validate checks an unfinished segment against the server's fixed source.
// It does not verify permission or make caller-provided progress trusted.
func (p HistoryScanProgress) Validate(id ObjectID, source ReadSource, after kernel.CommitID) error {
	if p.Algorithm != LocalStateAlgorithm || p.Object != id || p.Repository != source.Repository || p.Binding != source.Binding || p.Start != source.Commit || p.After != after || after == "" && p.Skipping || p.Position == "" || p.Pending.Commit != p.Position || p.State == "" || p.Pending.Status != StatusResolved && p.Pending.Status != StatusRemoved {
		return kernel.Fail(kernel.ErrUsageInvalid, "history progress does not match its fixed source, object or state algorithm")
	}
	return nil
}
