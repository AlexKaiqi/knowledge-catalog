package object

import (
	"bytes"
	"fmt"
)

// ChangeState keeps legacy unknown distinct from a known true/false result.
// Its JSON form is Boolean; the zero value is omitted from CommitResult. As a
// value it preserves comparable, immutable receipt semantics in Go.
type ChangeState string

const (
	ChangeUnknown   ChangeState = ""
	ChangeUnchanged ChangeState = "UNCHANGED"
	ChangeChanged   ChangeState = "CHANGED"
)

func (s ChangeState) MarshalJSON() ([]byte, error) {
	switch s {
	case ChangeUnknown:
		return []byte("null"), nil
	case ChangeUnchanged:
		return []byte("false"), nil
	case ChangeChanged:
		return []byte("true"), nil
	default:
		return nil, fmt.Errorf("invalid change state %q", s)
	}
}
func (s *ChangeState) UnmarshalJSON(raw []byte) error {
	switch string(bytes.TrimSpace(raw)) {
	case "null":
		*s = ChangeUnknown
	case "false":
		*s = ChangeUnchanged
	case "true":
		*s = ChangeChanged
	default:
		return fmt.Errorf("changed must be a boolean or null")
	}
	return nil
}
