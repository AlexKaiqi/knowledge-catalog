package journal

// WithStamp fills identity/correlation on events that left those fields empty.
// Writer / Catalog / ControlPlane call Finish without --as; the CLI stamps the call.
func WithStamp(inner Journal, as, requestID, ruleID string) Journal {
	return WithContext(inner, Stamp{Principal: as, RequestID: requestID, RuleID: ruleID})
}

type Stamp struct {
	Principal    string
	OnBehalfOf   string
	RequestID    string
	TraceID      string
	SpanID       string
	ParentSpanID string
	SessionID    string
	RuleID       string
}

// WithContext fills identity, delegation and trace correlation on events that
// do not already provide them. It does not authenticate either identity.
func WithContext(inner Journal, stamp Stamp) Journal {
	if inner == nil {
		return nil
	}
	if stamp == (Stamp{}) {
		return inner
	}
	return stamped{inner: inner, stamp: stamp}
}

type stamped struct {
	inner Journal
	stamp Stamp
}

func (s stamped) Record(event Event) error {
	if event.Principal == "" {
		event.Principal = s.stamp.Principal
	}
	if event.As == "" {
		event.As = event.Principal
	}
	if event.OnBehalfOf == "" {
		event.OnBehalfOf = s.stamp.OnBehalfOf
	}
	if event.RequestID == "" {
		event.RequestID = s.stamp.RequestID
	}
	if event.TraceID == "" {
		event.TraceID = s.stamp.TraceID
	}
	if event.SpanID == "" {
		event.SpanID = s.stamp.SpanID
	}
	if event.ParentSpanID == "" {
		event.ParentSpanID = s.stamp.ParentSpanID
	}
	if event.SessionID == "" {
		event.SessionID = s.stamp.SessionID
	}
	if event.RuleID == "" {
		event.RuleID = s.stamp.RuleID
	}
	return s.inner.Record(event)
}
