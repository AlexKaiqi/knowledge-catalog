package catalog

import (
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"

	"kc/kernel"
)

// KnowledgeSetFileName is the recipe file that lives at a member Repository's root and
// travels with Git (docs/COMPOSITION.md §1.4, §2.2). It is not a required
// format for the Repository to be usable without this tool — it is how a Workspace
// declaration hitchhikes on an ordinary clone.
const KnowledgeSetFileName = ".kc-dataset.yaml"

// KnowledgeSetRecipe is the on-disk shape of a mount Workspace. Path is always
// declared (empty string = root). files lists per-file deliveries (U10):
// one repository file delivered to one target path. This file is only for
// mount/file recipes; federated-read workspaces (Path nil) stay Catalog-only.
type KnowledgeSetRecipe struct {
	Name   string                  `yaml:"name" json:"name"`
	Mounts []KnowledgeSetMount     `yaml:"mounts" json:"mounts"`
	Files  []KnowledgeSetFileMount `yaml:"files,omitempty" json:"files,omitempty"`
}

// KnowledgeSetMount is one mount line in KnowledgeSetRecipe.
type KnowledgeSetMount struct {
	Repository string `yaml:"repository" json:"repository"`
	Selector   string `yaml:"selector" json:"selector"`
	Path       string `yaml:"path" json:"path"`
	SubPath    string `yaml:"subPath,omitempty" json:"subPath,omitempty"`
	BaseRev    string `yaml:"baseRev,omitempty" json:"baseRev,omitempty"`
}

// KnowledgeSetFileMount is one per-file line in KnowledgeSetRecipe: the file
// lives at File inside the member repository and is delivered at Target in
// the composed tree. Every entry of one repository shares one selector and
// one frozen commit, like its mounts.
type KnowledgeSetFileMount struct {
	Repository string `yaml:"repository" json:"repository"`
	Selector   string `yaml:"selector" json:"selector"`
	File       string `yaml:"file" json:"file"`
	Target     string `yaml:"target" json:"target"`
	BaseRev    string `yaml:"baseRev,omitempty" json:"baseRev,omitempty"`
}

// ParseKnowledgeSetRecipe reads .kc-dataset.yaml bytes.
func ParseKnowledgeSetRecipe(raw []byte) (KnowledgeSetRecipe, error) {
	var rec KnowledgeSetRecipe
	if err := yaml.Unmarshal(raw, &rec); err != nil {
		return KnowledgeSetRecipe{}, kernel.Fail(kernel.ErrUsageInvalid, "invalid %s: %v", KnowledgeSetFileName, err)
	}
	if strings.TrimSpace(rec.Name) == "" {
		return KnowledgeSetRecipe{}, kernel.Fail(kernel.ErrUsageInvalid, "%s is missing name", KnowledgeSetFileName)
	}
	if len(rec.Mounts) == 0 && len(rec.Files) == 0 {
		return KnowledgeSetRecipe{}, kernel.Fail(kernel.ErrUsageInvalid, "%s has no mounts or files", KnowledgeSetFileName)
	}
	for i, m := range rec.Mounts {
		if strings.TrimSpace(m.Repository) == "" || strings.TrimSpace(m.Selector) == "" {
			return KnowledgeSetRecipe{}, kernel.Fail(kernel.ErrUsageInvalid, "%s mount %d needs repository and selector", KnowledgeSetFileName, i)
		}
	}
	for i, f := range rec.Files {
		if strings.TrimSpace(f.Repository) == "" || strings.TrimSpace(f.Selector) == "" ||
			strings.TrimSpace(f.File) == "" || strings.TrimSpace(f.Target) == "" {
			return KnowledgeSetRecipe{}, kernel.Fail(kernel.ErrUsageInvalid,
				"%s file %d needs repository, selector, file and target", KnowledgeSetFileName, i)
		}
	}
	return rec, nil
}

// FormatKnowledgeSetRecipe writes the yaml alice (or bob) would commit.
func FormatKnowledgeSetRecipe(rec KnowledgeSetRecipe) ([]byte, error) {
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

// RecipeFromKnowledgeSet builds the portable file for a mount/file Workspace.
// Pure federated-read recipes (no Path and no File) return ok=false: they have
// nothing to hitchhike on git.
func RecipeFromKnowledgeSet(def KnowledgeSet) (KnowledgeSetRecipe, bool) {
	if len(def.Sources) == 0 {
		return KnowledgeSetRecipe{}, false
	}
	rec := KnowledgeSetRecipe{Name: def.SetID}
	for _, src := range def.Sources {
		if src.IsFileEntry() {
			rec.Files = append(rec.Files, KnowledgeSetFileMount{
				Repository: string(src.Repository),
				Selector:   src.Selector,
				File:       src.File,
				Target:     src.Target,
				BaseRev:    src.BaseRev,
			})
			continue
		}
		if src.Path == nil {
			return KnowledgeSetRecipe{}, false
		}
		rec.Mounts = append(rec.Mounts, KnowledgeSetMount{
			Repository: string(src.Repository),
			Selector:   src.Selector,
			Path:       *src.Path,
			SubPath:    src.SubPath,
			BaseRev:    src.BaseRev,
		})
	}
	return rec, len(rec.Mounts) > 0 || len(rec.Files) > 0
}

// Sources is the KnowledgeSet.Sources equivalent of this recipe.
func (r KnowledgeSetRecipe) Sources() []KnowledgeSetSource {
	out := make([]KnowledgeSetSource, 0, len(r.Mounts)+len(r.Files))
	for _, m := range r.Mounts {
		out = append(out, KnowledgeSetSource{
			Repository: kernel.RepositoryID(m.Repository),
			Selector:   m.Selector,
			Path:       MountPath(m.Path),
			SubPath:    m.SubPath,
			BaseRev:    m.BaseRev,
		})
	}
	for _, f := range r.Files {
		out = append(out, KnowledgeSetSource{
			Repository: kernel.RepositoryID(f.Repository),
			Selector:   f.Selector,
			File:       f.File,
			Target:     f.Target,
			BaseRev:    f.BaseRev,
		})
	}
	return out
}

// RootMount is the source declared at path "" (new-file fallback), or nil.
func RootMount(sources []KnowledgeSetSource) *KnowledgeSetSource {
	for i := range sources {
		src := &sources[i]
		if src.Path != nil && normalizeMountPath(*src.Path) == "" {
			return src
		}
	}
	return nil
}

// HasMountPaths reports a Workspace recipe (every source declared Path).
func HasMountPaths(sources []KnowledgeSetSource) bool {
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
