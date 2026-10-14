package testkit

import (
	"bytes"
	"encoding/json"
)

// ValidateJSONFixture checks a test payload against its caller-owned wire type
// before running the caller's semantic validation. It owns no protocol fields.
func ValidateJSONFixture(payload, target any, validate func() error) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if validate != nil {
		return validate()
	}
	return nil
}
