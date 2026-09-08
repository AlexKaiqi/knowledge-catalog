package opensearch

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"kc/kernel"
	"kc/retrieval"
)

// A fixed-width biased second followed by nanoseconds preserves time order
// under keyword comparison, without date's millisecond loss or date_nanos'
// restricted year range. This representation belongs only to the adapter.
func encodeTemporalKey(value string) (string, error) {
	instant, err := parseTemporalValue(value)
	if err != nil {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "invalid OpenSearch temporal value %q: %v", value, err)
	}
	return fmt.Sprintf("%020d%09d", uint64(instant.Unix())^(uint64(1)<<63), instant.Nanosecond()), nil
}

func parseTemporalValue(value string) (time.Time, error) {
	fieldType := "timestamp"
	if len(value) == len("2006-01-02") {
		fieldType = "date"
	}
	return retrieval.ParseScalarTime(fieldType, value)
}

func decodeTemporalKey(value, fieldType string) (string, error) {
	if len(value) != 29 {
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "invalid OpenSearch temporal sort key")
	}
	seconds, err := strconv.ParseUint(value[:20], 10, 64)
	if err != nil {
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "invalid OpenSearch temporal seconds")
	}
	nanos, err := strconv.ParseUint(value[20:], 10, 32)
	if err != nil || nanos >= 1e9 {
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "invalid OpenSearch temporal nanoseconds")
	}
	instant := time.Unix(int64(seconds^(uint64(1)<<63)), int64(nanos)).UTC()
	if strings.EqualFold(strings.TrimSpace(fieldType), "date") {
		return instant.Format("2006-01-02"), nil
	}
	return instant.Format(time.RFC3339Nano), nil
}

func logicalSortValue(req retrieval.SearchRequest, spec retrieval.AccessSpec, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	clause, ok := retrieval.SearchSortClause(req)
	if !ok || clause.Field == nil {
		return value, nil
	}
	field, err := spec.ResolveField(*clause.Field)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(strings.TrimSpace(field.Type)) {
	case "date", "datetime", "timestamp":
		key, ok := value.(string)
		if !ok {
			return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "invalid OpenSearch temporal sort value")
		}
		return decodeTemporalKey(key, field.Type)
	default:
		return value, nil
	}
}
