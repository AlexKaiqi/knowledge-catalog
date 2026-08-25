package catalog

import (
	"fmt"
	"strings"

	"kc/internal/gitdir"
	"kc/kernel"
)

// MountStatusReport is one mount's local git status. A skipped mount carries
// no git status and remains explicitly marked as skipped.
type MountStatusReport struct {
	Repository kernel.RepositoryID `json:"repository"`
	Path       string              `json:"path"`
	Commit     kernel.CommitID     `json:"commit"`
	Dirty      bool                `json:"dirty"`
	Changed    []string            `json:"changed,omitempty"`
	Skipped    bool                `json:"skipped,omitempty"`
}

func MountStatus(mounts []MountCheckout) ([]MountStatusReport, error) {
	out := make([]MountStatusReport, 0, len(mounts))
	for _, m := range mounts {
		if m.Skipped {
			out = append(out, MountStatusReport{Repository: m.Repository, Path: m.Path, Commit: m.Commit, Skipped: true})
			continue
		}
		raw, err := gitdir.At(m.Dir).Git("status", "--porcelain")
		if err != nil {
			return nil, fmt.Errorf("status mount %s at %s: %w", m.Repository, mountLabel(m.Path), err)
		}
		var changed []string
		for _, line := range strings.Split(raw, "\n") {
			if strings.TrimSpace(line) != "" {
				changed = append(changed, line)
			}
		}
		out = append(out, MountStatusReport{
			Repository: m.Repository,
			Path:       m.Path,
			Commit:     m.Commit,
			Dirty:      len(changed) > 0,
			Changed:    changed,
		})
	}
	return out, nil
}
