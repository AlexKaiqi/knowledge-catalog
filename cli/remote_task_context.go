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
	Server    string          `json:"server,omitempty"`
	AuthMode  string          `json:"authMode,omitempty"`
	Principal string          `json:"principal"`
	Catalog   string          `json:"catalog,omitempty"`
	Dataset   string          `json:"dataset"`
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
	var selected *mountedTaskContext
	// A project selection at the same root overrides its unbound host task;
	// a nested task remains the most specific context, including when unbound.
	for _, group := range []string{"tasks", "projects"} {
		contexts, err := os.ReadDir(filepath.Join(home, group))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range contexts {
			if !entry.IsDir() {
				continue
			}
			raw, readErr := os.ReadFile(filepath.Join(home, group, entry.Name(), "context.json"))
			if readErr != nil {
				continue
			}
			var candidate mountedTaskContext
			if json.Unmarshal(raw, &candidate) != nil || candidate.Version != 1 || !candidate.ReadOnly || !filepath.IsAbs(candidate.Root) {
				continue
			}
			if resolvedRoot, evalErr := filepath.EvalSymlinks(candidate.Root); evalErr == nil {
				candidate.Root = resolvedRoot
			}
			if !pathContains(candidate.Root, cwd) {
				continue
			}
			if selected == nil || len(candidate.Root) > len(selected.Root) || (group == "projects" && candidate.Root == selected.Root) {
				copy := candidate
				selected = &copy
			}
		}
	}
	if selected == nil {
		return nil
	}
	// The most specific unbound task also shadows a parent mount context.
	// It supplies no knowledge coordinates and must not inherit stale ones.
	if selected.Dataset == "" && (len(selected.Pin) == 0 || string(selected.Pin) == "null") {
		return nil
	}
	if selected.Principal == "" && selected.AuthMode != "token" && selected.AuthMode != "session" {
		return kernel.Fail(kernel.ErrPreconditionFailed, "active task mount context is incomplete")
	}
	// The endpoint is part of the private context, never of the portable pin.
	if selected.Server != "" {
		if explicit := strings.TrimRight(remoteServerURL(flags), "/"); explicit != "" && explicit != strings.TrimRight(selected.Server, "/") {
			return kernel.Fail(kernel.ErrPreconditionFailed, "--server conflicts with the active task context")
		}
		flags["server"] = selected.Server
	}
	principal := selected.Principal
	if selected.AuthMode == "token" || selected.AuthMode == "session" {
		principal = ""
	}
	for name, inherited := range map[string]string{"as": principal} {
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
	if catalogSearchRequested(publicPath, flags) || publicPath == "schema list" || FlagString(flags, "repo") != "" ||
		!knowledgeCLIPath(publicPath) {
		flags["_task-context"] = true
		return nil
	}
	if selected.Dataset == "" && FlagString(flags, "dataset") != "" {
		return kernel.Fail(kernel.ErrPreconditionFailed, "--dataset conflicts with the active temporary task context")
	}
	for name, inherited := range map[string]string{"catalog": selected.Catalog, "dataset": selected.Dataset} {
		if inherited == "" {
			continue
		}
		if explicit := strings.TrimSpace(FlagString(flags, name)); explicit != "" && explicit != inherited {
			return kernel.Fail(kernel.ErrPreconditionFailed, "--%s conflicts with the active task mount", name)
		}
		flags[name] = inherited
	}
	if len(selected.Pin) > 0 && string(selected.Pin) != "null" {
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
	}
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
