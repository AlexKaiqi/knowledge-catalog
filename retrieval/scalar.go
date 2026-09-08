package retrieval

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"kc/kernel"
)

// NormalizeScalarLiteral parses the string wire value according to the
// AccessField type. Providers compare this canonical form, never an accidental
// lexical representation of a number or timestamp.
func NormalizeScalarLiteral(fieldType, raw string) (string, error) {
	t := strings.ToLower(strings.TrimSpace(fieldType))
	switch t {
	case "", "string", "object_ref", "object_ref_list":
		return raw, nil
	case "bool", "boolean":
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return "", invalidScalar(fieldType, raw)
		}
		return strconv.FormatBool(v), nil
	case "int", "integer", "long":
		v, err := normalizeInteger(raw)
		if err != nil {
			return "", invalidScalar(fieldType, raw)
		}
		return v, nil
	case "number", "float", "double":
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return "", invalidScalar(fieldType, raw)
		}
		return strconv.FormatFloat(v, 'g', -1, 64), nil
	case "date":
		v, err := ParseScalarTime(t, raw)
		if err != nil {
			return "", invalidScalar(fieldType, raw)
		}
		return v.Format("2006-01-02"), nil
	case "datetime", "timestamp":
		v, err := ParseScalarTime(t, raw)
		if err != nil {
			return "", invalidScalar(fieldType, raw)
		}
		return v.UTC().Format(time.RFC3339Nano), nil
	default:
		return "", kernel.Fail(kernel.ErrUsageInvalid, "unsupported scalar type %q", fieldType)
	}
}

func NormalizeScalarValue(fieldType string, value any) (string, bool) {
	if value == nil {
		return "", false
	}
	var raw string
	switch v := value.(type) {
	case string:
		raw = v
	case json.Number:
		raw = v.String()
	case float64:
		if integerType(fieldType) {
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v {
				return "", false
			}
			raw = strconv.FormatFloat(float64(v), 'f', 0, 64)
			break
		}
		raw = strconv.FormatFloat(v, 'g', -1, 64)
	case float32:
		if integerType(fieldType) {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || math.Trunc(float64(v)) != float64(v) {
				return "", false
			}
			raw = strconv.FormatFloat(float64(v), 'f', 0, 64)
			break
		}
		raw = strconv.FormatFloat(float64(v), 'g', -1, 64)
	case int:
		raw = strconv.Itoa(v)
	case int64:
		raw = strconv.FormatInt(v, 10)
	case int32:
		raw = strconv.FormatFloat(float64(v), 'f', 0, 64)
	case bool:
		raw = strconv.FormatBool(v)
	default:
		return "", false
	}
	normalized, err := NormalizeScalarLiteral(fieldType, raw)
	return normalized, err == nil
}

// CompareScalarValues compares values using the declared logical type. Integer
// values retain all 64 bits; timestamps compare instants, including nanoseconds.
// Integer inputs are validated against int64 without converting through float64.
func CompareScalarValues(fieldType string, left, right any) (int, error) {
	l, ok := NormalizeScalarValue(fieldType, left)
	if !ok {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "invalid %s comparison value", fieldType)
	}
	r, ok := NormalizeScalarValue(fieldType, right)
	if !ok {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "invalid %s comparison value", fieldType)
	}
	switch strings.ToLower(strings.TrimSpace(fieldType)) {
	case "int", "integer", "long":
		lv, _ := strconv.ParseInt(l, 10, 64)
		rv, _ := strconv.ParseInt(r, 10, 64)
		if lv < rv {
			return -1, nil
		}
		if lv > rv {
			return 1, nil
		}
		return 0, nil
	case "number", "float", "double":
		lv, _ := strconv.ParseFloat(l, 64)
		rv, _ := strconv.ParseFloat(r, 64)
		if lv < rv {
			return -1, nil
		}
		if lv > rv {
			return 1, nil
		}
		return 0, nil
	case "datetime", "timestamp":
		lv, err := ParseScalarTime(fieldType, l)
		if err != nil {
			return 0, err
		}
		rv, err := ParseScalarTime(fieldType, r)
		if err != nil {
			return 0, err
		}
		if lv.Before(rv) {
			return -1, nil
		}
		if lv.After(rv) {
			return 1, nil
		}
		return 0, nil
	default:
		return strings.Compare(l, r), nil
	}
}

