package hook

import (
	"fmt"
	"os"
	"path/filepath"

	"kc/internal/jsonfile"
	"kc/kernel"
)

const (
	PhasePre  = "pre"
	PhasePost = "post"
)

var mutatingOn = map[string]bool{
	"put": true, "remove": true, "commit": true, "append": true, "propose": true,
	"preview": true, "validate": true, "record-validation": true, "merge": true,
	"define-view": true, "retire-view": true, "register": true,
	"archive-catalog": true, "archive-repo": true,
}

type Binding struct {
	ID      string `json:"id"`
	On      string `json:"on"`
	Phase   string `json:"phase"`
	Repo    string `json:"repo,omitempty"`
	Catalog string `json:"catalog,omitempty"`
	Run     string `json:"run,omitempty"`
	URL     string `json:"url,omitempty"`
}

type File struct {
	Bindings []Binding `json:"bindings"`
}

func Path(home string) string { return filepath.Join(home, "hooks.json") }

func Read(home string) (File, error) {
	file := Path(home)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return File{Bindings: []Binding{}}, nil
	}
	var raw File
	if err := jsonfile.Read(file, &raw); err != nil {
		return File{}, err
	}
	if raw.Bindings == nil {
		raw.Bindings = []Binding{}
	}
	return raw, nil
}

func Write(home string, file File) error {
	if file.Bindings == nil {
		file.Bindings = []Binding{}
	}
	return jsonfile.Write(Path(home), file)
}

func NextID(bindings []Binding) string {
	return fmt.Sprintf("hk_%d", len(bindings)+1)
}

func CanHook(on string) bool { return mutatingOn[on] }

func ValidateOn(on string) error {
	if on == "" {
		return kernel.Fail(kernel.ErrPreconditionFailed, "hook requires --on")
	}
	if !CanHook(on) {
		return kernel.Fail(kernel.ErrPreconditionFailed, "hook --on %s is not a mutating verb", on)
	}
	return nil
}

func ValidateBinding(b Binding) error {
	if err := ValidateOn(b.On); err != nil {
		return err
	}
	if b.Phase != PhasePre && b.Phase != PhasePost {
		return kernel.Fail(kernel.ErrPreconditionFailed, "--phase must be pre or post")
	}
	if b.Run == "" && b.URL == "" {
		return kernel.Fail(kernel.ErrPreconditionFailed, "hook requires --run or --url")
	}
	if b.Run != "" && b.URL != "" {
		return kernel.Fail(kernel.ErrPreconditionFailed, "use only one of --run or --url")
	}
	if b.Phase == PhasePre && b.URL != "" {
		return kernel.Fail(kernel.ErrPreconditionFailed, "pre hook must use --run (sync, fail closed)")
	}
	return nil
}

func (f File) Match(on, phase, repo, catalog string) []Binding {
	out := []Binding{}
	for _, b := range f.Bindings {
		if b.On != on || b.Phase != phase {
			continue
		}
		if b.Repo != "" && b.Repo != repo {
			continue
		}
		if b.Catalog != "" && b.Catalog != catalog {
			continue
		}
		out = append(out, b)
	}
	return out
}
