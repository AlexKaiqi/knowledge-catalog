// Package observability defines the non-canonical evidence produced when
// principals access versioned knowledge. Authentication is replaceable; local
// mode provides a shape-validating pass-through implementation.
package observability

import (
	"fmt"
	"strings"
	"unicode"

	"kc/kernel"
)

type Recorder interface {
	RecordAccess(AccessEvent) error
	RecordFeedback(FeedbackEvent) error
}

// Authenticator turns a transport-specific identity assertion into the
// identity context used by authorization and evidence. Deployments can replace
// the pass-through implementation without changing the access-event schema.
type Authenticator interface {
	Authenticate(IdentityAssertion) (IdentityContext, error)
}

type IdentityAssertion struct {
	Principal  string
	OnBehalfOf string
}

// PassThroughAuthenticator is the local-mode authentication policy. It only
// validates shape; it does not prove either identity or the delegation.
type PassThroughAuthenticator struct{}

func (PassThroughAuthenticator) Authenticate(assertion IdentityAssertion) (IdentityContext, error) {
	context := IdentityContext{Principal: assertion.Principal, OnBehalfOf: assertion.OnBehalfOf}
	return context, context.Validate()
}

type IdentityContext struct {
	Principal  string `json:"principal"`
	OnBehalfOf string `json:"onBehalfOf,omitempty"`
}

func (c IdentityContext) Validate() error {
	if err := validateIdentity("principal", c.Principal, true); err != nil {
		return err
	}
	return validateIdentity("onBehalfOf", c.OnBehalfOf, false)
}

func validateIdentity(name, value string, required bool) error {
	if value == "" {
		if required {
			return fmt.Errorf("%s is required", name)
		}
		return nil
	}
	if strings.TrimSpace(value) != value || len(value) > 256 {
		return fmt.Errorf("%s must be a trimmed identity of at most 256 bytes", name)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s must not contain control characters", name)
		}
	}
	return nil
}

type TraceContext struct {
	TraceID      string `json:"traceId,omitempty"`
	SpanID       string `json:"spanId,omitempty"`
	ParentSpanID string `json:"parentSpanId,omitempty"`
	SessionID    string `json:"sessionId,omitempty"`
}

func (c TraceContext) Validate() error {
	for name, value := range map[string]string{
		"traceId": c.TraceID, "spanId": c.SpanID,
		"parentSpanId": c.ParentSpanID, "sessionId": c.SessionID,
	} {
		if value == "" {
			continue
		}
		if len(value) > 128 {
			return fmt.Errorf("%s is too long", name)
		}
		for _, r := range value {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' || r == ':' {
				continue
			}
			return fmt.Errorf("%s must be a correlation token", name)
		}
	}
	if (c.SpanID != "" || c.ParentSpanID != "") && c.TraceID == "" {
		return fmt.Errorf("spanId and parentSpanId require traceId")
	}
	return nil
}

type KnowledgeAccess struct {
	KnowledgeRef kernel.PinnedKnowledgeRef `json:"knowledgeRef"`
	Address      *kernel.Address           `json:"address,omitempty"`
}

type FileAccess struct {
	FileRef kernel.FileRef `json:"fileRef"`
}

// SnapshotAccess records a bulk read whose smallest honest target is a pinned
// repository snapshot, such as materializing a whole Workspace checkout.
type SnapshotAccess struct {
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
}

type AccessEvent struct {
	OccurredAt string            `json:"occurredAt"`
	Identity   IdentityContext   `json:"identity"`
	Trace      TraceContext      `json:"trace,omitempty"`
	Action     string            `json:"action"`
	RequestID  string            `json:"requestId,omitempty"`
	Workspace  string            `json:"workspace,omitempty"`
	PinID      string            `json:"pinId,omitempty"`
	Decision   string            `json:"decision"`
	RuleID     string            `json:"ruleId,omitempty"`
	Result     string            `json:"result"`
	Knowledge  []KnowledgeAccess `json:"knowledge,omitempty"`
	Files      []FileAccess      `json:"files,omitempty"`
	Snapshots  []SnapshotAccess  `json:"snapshots,omitempty"`
	Stream     string            `json:"stream,omitempty"`
	Cursor     string            `json:"cursor,omitempty"`
	Error      map[string]any    `json:"error,omitempty"`
}

func (e AccessEvent) Validate() error {
	if err := e.Identity.Validate(); err != nil {
		return err
	}
	if err := e.Trace.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(e.Action) == "" {
		return fmt.Errorf("action is required")
	}
	if e.Decision != "ALLOW" && e.Decision != "DENY" {
		return fmt.Errorf("decision must be ALLOW or DENY")
	}
	if e.Result != "RESOLVED" && e.Result != "ERROR" {
		return fmt.Errorf("result must be RESOLVED or ERROR")
	}
	for _, target := range e.Knowledge {
		ref := target.KnowledgeRef
		if ref.Repository == "" || ref.Commit == "" || ref.Object == "" {
			return fmt.Errorf("knowledge access requires repository, commit, and object")
		}
	}
	for _, target := range e.Files {
		if target.FileRef.Repository == "" || target.FileRef.Commit == "" || target.FileRef.Path == "" {
			return fmt.Errorf("file access requires repository, commit, and path")
		}
	}
	for _, target := range e.Snapshots {
		if target.Repository == "" || target.Commit == "" {
			return fmt.Errorf("snapshot access requires repository and commit")
		}
	}
	return nil
}

type FeedbackEvent struct {
	OccurredAt string          `json:"occurredAt"`
	Identity   IdentityContext `json:"identity"`
	Trace      TraceContext    `json:"trace"`
	Workspace  string          `json:"workspace,omitempty"`
	Outcome    string          `json:"outcome"`
	Message    string          `json:"message,omitempty"`
}

func (e FeedbackEvent) Validate() error {
	if err := e.Identity.Validate(); err != nil {
		return err
	}
	if err := e.Trace.Validate(); err != nil {
		return err
	}
	if e.Trace.TraceID == "" {
		return fmt.Errorf("feedback requires traceId")
	}
	if strings.TrimSpace(e.Outcome) == "" {
		return fmt.Errorf("feedback outcome is required")
	}
	return nil
}

type AccessQuery struct {
	Principal  string
	OnBehalfOf string
	Action     string
	TraceID    string
	Repository kernel.RepositoryID
	Object     kernel.ObjectID
	Limit      int
}

type KnowledgeHit struct {
	KnowledgeRef    kernel.PinnedKnowledgeRef `json:"knowledgeRef"`
	Address         *kernel.Address           `json:"address,omitempty"`
	Hits            int                       `json:"hits"`
	FirstAccessedAt string                    `json:"firstAccessedAt"`
	LastAccessedAt  string                    `json:"lastAccessedAt"`
	Principals      map[string]int            `json:"principals"`
	OnBehalfOf      map[string]int            `json:"onBehalfOf,omitempty"`
}

type TraceEntry struct {
	Kind       string         `json:"kind"`
	OccurredAt string         `json:"occurredAt"`
	Access     *AccessEvent   `json:"access,omitempty"`
	Feedback   *FeedbackEvent `json:"feedback,omitempty"`
}

type TraceView struct {
	TraceID string       `json:"traceId"`
	Entries []TraceEntry `json:"entries"`
}
