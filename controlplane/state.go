package controlplane

import (
	"os"

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

type ControlBundle struct {
	Catalogs    map[string]ControlState     `json:"catalogs"`
	Proposals   map[string]Proposal         `json:"proposals,omitempty"`
	Previews    map[string]Preview          `json:"previews,omitempty"`
	Validations map[string]ValidationReport `json:"validations,omitempty"`
}

type FileControlState struct {
	file string
}

func NewFileControlState(file string) *FileControlState {
	return &FileControlState{file: file}
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

func (s *FileControlState) LoadBundle() (map[string]ControlState, error) {
	if _, err := os.Stat(s.file); os.IsNotExist(err) {
		return map[string]ControlState{}, nil
	}
	var raw ControlBundle
	if err := jsonfile.Read(s.file, &raw); err != nil {
		return nil, err
	}
	out := map[string]ControlState{}
	for id, st := range raw.Catalogs {
		out[id] = normalizeState(st)
	}
	if len(out) == 0 && (len(raw.Proposals) > 0 || len(raw.Previews) > 0 || len(raw.Validations) > 0) {
		out[""] = normalizeState(ControlState{Proposals: raw.Proposals, Previews: raw.Previews, Validations: raw.Validations})
	}
	return out, nil
}

func (s *FileControlState) Load() (ControlState, error) {
	all, err := s.LoadBundle()
	if err != nil {
		return ControlState{}, err
	}
	if st, ok := all[""]; ok {
		return st, nil
	}
	return EmptyControlState, nil
}

func (s *FileControlState) Save(state ControlState) error {
	return s.SaveBundle(map[string]ControlState{"": normalizeState(state)})
}

func (s *FileControlState) SaveBundle(states map[string]ControlState) error {
	copied := map[string]ControlState{}
	for id, st := range states {
		copied[id] = normalizeState(st)
	}
	return jsonfile.Write(s.file, ControlBundle{Catalogs: copied})
}
