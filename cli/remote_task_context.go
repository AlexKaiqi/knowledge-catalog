package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"kc/kernel"
)

type mountedTaskContext struct {
	Version   int             `json:"version"`
	Principal string          `json:"principal"`
	Catalog   string          `json:"catalog,omitempty"`
	Workspace string          `json:"workspace"`
	Pin       json.RawMessage `json:"pin"`
	Root      string          `json:"root"`
	ReadOnly  bool            `json:"readOnly"`
}

func inheritTaskContext(publicPath string, flags map[string]FlagValue) error {
	home, err := resolveHome(flags)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if resolved, evalErr := filepath.EvalSymlinks(cwd); evalErr == nil {
		cwd = resolved
	}
	contexts, err := os.ReadDir(filepath.Join(home, "tasks"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var selected *mountedTaskContext
	for _, entry := range contexts {
		if !entry.IsDir() {
			continue
		}
		raw, readErr := os.ReadFile(filepath.Join(home, "tasks", entry.Name(), "context.json"))
		if readErr != nil {
			continue
		}
		var candidate mountedTaskContext
		if json.Unmarshal(raw, &candidate) != nil || candidate.Version != 1 || !candidate.ReadOnly || candidate.Root == "" {
			continue
		}
		if resolvedRoot, evalErr := filepath.EvalSymlinks(candidate.Root); evalErr == nil {
			candidate.Root = resolvedRoot
		}
		if !pathContains(candidate.Root, cwd) {
			continue
		}
		if selected == nil || len(candidate.Root) > len(selected.Root) {
			copy := candidate
			selected = &copy
		}
	}
	if selected == nil {
		return nil
	}
	if selected.Principal == "" || selected.Workspace == "" || len(selected.Pin) == 0 {
		return kernel.Fail(kernel.ErrPreconditionFailed, "active task mount context is incomplete")
	}
	for name, inherited := range map[string]string{"as": selected.Principal} {
		if inherited == "" {
			continue
		}
		if explicit := strings.TrimSpace(FlagString(flags, name)); explicit != "" && explicit != inherited {
			return kernel.Fail(kernel.ErrPreconditionFailed, "--%s conflicts with the active task mount", name)
		}
		flags[name] = inherited
	}
	// Schema discovery and maintainer --repo reads are pinned to one
	// Repository basis. Inheriting a mounted Workspace/pin would mix the
	// consumer knowledge-set path into those commands.
	if publicPath == "knowledge schema list" || FlagString(flags, "repo") != "" ||
		!strings.HasPrefix(publicPath, "knowledge ") {
		flags["_task-context"] = true
		return nil
	}
	for name, inherited := range map[string]string{"catalog": selected.Catalog, "workspace": selected.Workspace} {
		if inherited == "" {
			continue
		}
		if explicit := strings.TrimSpace(FlagString(flags, name)); explicit != "" && explicit != inherited {
			return kernel.Fail(kernel.ErrPreconditionFailed, "--%s conflicts with the active task mount", name)
		}
		flags[name] = inherited
	}
	if explicit := strings.TrimSpace(FlagString(flags, "pin")); explicit != "" {
		raw := explicit
		if !strings.HasPrefix(raw, "{") {
			if content, readErr := os.ReadFile(raw); readErr == nil {
				raw = string(content)
			}
		}
		if !sameJSON([]byte(raw), selected.Pin) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "--pin conflicts with the active task mount")
		}
	}
	flags["pin"] = string(selected.Pin)
	flags["_task-context"] = true
	return nil
}

func pathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)))
}

func sameJSON(left, right []byte) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && kernel.CanonicalDigest(a) == kernel.CanonicalDigest(b)
}
