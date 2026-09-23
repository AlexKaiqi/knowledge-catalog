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
// --home. It is analogous to Android Repo's local_manifests: add / replace / remove mounts
// without rewriting the shared recipe. Empty principal is the home owner.
func OverlayFile(home, principal, setID string) string {
	if strings.TrimSpace(principal) == "" {
		principal = overlayOwner
	}
	return filepath.Join(home, "overlays", fileToken(principal), fileToken(setID)+yamlExt)
}

// KnowledgeSetOverlay is the on-disk shape of OverlayFile. It is not a Catalog
// registry object and never hitchhikes on git.
type KnowledgeSetOverlay struct {
	Name   string                  `yaml:"name,omitempty" json:"name,omitempty"`
	Remove []string                `yaml:"remove,omitempty" json:"remove,omitempty"`
	Mounts []KnowledgeSetMount     `yaml:"mounts,omitempty" json:"mounts,omitempty"`
	Files  []KnowledgeSetFileMount `yaml:"files,omitempty" json:"files,omitempty"`
}

func ParseKnowledgeSetOverlay(raw []byte) (KnowledgeSetOverlay, error) {
	var over KnowledgeSetOverlay
	if err := yaml.Unmarshal(raw, &over); err != nil {
		return KnowledgeSetOverlay{}, kernel.Fail(kernel.ErrUsageInvalid, "invalid dataset overlay: %v", err)
	}
	for i, m := range over.Mounts {
		if strings.TrimSpace(m.Repository) == "" || strings.TrimSpace(m.Selector) == "" {
			return KnowledgeSetOverlay{}, kernel.Fail(kernel.ErrUsageInvalid, "overlay mount %d needs repository and selector", i)
		}
	}
	for i, f := range over.Files {
		if strings.TrimSpace(f.Repository) == "" || strings.TrimSpace(f.Selector) == "" ||
			strings.TrimSpace(f.File) == "" || strings.TrimSpace(f.Target) == "" {
			return KnowledgeSetOverlay{}, kernel.Fail(kernel.ErrUsageInvalid,
				"overlay file %d needs repository, selector, file and target", i)
		}
	}
	for i, id := range over.Remove {
		if strings.TrimSpace(id) == "" {
			return KnowledgeSetOverlay{}, kernel.Fail(kernel.ErrUsageInvalid, "overlay remove %d is empty", i)
		}
	}
	return over, nil
}

func FormatKnowledgeSetOverlay(over KnowledgeSetOverlay) ([]byte, error) {
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

func (o KnowledgeSetOverlay) empty() bool {
	return len(o.Remove) == 0 && len(o.Mounts) == 0 && len(o.Files) == 0
}

// MergeOverlay applies a local overlay onto a shared recipe. The result is
// not persisted. Remove drops every mount and file entry of a repository id;
// an overlay mount replaces the base mount at the same Workspace path or adds
// a new path, and an overlay file replaces the base file entry at the same
// delivered target or adds a new one.
func MergeOverlay(base KnowledgeSet, overlay KnowledgeSetOverlay) (KnowledgeSet, error) {
	if overlay.empty() {
		return base, nil
	}
	if overlay.Name != "" && overlay.Name != base.SetID {
		return KnowledgeSet{}, kernel.Fail(kernel.ErrUsageInvalid,
			"overlay name %s does not match workspace %s", overlay.Name, base.SetID)
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
			return KnowledgeSet{}, kernel.Fail(kernel.ErrUsageInvalid,
				"overlay remove names repository %s which is not in workspace %s", id, base.SetID)
		}
		drop[rid] = struct{}{}
	}
	sources := make([]KnowledgeSetSource, 0, len(base.Sources)+len(overlay.Mounts)+len(overlay.Files))
	index := map[string]int{}
	fileIndex := map[string]int{}
	for _, src := range base.Sources {
		if _, skip := drop[src.Repository]; skip {
			continue
		}
		if !src.IsFileEntry() {
			if src.Path == nil {
				return KnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid,
					"workspace %s has a source without a mount path; a mount overlay cannot be applied", base.SetID)
			}
			index[normalizeMountPath(*src.Path)] = len(sources)
		} else {
			fileIndex[normalizeMountPath(src.Target)] = len(sources)
		}
		sources = append(sources, src)
	}
	// The surviving base may already be frozen (a published definition used as
	// the local overlay base). Overlay YAML never carries commits, so an
	// overlay entry that names the same repository and the same selector
	// inherits that repository's one frozen coordinate instead of splitting
	// it; a different selector stays a genuine coordinate conflict and is
	// rejected by validateSourceCoordinates below.
	frozen := map[kernel.RepositoryID]KnowledgeSetSource{}
	for _, src := range sources {
		if src.Commit == "" && src.BaseRev == "" {
			continue
		}
		rep, seen := frozen[src.Repository]
		if !seen {
			frozen[src.Repository] = src
			continue
		}
		if rep.Commit != src.Commit || rep.BaseRev != src.BaseRev || rep.Selector != src.Selector {
			delete(frozen, src.Repository) // inconsistent base; validation rejects it later
		}
	}
	inherit := func(src KnowledgeSetSource) KnowledgeSetSource {
		if src.Commit != "" || src.BaseRev != "" {
			return src
		}
		if rep, ok := frozen[src.Repository]; ok && rep.Selector == src.Selector {
			src.Commit, src.BaseRev = rep.Commit, rep.BaseRev
		}
		return src
	}
	seenOverlay := map[string]struct{}{}
	for _, m := range overlay.Mounts {
		rid := kernel.RepositoryID(m.Repository)
		mountPath := normalizeMountPath(m.Path)
		if _, dup := seenOverlay[mountPath]; dup {
			return KnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "mount path %s appears twice in the overlay", mountLabel(mountPath))
		}
		seenOverlay[mountPath] = struct{}{}
		src := inherit(KnowledgeSetSource{
			Repository: rid,
			Selector:   m.Selector,
			Path:       MountPath(m.Path),
			SubPath:    m.SubPath,
			BaseRev:    m.BaseRev,
		})
		if i, ok := index[mountPath]; ok {
			sources[i] = src
			continue
		}
		index[mountPath] = len(sources)
		sources = append(sources, src)
	}
	seenFiles := map[string]struct{}{}
	for _, f := range overlay.Files {
		if _, skip := drop[kernel.RepositoryID(f.Repository)]; skip {
			continue
		}
		target := normalizeMountPath(f.Target)
		if _, dup := seenFiles[target]; dup {
			return KnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "delivered target %s appears twice in the overlay", target)
		}
		seenFiles[target] = struct{}{}
		src := inherit(KnowledgeSetSource{
			Repository: kernel.RepositoryID(f.Repository),
			Selector:   f.Selector,
			File:       f.File,
			Target:     f.Target,
			BaseRev:    f.BaseRev,
		})
		// Same slot rule as mounts: the overlay owns the delivered target it
		// names, replacing whatever file entry held it before.
		if i, ok := fileIndex[target]; ok {
			sources[i] = src
			continue
		}
		fileIndex[target] = len(sources)
		sources = append(sources, src)
	}
	if len(sources) == 0 {
		return KnowledgeSet{}, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "a workspace must contain at least one repository")
	}
	if err := validateMountPaths(sources); err != nil {
		return KnowledgeSet{}, err
	}
	if err := validateSourceCoordinates(sources); err != nil {
		return KnowledgeSet{}, err
	}
	out := base
	out.Sources = sources
	out.Items = nil
	return out, nil
}
