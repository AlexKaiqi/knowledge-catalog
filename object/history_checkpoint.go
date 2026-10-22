package object

import "kc/kernel"

// HistoryCheckpoint is service-issued continuation metadata. Open contains
// unfinished sources only; nil progress means that source has not been scanned.
// It retains no permission and must be reauthorized before any source access.
type HistoryCheckpoint struct {
	Version int                                          `json:"version"`
	Basis   ReadBasis                                    `json:"basis"`
	Ref     string                                       `json:"ref,omitempty"`
	Open    map[kernel.RepositoryID]*HistoryScanProgress `json:"open"`
}

func (c HistoryCheckpoint) Validate() error {
	if err := c.Basis.Validate(); err != nil {
		return err
	}
	ref, err := ParseReference(c.Basis.Reference)
	if err != nil {
		return err
	}
	if c.Version != 1 || c.Basis.Shape != "" || ref.Locator != "" || len(c.Open) == 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "invalid history checkpoint scope or version")
	}
	sources := map[kernel.RepositoryID]ReadSource{}
	for _, source := range c.Basis.Candidates {
		sources[source.Repository] = source
	}
	for repository, progress := range c.Open {
		source, ok := sources[repository]
		if !ok {
			return kernel.Fail(kernel.ErrUsageInvalid, "history checkpoint contains an unselected source")
		}
		if progress == nil {
			continue
		}
		if err := progress.Validate(ref.Object, source, ""); err != nil {
			return err
		}
	}
	return nil
}
