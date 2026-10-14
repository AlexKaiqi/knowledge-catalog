package dataset

import (
	"bytes"
	"maps"
	"strings"

	"gopkg.in/yaml.v3"

	"kc/kernel"
)

// DatasetFileName is the recipe file that lives at a member Repository's root and
// travels with Git (doc/design/dataset.md). It is not a required
// format for the Repository to be usable without this tool — it is how a Workspace
// declaration hitchhikes on an ordinary clone.
const DatasetFileName = ".kc-dataset.yaml"

// DatasetRecipe is the on-disk shape of a mount Workspace. Path is always
// declared (empty string = root). files lists per-file deliveries (U10):
// one repository file delivered to one target path. This file is only for
// mount/file recipes; federated-read workspaces (Path nil) stay service-only.
type DatasetRecipe struct {
	Name     string             `yaml:"name" json:"name"`
	Mounts   []DatasetMount     `yaml:"mounts" json:"mounts"`
	Files    []DatasetFileMount `yaml:"files,omitempty" json:"files,omitempty"`
	Metadata map[string]string  `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// DatasetMount is one mount line in DatasetRecipe.
type DatasetMount struct {
	Repository string `yaml:"repository" json:"repository"`
	Selector   string `yaml:"selector" json:"selector"`
	Path       string `yaml:"path" json:"path"`
	SubPath    string `yaml:"subPath,omitempty" json:"subPath,omitempty"`
	BaseRev    string `yaml:"baseRev,omitempty" json:"baseRev,omitempty"`
}

// DatasetFileMount is one per-file line in DatasetRecipe: the file
// lives at File inside the member repository and is delivered at Target in
// the composed tree. Every entry of one repository shares one selector and
// one frozen commit, like its mounts.
type DatasetFileMount struct {
	Repository string `yaml:"repository" json:"repository"`
	Selector   string `yaml:"selector" json:"selector"`
	File       string `yaml:"file" json:"file"`
	Target     string `yaml:"target" json:"target"`
	BaseRev    string `yaml:"baseRev,omitempty" json:"baseRev,omitempty"`
}

// ParseDatasetRecipe reads .kc-dataset.yaml bytes.
func ParseDatasetRecipe(raw []byte) (DatasetRecipe, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return DatasetRecipe{}, kernel.Fail(kernel.ErrUsageInvalid, "invalid %s: %v", DatasetFileName, err)
	}
	if err := validateRecipeMetadata(&document); err != nil {
		return DatasetRecipe{}, err
	}
	var rec DatasetRecipe
	if err := document.Decode(&rec); err != nil {
		return DatasetRecipe{}, kernel.Fail(kernel.ErrUsageInvalid, "invalid %s: %v", DatasetFileName, err)
	}
	if strings.TrimSpace(rec.Name) == "" {
		return DatasetRecipe{}, kernel.Fail(kernel.ErrUsageInvalid, "%s is missing name", DatasetFileName)
	}
	if len(rec.Mounts) == 0 && len(rec.Files) == 0 {
		return DatasetRecipe{}, kernel.Fail(kernel.ErrUsageInvalid, "%s has no mounts or files", DatasetFileName)
	}
	for i, m := range rec.Mounts {
		if strings.TrimSpace(m.Repository) == "" || strings.TrimSpace(m.Selector) == "" {
			return DatasetRecipe{}, kernel.Fail(kernel.ErrUsageInvalid, "%s mount %d needs repository and selector", DatasetFileName, i)
		}
	}
	for i, f := range rec.Files {
		if strings.TrimSpace(f.Repository) == "" || strings.TrimSpace(f.Selector) == "" ||
			strings.TrimSpace(f.File) == "" || strings.TrimSpace(f.Target) == "" {
			return DatasetRecipe{}, kernel.Fail(kernel.ErrUsageInvalid,
				"%s file %d needs repository, selector, file and target", DatasetFileName, i)
		}
	}
	return rec, nil
}

func validateRecipeMetadata(document *yaml.Node) error {
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil // Existing recipe shape validation reports the structural error.
	}
	root := document.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value != "metadata" {
			continue
		}
		metadata := root.Content[i+1]
		if metadata.Kind != yaml.MappingNode {
			return kernel.Fail(kernel.ErrUsageInvalid, "%s metadata must contain string keys and values", DatasetFileName)
		}
		for _, entry := range metadata.Content {
			if entry.Kind != yaml.ScalarNode || entry.Tag != "!!str" {
				return kernel.Fail(kernel.ErrUsageInvalid, "%s metadata must contain string keys and values", DatasetFileName)
			}
		}
	}
	return nil
}

// FormatDatasetRecipe writes the yaml alice (or bob) would commit.
func FormatDatasetRecipe(rec DatasetRecipe) ([]byte, error) {
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

// RecipeFromDataset builds the portable file for a mount/file Workspace.
// Pure federated-read recipes (no Path and no File) return ok=false: they have
// nothing to hitchhike on git.
func RecipeFromDataset(def Dataset) (DatasetRecipe, bool) {
	if len(def.Sources) == 0 {
		return DatasetRecipe{}, false
	}
	rec := DatasetRecipe{Name: def.SetID, Metadata: maps.Clone(def.Metadata)}
	for _, src := range def.Sources {
		if src.IsFileEntry() {
			rec.Files = append(rec.Files, DatasetFileMount{
				Repository: string(src.Repository),
				Selector:   src.Selector,
				File:       src.File,
				Target:     src.Target,
				BaseRev:    src.BaseRev,
			})
			continue
		}
		if src.Path == nil {
			return DatasetRecipe{}, false
		}
		rec.Mounts = append(rec.Mounts, DatasetMount{
			Repository: string(src.Repository),
			Selector:   src.Selector,
			Path:       *src.Path,
			SubPath:    src.SubPath,
			BaseRev:    src.BaseRev,
		})
	}
	return rec, len(rec.Mounts) > 0 || len(rec.Files) > 0
}

// Sources is the Dataset.Sources equivalent of this recipe.
func (r DatasetRecipe) Sources() []DatasetSource {
	out := make([]DatasetSource, 0, len(r.Mounts)+len(r.Files))
	for _, m := range r.Mounts {
		out = append(out, DatasetSource{
			Repository: kernel.RepositoryID(m.Repository),
			Selector:   m.Selector,
			Path:       MountPath(m.Path),
			SubPath:    m.SubPath,
			BaseRev:    m.BaseRev,
		})
	}
	for _, f := range r.Files {
		out = append(out, DatasetSource{
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
func RootMount(sources []DatasetSource) *DatasetSource {
	for i := range sources {
		src := &sources[i]
		if src.Path != nil && normalizeMountPath(*src.Path) == "" {
			return src
		}
	}
	return nil
}

// HasMountPaths reports a Workspace recipe (every source declared Path).
func HasMountPaths(sources []DatasetSource) bool {
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
