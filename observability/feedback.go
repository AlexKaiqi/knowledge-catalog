package observability

import (
	"fmt"
	"strings"
)

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
