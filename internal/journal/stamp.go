package journal

// WithStamp fills identity/correlation on events that left those fields empty.
// Writer / Catalog / ControlPlane call Finish without --as; the CLI stamps the call.
func WithStamp(inner Journal, as, requestID, ruleID string) Journal {
	if inner == nil {
		return nil
	}
	if as == "" && requestID == "" && ruleID == "" {
		return inner
	}
	return stamped{inner: inner, as: as, requestID: requestID, ruleID: ruleID}
}

type stamped struct {
	inner     Journal
	as        string
	requestID string
	ruleID    string
}

func (s stamped) Record(event Event) error {
	if event.As == "" {
		event.As = s.as
	}
	if event.RequestID == "" {
		event.RequestID = s.requestID
	}
	if event.RuleID == "" {
		event.RuleID = s.ruleID
	}
	return s.inner.Record(event)
}
