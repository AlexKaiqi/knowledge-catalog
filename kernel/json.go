package kernel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"strings"
)

// UnmarshalJSON decodes one JSON value without rounding interface-valued
// numbers. It retains the usual float64 representation only when marshaling
// that float reproduces the same exact JSON number; otherwise it uses Number.
func UnmarshalJSON(data []byte, target any) error {
	// Match json.Unmarshal's complete-input validation before mutating target.
	if !json.Valid(data) {
		return json.Unmarshal(data, new(any))
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := DecodeJSON(decoder, target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("JSON input must contain exactly one value")
	}
	return nil
}

// DecodeJSON preserves exact numbers while retaining the caller's decoder
// configuration. Stream limits, unknown-field policy and trailing-value checks
// remain the caller's responsibility, as with json.Decoder.Decode.
func DecodeJSON(decoder *json.Decoder, target any) error {
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	normalizeDecodedJSON(reflect.ValueOf(target))
	return nil
}

func normalizeDecodedJSON(value reflect.Value) {
	if !value.IsValid() {
		return
	}
	switch value.Kind() {
	case reflect.Pointer:
		if !value.IsNil() {
			normalizeDecodedJSON(value.Elem())
		}
	case reflect.Interface:
		if value.IsNil() {
			return
		}
		if number, ok := value.Interface().(json.Number); ok && value.CanSet() {
			if floating, err := number.Float64(); err == nil {
				encoded, err := json.Marshal(floating)
				original, valid := canonicalJSONNumber(number.String())
				if normalized, ok := canonicalJSONNumber(string(encoded)); err == nil && ok && valid && original == normalized {
					value.Set(reflect.ValueOf(floating))
				}
			}
			return
		}
		normalizeDecodedJSON(value.Elem())
	case reflect.Map:
		iter := value.MapRange()
		for iter.Next() {
			item := reflect.New(value.Type().Elem()).Elem()
			item.Set(iter.Value())
			normalizeDecodedJSON(item)
			value.SetMapIndex(iter.Key(), item)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			normalizeDecodedJSON(value.Index(i))
		}
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if field := value.Field(i); field.CanSet() {
				normalizeDecodedJSON(field)
			}
		}
	}
}

// canonicalJSONNumber normalizes a decimal coefficient and exponent without
// expanding arbitrary powers of ten. Fixed notation uses encoding/json's
// ordinary magnitude range, preserving existing digests for ordinary values.
func canonicalJSONNumber(raw string) (string, bool) {
	if raw == "" || (raw[0] != '-' && (raw[0] < '0' || raw[0] > '9')) || !json.Valid([]byte(raw)) {
		return "", false
	}
	sign := ""
	if raw[0] == '-' {
		sign, raw = "-", raw[1:]
	}
	mantissa := raw
	exponent := new(big.Int)
	if at := strings.IndexAny(raw, "eE"); at >= 0 {
		mantissa = raw[:at]
		if _, ok := exponent.SetString(raw[at+1:], 10); !ok {
			return "", false
		}
	}
	fractionDigits := 0
	if at := strings.IndexByte(mantissa, '.'); at >= 0 {
		fractionDigits = len(mantissa) - at - 1
		mantissa = mantissa[:at] + mantissa[at+1:]
	}
	digits := strings.TrimLeft(mantissa, "0")
	if digits == "" {
		return "0", true
	}
	trimmed := strings.TrimRight(digits, "0")
	exponent.Add(exponent, big.NewInt(int64(len(digits)-len(trimmed)-fractionDigits)))
	digits = trimmed
	scientific := new(big.Int).Add(exponent, big.NewInt(int64(len(digits)-1)))
	if scientific.IsInt64() && scientific.Int64() >= -6 && scientific.Int64() < 21 {
		point := len(digits) + int(exponent.Int64())
		switch {
		case point <= 0:
			return sign + "0." + strings.Repeat("0", -point) + digits, true
		case point >= len(digits):
			return sign + digits + strings.Repeat("0", point-len(digits)), true
		default:
			return sign + digits[:point] + "." + digits[point:], true
		}
	}
	coefficient := digits[:1]
	if len(digits) > 1 {
		coefficient += "." + digits[1:]
	}
	exponentText := scientific.String()
	if scientific.Sign() >= 0 {
		exponentText = "+" + exponentText
	}
	return sign + coefficient + "e" + exponentText, true
}