func integerType(fieldType string) bool {
	switch strings.ToLower(strings.TrimSpace(fieldType)) {
	case "int", "integer", "long":
		return true
	}
	return false
}

func invalidScalar(fieldType, raw string) error {
	return kernel.Fail(kernel.ErrUsageInvalid, "value %q is not a valid %s scalar", raw, fieldType)
}

var integerLiteral = regexp.MustCompile(`^([+-]?)([0-9]+)(?:\.([0-9]+))?(?:[eE]([+-]?[0-9]+))?$`)
var extendedUTCYear = regexp.MustCompile(`^(-0001|10000)(-[0-9]{2}-[0-9]{2}T.*Z)$`)
var temporalFraction = regexp.MustCompile(`[.,]([0-9]+)(?:Z|[+-][0-9]{2}:[0-9]{2})$`)

// normalizeInteger accepts mathematically integral decimal/exponent literals
// without rounding or allocating a power proportional to an untrusted exponent.
func normalizeInteger(raw string) (string, error) {
	parts := integerLiteral.FindStringSubmatch(raw)
	if parts == nil {
		return "", invalidScalar("integer", raw)
	}
	digits := strings.TrimLeft(parts[2]+parts[3], "0")
	if digits == "" {
		return "0", nil
	}
	exponent := int64(0)
	if parts[4] != "" {
		var err error
		exponent, err = strconv.ParseInt(parts[4], 10, 64)
		if err != nil {
			return "", invalidScalar("integer", raw)
		}
	}
	if exponent > int64(len(parts[3]))+19 || exponent < -int64(len(digits)) {
		return "", invalidScalar("integer", raw)
	}
	shift := exponent - int64(len(parts[3]))
	trimmed := strings.TrimRight(digits, "0")
	shift += int64(len(digits) - len(trimmed))
	digits = trimmed
	if shift < 0 || int64(len(digits))+shift > 19 {
		return "", invalidScalar("integer", raw)
	}
	integer := parts[1] + digits + strings.Repeat("0", int(shift))
	value, err := strconv.ParseInt(integer, 10, 64)
	if err != nil {
		return "", invalidScalar("integer", raw)
	}
	return strconv.FormatInt(value, 10), nil
}

// ParseScalarTime shares the nanosecond temporal contract across providers and
// the executor. Besides RFC3339 inputs, it accepts the two extended UTC years
// produced when valid boundary-year inputs are normalized from an offset.
func ParseScalarTime(fieldType, raw string) (time.Time, error) {
	if strings.EqualFold(strings.TrimSpace(fieldType), "date") {
		value, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return time.Time{}, invalidScalar(fieldType, raw)
		}
		return value, nil
	}
	switch strings.ToLower(strings.TrimSpace(fieldType)) {
	case "datetime", "timestamp":
	default:
		return time.Time{}, invalidScalar(fieldType, raw)
	}
	if fraction := temporalFraction.FindStringSubmatch(raw); fraction != nil && len(fraction[1]) > 9 && strings.Trim(fraction[1][9:], "0") != "" {
		return time.Time{}, invalidScalar(fieldType, raw)
	}
	if value, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return value, nil
	}
	parts := extendedUTCYear.FindStringSubmatch(raw)
	if parts == nil {
		return time.Time{}, invalidScalar(fieldType, raw)
	}
	value, err := time.Parse(time.RFC3339Nano, "2000"+parts[2])
	if err != nil {
		return time.Time{}, invalidScalar(fieldType, raw)
	}
	year, _ := strconv.Atoi(parts[1])
	extended := time.Date(year, value.Month(), value.Day(), value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), time.UTC)
	if extended.Month() != value.Month() || extended.Day() != value.Day() {
		return time.Time{}, invalidScalar(fieldType, raw)
	}
	return extended, nil
}
