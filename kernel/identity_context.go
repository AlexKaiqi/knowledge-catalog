package kernel

import (
	"fmt"
	"strings"
	"unicode"
)

// IdentityContext is the principal evidence shape shared by authorization,
// access evidence, and serving views (CAP-ORG-01 B13, ruling A: the shape
// lives on the protocol base so the object layer consumes a narrow port
// instead of the app-layer observability capability).
type IdentityContext struct {
	Principal  string `json:"principal"`
	OnBehalfOf string `json:"onBehalfOf,omitempty"`
}

// Validate checks the identity shape.
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

// TraceContext is the correlation-evidence shape (trace/span identifiers).
type TraceContext struct {
	TraceID      string `json:"traceId,omitempty"`
	SpanID       string `json:"spanId,omitempty"`
	ParentSpanID string `json:"parentSpanId,omitempty"`
}

// Validate checks the trace shape.
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
