package catalog

import (
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"

	"kc/kernel"
)

// WorkspaceFileName is the recipe file that lives at a member Repository's root and
// travels with Git (docs/COMPOSITION.md §1.4, §2.2). It is not a required
// format for the Repository to be usable without this tool — it is how a Workspace
// declaration hitchhikes on an ordinary clone.
const WorkspaceFileName = ".kc-workspace.yaml"

// WorkspaceRecipe is the on-disk shape of a mount Workspace. Path is always
// declared (empty string = root). This file is only for mount recipes;
// federated-read workspaces (Path nil) stay Catalog-only.
type WorkspaceRecipe struct {
	Name   string           `yaml:"name" json:"name"`
	Mounts []WorkspaceMount `yaml:"mounts" json:"mounts"`
}

// WorkspaceMount is one mount line in WorkspaceRecipe.
type WorkspaceMount struct {
	Repository string `yaml:"repository" json:"repository"`
	Selector   string `yaml:"selector" json:"selector"`
	Path       string `yaml:"path" json:"path"`
	SubPath    string `yaml:"subPath,omitempty" json:"subPath,omitempty"`
	BaseRev    string `yaml:"baseRev,omitempty" json:"baseRev,omitempty"`
}

// ParseWorkspaceRecipe reads .kc-workspace.yaml bytes.
func ParseWorkspaceRecipe(raw []byte) (WorkspaceRecipe, error) {
	var rec WorkspaceRecipe
	if err := yaml.Unmarshal(raw, &rec); err != nil {
		return WorkspaceRecipe{}, kernel.Fail(kernel.ErrUsageInvalid, "invalid %s: %v", WorkspaceFileName, err)
	}
	if strings.TrimSpace(rec.Name) == "" {
		return WorkspaceRecipe{}, kernel.Fail(kernel.ErrUsageInvalid, "%s is missing name", WorkspaceFileName)
	}
	if len(rec.Mounts) == 0 {
		return WorkspaceRecipe{}, kernel.Fail(kernel.ErrUsageInvalid, "%s has no mounts", WorkspaceFileName)
	}
	for i, m := range rec.Mounts {
		if strings.TrimSpace(m.Repository) == "" || strings.TrimSpace(m.Selector) == "" {
			return WorkspaceRecipe{}, kernel.Fail(kernel.ErrUsageInvalid, "%s mount %d needs repository and selector", WorkspaceFileName, i)
		}
	}
	return rec, nil
}

// FormatWorkspaceRecipe writes the yaml alice (or bob) would commit.
func FormatWorkspaceRecipe(rec WorkspaceRecipe) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(rec); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RecipeFromWorkspace builds the portable file for a mount Workspace. Federated-read
// recipes (no Path) return ok=false: they have nothing to hitchhike on git.
func RecipeFromWorkspace(def WorkspaceDefinition) (WorkspaceRecipe, bool) {
	if len(def.Sources) == 0 {
		return WorkspaceRecipe{}, false
	}
	mounts := make([]WorkspaceMount, 0, len(def.Sources))
	for _, src := range def.Sources {
		if src.Path == nil {
			return WorkspaceRecipe{}, false
		}
		mounts = append(mounts, WorkspaceMount{
			Repository: string(src.Repository),
			Selector:   src.Selector,
			Path:       *src.Path,
			SubPath:    src.SubPath,
			BaseRev:    src.BaseRev,
		})
	}
	return WorkspaceRecipe{Name: def.WorkspaceID, Mounts: mounts}, true
}

// Sources is the WorkspaceDefinition.Sources equivalent of this recipe.
func (r WorkspaceRecipe) Sources() []WorkspaceSource {
	out := make([]WorkspaceSource, 0, len(r.Mounts))
	for _, m := range r.Mounts {
		out = append(out, WorkspaceSource{
			Repository: kernel.RepositoryID(m.Repository),
			Selector:   m.Selector,
			Path:       MountPath(m.Path),
			SubPath:    m.SubPath,
			BaseRev:    m.BaseRev,
		})
	}
	return out
}

// RootMount is the source declared at path "" (new-file fallback), or nil.
func RootMount(sources []WorkspaceSource) *WorkspaceSource {
	for i := range sources {
		src := &sources[i]
		if src.Path != nil && normalizeMountPath(*src.Path) == "" {
			return src
		}
	}
	return nil
}

// HasMountPaths reports a Workspace recipe (every source declared Path).
func HasMountPaths(sources []WorkspaceSource) bool {
	if len(sources) == 0 {
		return false
	}
	for _, src := range sources {
		if src.Path == nil {
			return false
		}
	}
	return true
}
