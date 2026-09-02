package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kc/internal/gitdir"
	"kc/kernel"
)

// MountWrite is one dirty path in a checked-out mount, ready for
// Writer.RawWrite. Catalog does not interpret the file content.
type MountWrite struct {
	Repository kernel.RepositoryID `json:"repository"`
	Path       string              `json:"path"`
	Content    []byte              `json:"content,omitempty"`
	Remove     bool                `json:"remove,omitempty"`
}

func CollectMountChanges(mounts []MountCheckout) ([]MountWrite, error) {
	var out []MountWrite
	for _, m := range mounts {
		if m.Skipped || m.Dir == "" {
			continue
		}
		changes, err := gitdir.At(m.Dir).PorcelainChanges()
		if err != nil {
			return nil, fmt.Errorf("collect mount %s at %s: %w", m.Repository, mountLabel(m.Path), err)
		}
		for _, ch := range changes {
			writes, err := collectOneChange(m, ch)
			if err != nil {
				return nil, err
			}
			out = append(out, writes...)
		}
	}
	return out, nil
}

func collectOneChange(m MountCheckout, ch gitdir.WorktreeChange) ([]MountWrite, error) {
	rel := strings.Trim(ch.Path, "/")
	if ch.Removed {
		return []MountWrite{{Repository: m.Repository, Path: rel, Remove: true}}, nil
	}
	full := filepath.Join(m.Dir, filepath.FromSlash(rel))
	info, err := os.Stat(full)
	if err != nil {
		return nil, fmt.Errorf("stat %s in mount %s: %w", rel, m.Repository, err)
	}
	if !info.IsDir() {
		raw, err := os.ReadFile(full)
		if err != nil {
			return nil, fmt.Errorf("read %s in mount %s: %w", rel, m.Repository, err)
		}
		return []MountWrite{{Repository: m.Repository, Path: rel, Content: raw}}, nil
	}
	var out []MountWrite
	err = filepath.WalkDir(full, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		relFile, err := filepath.Rel(m.Dir, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, MountWrite{Repository: m.Repository, Path: filepath.ToSlash(relFile), Content: raw})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s in mount %s: %w", rel, m.Repository, err)
	}
	return out, nil
}
