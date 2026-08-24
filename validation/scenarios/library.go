// Package scenarios loads and validates the data-warehouse scenario library.
// User journeys compose executable actions; suites only group user journeys.
package scenarios

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed library.yaml
var embeddedLibrary []byte

type Fixture struct {
	ID          string `yaml:"id" json:"id"`
	Description string `yaml:"description" json:"description"`
}

type Library struct {
	Version   int        `yaml:"version" json:"version"`
	Fixture   Fixture    `yaml:"fixture" json:"fixture"`
	Scenarios []Scenario `yaml:"scenarios" json:"scenarios"`
}

type Scenario struct {
	ID           string            `yaml:"id" json:"id"`
	Aliases      []string          `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Title        string            `yaml:"title" json:"title"`
	Kind         string            `yaml:"kind" json:"kind"`
	Roles        []string          `yaml:"roles" json:"roles"`
	Goal         string            `yaml:"goal,omitempty" json:"goal,omitempty"`
	Outcome      string            `yaml:"outcome,omitempty" json:"outcome,omitempty"`
	Capabilities []string          `yaml:"capabilities" json:"capabilities"`
	Needs        []string          `yaml:"needs,omitempty" json:"needs,omitempty"`
	Steps        []string          `yaml:"steps,omitempty" json:"steps,omitempty"`
	Requires     []string          `yaml:"requires,omitempty" json:"requires,omitempty"`
	Env          map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Command      []string          `yaml:"command,omitempty" json:"command,omitempty"`
	Assertions   []string          `yaml:"assertions" json:"assertions"`
}

func Load() (*Library, error) {
	var library Library
	if err := yaml.Unmarshal(embeddedLibrary, &library); err != nil {
		return nil, fmt.Errorf("decode scenario library: %w", err)
	}
	if err := library.Validate(); err != nil {
		return nil, err
	}
	return &library, nil
}

func (l *Library) Validate() error {
	if l.Version != 1 {
		return fmt.Errorf("scenario library version must be 1, got %d", l.Version)
	}
	if strings.TrimSpace(l.Fixture.ID) == "" {
		return fmt.Errorf("scenario library fixture id is required")
	}
	byID := make(map[string]Scenario, len(l.Scenarios))
	names := make(map[string]string)
	for _, scenario := range l.Scenarios {
		if strings.TrimSpace(scenario.ID) == "" {
			return fmt.Errorf("scenario id is required")
		}
		if _, exists := byID[scenario.ID]; exists {
			return fmt.Errorf("duplicate scenario id %q", scenario.ID)
		}
		if strings.TrimSpace(scenario.Title) == "" || len(scenario.Roles) == 0 || len(scenario.Assertions) == 0 {
			return fmt.Errorf("scenario %q requires title, roles and assertions", scenario.ID)
		}
		switch scenario.Kind {
		case "action":
			if len(scenario.Command) == 0 || len(scenario.Steps) != 0 {
				return fmt.Errorf("action %q requires command and forbids steps", scenario.ID)
			}
		case "journey", "suite":
			if len(scenario.Steps) == 0 || len(scenario.Command) != 0 || len(scenario.Needs) != 0 {
				return fmt.Errorf("%s %q requires steps and forbids command/needs", scenario.Kind, scenario.ID)
			}
			if strings.TrimSpace(scenario.Goal) == "" || strings.TrimSpace(scenario.Outcome) == "" {
				return fmt.Errorf("%s %q requires a user goal and outcome", scenario.Kind, scenario.ID)
			}
		default:
			return fmt.Errorf("scenario %q has unknown kind %q", scenario.ID, scenario.Kind)
		}
		byID[scenario.ID] = scenario
		for _, name := range append([]string{scenario.ID}, scenario.Aliases...) {
			key := strings.ToLower(name)
			if owner, exists := names[key]; exists {
				return fmt.Errorf("scenario name %q is shared by %q and %q", name, owner, scenario.ID)
			}
			names[key] = scenario.ID
		}
	}
	for _, scenario := range l.Scenarios {
		for _, child := range append(append([]string{}, scenario.Needs...), scenario.Steps...) {
			if _, exists := byID[child]; !exists {
				return fmt.Errorf("scenario %q references unknown scenario %q", scenario.ID, child)
			}
		}
	}
	for _, scenario := range l.Scenarios {
		if _, err := l.Plan(scenario.ID); err != nil {
			return err
		}
	}
	return nil
}

func (l *Library) Resolve(name string) (Scenario, bool) {
	for _, scenario := range l.Scenarios {
		if strings.EqualFold(scenario.ID, name) {
			return scenario, true
		}
		for _, alias := range scenario.Aliases {
			if strings.EqualFold(alias, name) {
				return scenario, true
			}
		}
	}
	return Scenario{}, false
}

// Plan expands action dependencies, user journeys and suites into a stable,
// de-duplicated action list. A cycle reports the node that closes it.
func (l *Library) Plan(name string) ([]Scenario, error) {
	target, ok := l.Resolve(name)
	if !ok {
		return nil, fmt.Errorf("unknown scenario %q", name)
	}
	byID := make(map[string]Scenario, len(l.Scenarios))
	for _, scenario := range l.Scenarios {
		byID[scenario.ID] = scenario
	}
	done := map[string]bool{}
	active := map[string]bool{}
	var plan []Scenario
	var visit func(string) error
	visit = func(id string) error {
		if done[id] {
			return nil
		}
		if active[id] {
			return fmt.Errorf("scenario dependency cycle closes at %q", id)
		}
		active[id] = true
		scenario := byID[id]
		children := scenario.Needs
		if scenario.Kind == "journey" || scenario.Kind == "suite" {
			children = scenario.Steps
		}
		for _, child := range children {
			if _, exists := byID[child]; !exists {
				return fmt.Errorf("scenario %q references unknown scenario %q", id, child)
			}
			if err := visit(child); err != nil {
				return err
			}
		}
		active[id] = false
		done[id] = true
		if scenario.Kind == "action" {
			plan = append(plan, scenario)
		}
		return nil
	}
	if err := visit(target.ID); err != nil {
		return nil, err
	}
	return plan, nil
}

func (l *Library) Sorted() []Scenario {
	out := append([]Scenario(nil), l.Scenarios...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
