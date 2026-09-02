package observability

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"time"

	"kc/kernel"
)

type accessCursor struct {
	OccurredAt string `json:"occurredAt"`
	EvidenceID string `json:"evidenceId"`
}

func (s *FileStore) GetAccess(ctx context.Context, evidenceID string) (AccessEvent, bool, error) {
	if err := ctx.Err(); err != nil {
		return AccessEvent{}, false, err
	}
	if evidenceID == "" {
		return AccessEvent{}, false, nil
	}
	all, err := readJSONL[AccessEvent](s.AccessPath)
	if err != nil {
		return AccessEvent{}, false, err
	}
	for _, event := range all {
		if event.EvidenceID == evidenceID {
			return event, true, nil
		}
	}
	return AccessEvent{}, false, nil
}

func (s *FileStore) Access(ctx context.Context, query AccessQuery) (AccessPage, error) {
	if err := ctx.Err(); err != nil {
		return AccessPage{}, err
	}
	all, err := readJSONL[AccessEvent](s.AccessPath)
	if err != nil {
		return AccessPage{}, err
	}
	matched, err := matchingAccess(all, query)
	if err != nil {
		return AccessPage{}, err
	}
	cursor, err := decodeAccessCursor(query.Continuation)
	if err != nil {
		return AccessPage{}, err
	}
	if cursor != nil {
		filtered := make([]AccessEvent, 0, len(matched))
		for _, event := range matched {
			if accessBeforeCursor(event, *cursor) {
				filtered = append(filtered, event)
			}
		}
		matched = filtered
	}
	sortAccessEvents(matched)
	page := AccessPage{
		Entries:         []AccessEvent{},
		Exhausted:       true,
		CompleteThrough: accessWatermark(all),
	}
	if query.Limit > 0 && len(matched) > query.Limit {
		start := len(matched) - query.Limit
		page.Entries = append([]AccessEvent{}, matched[start:]...)
		page.Continuation = encodeAccessCursor(page.Entries[0])
		page.Exhausted = start == 0
		return page, nil
	}
	page.Entries = matched
	return page, nil
}

func (s *FileStore) matchingAccess(query AccessQuery) ([]AccessEvent, error) {
	all, err := readJSONL[AccessEvent](s.AccessPath)
	if err != nil {
		return nil, err
	}
	return matchingAccess(all, query)
}

func matchingAccess(all []AccessEvent, query AccessQuery) ([]AccessEvent, error) {
	if err := validateAccessRange(query.Since, query.Until); err != nil {
		return nil, err
	}
	out := make([]AccessEvent, 0, len(all))
	for _, event := range all {
		ok, err := accessMatches(event, query)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, event)
		}
	}
	return out, nil
}

func accessMatches(event AccessEvent, query AccessQuery) (bool, error) {
	if query.EvidenceID != "" && event.EvidenceID != query.EvidenceID {
		return false, nil
	}
	if query.Principal != "" && event.Identity.Principal != query.Principal {
		return false, nil
	}
	if query.OnBehalfOf != "" && event.Identity.OnBehalfOf != query.OnBehalfOf {
		return false, nil
	}
	if query.Action != "" && event.Action != query.Action {
		return false, nil
	}
	if query.TraceID != "" && event.Trace.TraceID != query.TraceID {
		return false, nil
	}
	inRange, err := occurredInRange(event.OccurredAt, query.Since, query.Until)
	if err != nil {
		return false, err
	}
	if !inRange {
		return false, nil
	}
	if (query.Repository != "" || query.Object != "") && !matchesKnowledge(event, query) {
		return false, nil
	}
	return true, nil
}

func matchesKnowledge(event AccessEvent, query AccessQuery) bool {
	for _, target := range event.Knowledge {
		ref := target.KnowledgeRef
		if query.Repository != "" && ref.Repository != query.Repository {
			continue
		}
		if query.Object != "" && ref.Object != query.Object {
			continue
		}
		return true
	}
	return false
}

func validateAccessRange(since, until string) error {
	if since != "" {
		if _, err := parseOccurredAt(since); err != nil {
			return kernel.Fail(kernel.ErrUsageInvalid, "access since is invalid")
		}
	}
	if until != "" {
		if _, err := parseOccurredAt(until); err != nil {
			return kernel.Fail(kernel.ErrUsageInvalid, "access until is invalid")
		}
	}
	if since != "" && until != "" {
		start, _ := parseOccurredAt(since)
		end, _ := parseOccurredAt(until)
		if end.Before(start) {
			return kernel.Fail(kernel.ErrUsageInvalid, "access until must not precede since")
		}
	}
	return nil
}

func occurredInRange(value, since, until string) (bool, error) {
	if since == "" && until == "" {
		return true, nil
	}
	occurred, err := parseOccurredAt(value)
	if err != nil {
		return false, kernel.Fail(kernel.ErrPreconditionFailed, "access evidence has an invalid occurredAt")
	}
	if since != "" {
		start, err := parseOccurredAt(since)
		if err != nil {
			return false, kernel.Fail(kernel.ErrUsageInvalid, "access since is invalid")
		}
		if occurred.Before(start) {
			return false, nil
		}
	}
	if until != "" {
		end, err := parseOccurredAt(until)
		if err != nil {
			return false, kernel.Fail(kernel.ErrUsageInvalid, "access until is invalid")
		}
		if occurred.After(end) {
			return false, nil
		}
	}
	return true, nil
}

func parseOccurredAt(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	return time.Parse(time.RFC3339, value)
}

func sortAccessEvents(events []AccessEvent) {
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].OccurredAt != events[j].OccurredAt {
			return occurredBefore(events[i].OccurredAt, events[j].OccurredAt)
		}
		return events[i].EvidenceID < events[j].EvidenceID
	})
}

func accessWatermark(events []AccessEvent) string {
	watermark := ""
	for _, event := range events {
		if watermark == "" || occurredBefore(watermark, event.OccurredAt) {
			watermark = event.OccurredAt
		}
	}
	return watermark
}

func accessBeforeCursor(event AccessEvent, cursor accessCursor) bool {
	if occurredBefore(event.OccurredAt, cursor.OccurredAt) {
		return true
	}
	if occurredBefore(cursor.OccurredAt, event.OccurredAt) {
		return false
	}
	return event.EvidenceID < cursor.EvidenceID
}

func encodeAccessCursor(event AccessEvent) string {
	raw, err := json.Marshal(accessCursor{OccurredAt: event.OccurredAt, EvidenceID: event.EvidenceID})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeAccessCursor(value string) (*accessCursor, error) {
	if value == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "access continuation is invalid")
	}
	var cursor accessCursor
	if err := json.Unmarshal(raw, &cursor); err != nil || cursor.OccurredAt == "" || cursor.EvidenceID == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "access continuation is invalid")
	}
	if _, err := parseOccurredAt(cursor.OccurredAt); err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "access continuation is invalid")
	}
	return &cursor, nil
}
