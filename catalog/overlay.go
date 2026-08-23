package catalog

import (
	"bytes"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"kc/kernel"
)

const overlayOwner = "owner"

// OverlayFile is the local-only overlay for one (principal, workspace) in this
// --home. It is Android repo's local_manifests: add / replace / remove mounts
// without rewriting the shared recipe. Empty principal is the home owner.
func OverlayFile(home, principal, workspaceID string) string {
	if strings.TrimSpace(principal) == "" {
		principal = overlayOwner
	}
	return filepath.Join(home, "overlays", fileToken(principal), fileToken(workspaceID)+yamlExt)
}

// WorkspaceOverlay is the on-disk shape of OverlayFile. It is not a Catalog
// registry object and never hitchhikes on git.
type WorkspaceOverlay struct {
	Name   string           `yaml:"name,omitempty" json:"name,omitempty"`
	Remove []string         `yaml:"remove,omitempty" json:"remove,omitempty"`
	Mounts []WorkspaceMount `yaml:"mounts,omitempty" json:"mounts,omitempty"`
}

func ParseWorkspaceOverlay(raw []byte) (WorkspaceOverlay, error) {
	var over WorkspaceOverlay
	if err := yaml.Unmarshal(raw, &over); err != nil {
		return WorkspaceOverlay{}, kernel.Fail(kernel.ErrUsageInvalid, "invalid workspace overlay: %v", err)
	}
	for i, m := range over.Mounts {
		if strings.TrimSpace(m.Repository) == "" || strings.TrimSpace(m.Selector) == "" {
			return WorkspaceOverlay{}, kernel.Fail(kernel.ErrUsageInvalid, "overlay mount %d needs repository and selector", i)
		}
	}
	for i, id := range over.Remove {
		if strings.TrimSpace(id) == "" {
			return WorkspaceOverlay{}, kernel.Fail(kernel.ErrUsageInvalid, "overlay remove %d is empty", i)
		}
	}
	return over, nil
}

func FormatWorkspaceOverlay(over WorkspaceOverlay) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(over); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (o WorkspaceOverlay) empty() bool {
	return len(o.Remove) == 0 && len(o.Mounts) == 0
}

// MergeOverlay applies a local overlay onto a shared recipe. The result is
// not persisted. Remove drops members by repository id; mounts add or replace
// by repository. Path uniqueness and one-repo-one-mount still hold.
func MergeOverlay(base WorkspaceDefinition, overlay WorkspaceOverlay) (WorkspaceDefinition, error) {
	if overlay.empty() {
		return base, nil
	}
	if overlay.Name != "" && overlay.Name != base.WorkspaceID {
		return WorkspaceDefinition{}, kernel.Fail(kernel.ErrUsageInvalid,
			"overlay name %s does not match workspace %s", overlay.Name, base.WorkspaceID)
	}
	drop := map[kernel.RepositoryID]struct{}{}
	for _, id := range overlay.Remove {
		rid := kernel.RepositoryID(id)
		found := false
		for _, src := range base.Sources {
			if src.Repository == rid {
				found = true
				break
			}
		}
		if !found {
			return WorkspaceDefinition{}, kernel.Fail(kernel.ErrUsageInvalid,
				"overlay remove names repository %s which is not in workspace %s", id, base.WorkspaceID)
		}
		drop[rid] = struct{}{}
	}
	sources := make([]WorkspaceSource, 0, len(base.Sources)+len(overlay.Mounts))
	index := map[kernel.RepositoryID]int{}
	for _, src := range base.Sources {
		if _, skip := drop[src.Repository]; skip {
			continue
		}
		index[src.Repository] = len(sources)
		sources = append(sources, src)
	}
	seenOverlay := map[kernel.RepositoryID]struct{}{}
	for _, m := range overlay.Mounts {
		rid := kernel.RepositoryID(m.Repository)
		if _, dup := seenOverlay[rid]; dup {
			return WorkspaceDefinition{}, kernel.Fail(kernel.ErrWorkspaceInvalid, "repository %s appears twice in the overlay", rid)
		}
		seenOverlay[rid] = struct{}{}
		src := WorkspaceSource{
			Repository: rid,
			Selector:   m.Selector,
			Path:       MountPath(m.Path),
			SubPath:    m.SubPath,
			BaseRev:    m.BaseRev,
		}
		if i, ok := index[rid]; ok {
			sources[i] = src
			continue
		}
		index[rid] = len(sources)
		sources = append(sources, src)
	}
	if len(sources) == 0 {
		return WorkspaceDefinition{}, kernel.Fail(kernel.ErrWorkspaceInvalid, "a workspace must contain at least one repository")
	}
	if err := validateMountPaths(sources); err != nil {
		return WorkspaceDefinition{}, err
	}
	out := base
	out.Sources = sources
	return out, nil
}
