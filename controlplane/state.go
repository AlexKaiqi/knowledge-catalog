package controlplane

import (
	"encoding/json"
	"kc/kernel"
	"os"
	"strings"

	"kc/internal/jsonfile"
)

type ControlState struct {
	Proposals   map[string]Proposal         `json:"proposals"`
	Previews    map[string]Preview          `json:"previews"`
	Validations map[string]ValidationReport `json:"validations"`
}

var EmptyControlState = ControlState{
	Proposals:   map[string]Proposal{},
	Previews:    map[string]Preview{},
	Validations: map[string]ValidationReport{},
}

type FileControlState struct {
	file        string
	authorityID string
}

func NewFileControlState(file, authorityID string) *FileControlState {
	return &FileControlState{file: file, authorityID: authorityID}
}

func normalizeState(raw ControlState) ControlState {
	if raw.Proposals == nil {
		raw.Proposals = map[string]Proposal{}
	}
	if raw.Previews == nil {
		raw.Previews = map[string]Preview{}
	}
	if raw.Validations == nil {
		raw.Validations = map[string]ValidationReport{}
	}
	return raw
}

// Load recovers the unique service control state. A legacy single-space bundle
// is read without writes; multiple spaces cannot be merged implicitly.
func (s *FileControlState) Load() (ControlState, error) {
	if _, err := os.Stat(s.file); os.IsNotExist(err) {
		return normalizeState(ControlState{}), nil
	}
	var raw map[string]json.RawMessage
	if err := jsonfile.Read(s.file, &raw); err != nil {
		return ControlState{}, err
	}
	var legacy json.RawMessage
	for field, value := range raw {
		if strings.EqualFold(field, "catalogs") {
			if len(raw) != 1 {
				return ControlState{}, kernel.Fail(kernel.ErrPreconditionFailed, "mixed legacy and service control state requires an explicit migration")
			}
			legacy = value
		}
	}
	if legacy != nil {
		var states map[string]ControlState
		if err := json.Unmarshal(legacy, &states); err != nil {
			return ControlState{}, err
		}
		if len(states) > 1 {
			return ControlState{}, kernel.Fail(kernel.ErrPreconditionFailed, "multiple legacy registration spaces require an explicit migration")
		}
		for id, state := range states {
			if id != "" && id != s.authorityID {
				return ControlState{}, kernel.Fail(kernel.ErrPreconditionFailed, "legacy control authority identity does not match the configured registry")
			}
			return normalizeState(state), nil
		}
		return normalizeState(ControlState{}), nil
	}
	var state ControlState
	if err := jsonfile.Read(s.file, &state); err != nil {
		return ControlState{}, err
	}
	return normalizeState(state), nil
}

func (s *FileControlState) Save(state ControlState) error {
	return jsonfile.Write(s.file, normalizeState(state))
}
