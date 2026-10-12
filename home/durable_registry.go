package home

import (
	"bytes"
	"encoding/json"
	"io"
	"kc/kernel"
	"strings"
)

// Durable formats follow encoding/json's case-insensitive matching. Refuse
// repeated aliases rather than choosing one identity from an ambiguous receipt.
func durableFields(body []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if token != json.Delim('{') {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "durable receipt must be an object")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "durable receipt field must have a name")
		}
		folded := strings.ToLower(name)
		if _, ok := fields[folded]; ok {
			return nil, kernel.Fail(kernel.ErrPreconditionFailed, "durable receipt repeats field %s", name)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[folded] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "durable receipt has trailing data")
	}
	return fields, nil
}
