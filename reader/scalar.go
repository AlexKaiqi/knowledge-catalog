package reader

import (
	"encoding/json"
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
	case "", "string":
		return raw, nil
	case "bool", "boolean":
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return "", invalidScalar(fieldType, raw)
		}
		return strconv.FormatBool(v), nil
	case "int", "integer", "long":
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return "", invalidScalar(fieldType, raw)
		}
		return strconv.FormatInt(v, 10), nil
	case "number", "float", "double":
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return "", invalidScalar(fieldType, raw)
		}
		return strconv.FormatFloat(v, 'g', -1, 64), nil
	case "date":
		v, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return "", invalidScalar(fieldType, raw)
		}
		return v.Format("2006-01-02"), nil
	case "datetime", "timestamp":
		v, err := time.Parse(time.RFC3339, raw)
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
		raw = strconv.FormatFloat(v, 'g', -1, 64)
	case float32:
		raw = strconv.FormatFloat(float64(v), 'g', -1, 64)
	case int:
		raw = strconv.Itoa(v)
	case int64:
		raw = strconv.FormatInt(v, 10)
	case int32:
		raw = strconv.FormatInt(int64(v), 10)
	case bool:
		raw = strconv.FormatBool(v)
	default:
		return "", false
	}
	normalized, err := NormalizeScalarLiteral(fieldType, raw)
	return normalized, err == nil
}

func invalidScalar(fieldType, raw string) error {
	return kernel.Fail(kernel.ErrUsageInvalid, "value %q is not a valid %s scalar", raw, fieldType)
}
