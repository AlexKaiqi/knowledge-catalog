package observability

import (
	"fmt"
	"unicode"
)

type TraceContext struct {
	TraceID      string `json:"traceId,omitempty"`
	SpanID       string `json:"spanId,omitempty"`
	ParentSpanID string `json:"parentSpanId,omitempty"`
}

func (c TraceContext) Validate() error {
	for name, value := range map[string]string{
		"traceId": c.TraceID, "spanId": c.SpanID,
		"parentSpanId": c.ParentSpanID,
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
