package gate

import (
	"fmt"
	"os"
	"path/filepath"

	"kc/internal/jsonfile"
	"kc/kernel"
)

type Rule struct {
	ID      string   `json:"id"`
	On      string   `json:"on"`
	Repo    string   `json:"repo,omitempty"`
	Catalog string   `json:"catalog,omitempty"`
	Require []string `json:"require"`
}

type File struct {
	Rules []Rule `json:"rules"`
}

func Path(home string) string { return filepath.Join(home, "gates.json") }

func Read(home string) (File, error) {
	file := Path(home)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return File{Rules: []Rule{}}, nil
	}
	var raw File
	if err := jsonfile.Read(file, &raw); err != nil {
		return File{}, err
	}
	if raw.Rules == nil {
		raw.Rules = []Rule{}
	}
	return raw, nil
}

func Write(home string, file File) error {
	if file.Rules == nil {
		file.Rules = []Rule{}
	}
	return jsonfile.Write(Path(home), file)
}

func NextID(rules []Rule) string {
	return fmt.Sprintf("gt_%d", len(rules)+1)
}

func (f File) Required(on, repo, catalog string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, rule := range f.Rules {
		if rule.On != on {
			continue
		}
		if on == OnMerge && rule.Repo != "" && rule.Repo != repo {
			continue
		}
		for _, name := range rule.Require {
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func ValidateOn(on string) error {
	if on != OnMerge {
		return kernel.Fail(kernel.ErrUsageInvalid, "--on must be merge")
	}
	return nil
}
