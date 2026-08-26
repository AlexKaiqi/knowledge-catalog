package catalog

import (
	"path"
	"strings"

	"kc/kernel"
)

// normalizeMountPath strips slashes so "", "/", and "." all mean root, and
// "refs/semantic/" and "refs//semantic" normalize to the same key.
func normalizeMountPath(p string) string {
	trimmed := strings.Trim(strings.TrimSpace(p), "/")
	if trimmed == "" || trimmed == "." {
		return ""
	}
	return path.Clean(trimmed)
}

// validateMountPaths enforces docs/COMPOSITION.md §2.4: once any source
// declares a mount Path, every source must (invariant 1 — no implicit
// ownership), and declared paths must not collide or nest (invariant 2 — a
// path belongs to exactly one mount). A recipe with no Path at all is left
// alone: it only feeds federated knowledge reads and never needed mounts.
func validateMountPaths(sources []WorkspaceSource) error {
	declared := 0
	for _, src := range sources {
		if src.Path != nil {
			declared++
			if err := validateRelativeTreePath("mount path", *src.Path, true); err != nil {
				return err
			}
			if err := validateRelativeTreePath("mount subPath", src.SubPath, true); err != nil {
				return err
			}
		} else if strings.TrimSpace(src.SubPath) != "" {
			return kernel.Fail(kernel.ErrWorkspaceInvalid, "repository %s declares subPath without a mount path", src.Repository)
		}
	}
	if declared == 0 {
		return nil
	}
	if declared != len(sources) {
		return kernel.Fail(kernel.ErrWorkspaceInvalid,
			"mount path must be declared on every source once any source declares one (root is Path: \"\")")
	}
	normalized := make([]string, len(sources))
	owners := map[string]kernel.RepositoryID{}
	for i, src := range sources {
		norm := normalizeMountPath(*src.Path)
		normalized[i] = norm
		if owner, dup := owners[norm]; dup {
			return kernel.Fail(kernel.ErrWorkspaceInvalid,
				"mount path %s is claimed by both %s and %s", mountLabel(norm), owner, src.Repository)
		}
		owners[norm] = src.Repository
	}
	for i, a := range normalized {
		if a == "" {
			continue // root is the fallback, not a prefix any other mount can nest under
		}
		for j, b := range normalized {
			if i == j || b == "" {
				continue
			}
			if strings.HasPrefix(b, a+"/") {
				return kernel.Fail(kernel.ErrWorkspaceInvalid,
					"mount path %s (%s) nests inside mount path %s (%s)",
					mountLabel(b), sources[j].Repository, mountLabel(a), sources[i].Repository)
			}
		}
	}
	return nil
}

func validateRelativeTreePath(label, value string, allowRoot bool) error {
	if strings.ContainsRune(value, '\x00') || strings.Contains(value, "\\") {
		return kernel.Fail(kernel.ErrWorkspaceInvalid, "%s %q is not a portable repository path", label, value)
	}
	clean := normalizeMountPath(value)
	if clean == "" && allowRoot {
		return nil
	}
	if clean == "" || clean == ".." || strings.HasPrefix(clean, "../") {
		return kernel.Fail(kernel.ErrWorkspaceInvalid, "%s %q escapes its tree root", label, value)
	}
	return nil
}

func mountLabel(norm string) string {
	if norm == "" {
		return "<root>"
	}
	return norm
}

// MountRoute is one workspace file's ownership: which member repository it
// belongs to, and its path inside that repository once the owning mount's
// Path/SubPath prefixes are undone. It is a pure function of the recipe's
// mounts, not of a resolved commit — the same file always routes the same
// way regardless of which commit each member is currently pinned at.
type MountRoute struct {
	Repository kernel.RepositoryID
	Path       string
}

// RouteMount finds the mount owning workspacePath by longest declared-path
// match, falling back to the root mount (Path: "") when no other mount
// claims it. See docs/COMPOSITION.md §2.2 (write-back routing).
//
// ErrUsageInvalid when def declares no mount paths at all (it is a
// federated-read recipe, not a workspace) or when no mount owns the path.
func RouteMount(def WorkspaceDefinition, workspacePath string) (MountRoute, error) {
	clean := normalizeMountPath(workspacePath)
	if clean == "" {
		return MountRoute{}, kernel.Fail(kernel.ErrUsageInvalid, "workspace path is empty")
	}
	var best, root *WorkspaceSource
	bestNorm := ""
	declared := false
	for i := range def.Sources {
		src := &def.Sources[i]
		if src.Path == nil {
			continue
		}
		declared = true
		norm := normalizeMountPath(*src.Path)
		if norm == "" {
			root = src
			continue
		}
		if norm != clean && !strings.HasPrefix(clean, norm+"/") {
			continue
		}
		if best == nil || len(norm) > len(bestNorm) {
			best, bestNorm = src, norm
		}
	}
	if !declared {
		return MountRoute{}, kernel.Fail(kernel.ErrUsageInvalid,
			"workspace %s declares no mount paths; it is a federated-read recipe, not a workspace", def.WorkspaceID)
	}
	if best != nil {
		inRepo := ""
		if clean != bestNorm {
			inRepo = strings.TrimPrefix(clean, bestNorm+"/")
		}
		return MountRoute{Repository: best.Repository, Path: joinSubPath(best.SubPath, inRepo)}, nil
	}
	if root != nil {
		return MountRoute{Repository: root.Repository, Path: joinSubPath(root.SubPath, clean)}, nil
	}
	return MountRoute{}, kernel.Fail(kernel.ErrUsageInvalid, "no mount in workspace %s owns path %s", def.WorkspaceID, workspacePath)
}

// RouteMounts partitions many workspace paths by owning repository, so an
// edit spanning several mounts becomes N independent per-repository batches
// instead of one write pretending to span repositories (K-01, invariant 5:
// one write, one repository — cross-mount edits split, they do not merge).
func RouteMounts(def WorkspaceDefinition, workspacePaths []string) (map[kernel.RepositoryID][]MountRoute, error) {
	out := map[kernel.RepositoryID][]MountRoute{}
	for _, p := range workspacePaths {
		route, err := RouteMount(def, p)
		if err != nil {
			return nil, err
		}
		out[route.Repository] = append(out[route.Repository], route)
	}
	return out, nil
}

func joinSubPath(subPath, inRepo string) string {
	subPath = strings.Trim(subPath, "/")
	inRepo = strings.Trim(inRepo, "/")
	switch {
	case subPath == "":
		return inRepo
	case inRepo == "":
		return subPath
	default:
		return subPath + "/" + inRepo
	}
}
