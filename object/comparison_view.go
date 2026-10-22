package object

import (
	"kc/kernel"
	"strings"
)

// ComparisonView names exactly one fixed view. An opaque retained basis grants
// no permission; Open must revalidate its publication and current access.
type ComparisonView struct {
	Basis      string              `json:"basis,omitempty"`
	Repository kernel.RepositoryID `json:"repository,omitempty"`
	Commit     kernel.CommitID     `json:"commit,omitempty"`
	Dataset    string              `json:"dataset,omitempty"`
	Revision   int                 `json:"revision,omitempty"`
}

func (v ComparisonView) Validate() error {
	count := 0
	if v.Basis != "" {
		count++
	}
	if v.Repository != "" || v.Commit != "" {
		count++
		if strings.TrimSpace(string(v.Repository)) == "" || strings.TrimSpace(string(v.Commit)) == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "object comparison requires both Repository and fixed commit")
		}
	}
	if v.Dataset != "" || v.Revision != 0 {
		count++
		if strings.TrimSpace(v.Dataset) == "" || v.Revision < 1 {
			return kernel.Fail(kernel.ErrUsageInvalid, "object comparison requires a named Dataset and positive fixed revision")
		}
	}
	if count != 1 || v.Basis != "" && strings.TrimSpace(v.Basis) == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "each comparison side requires exactly one fixed view")
	}
	return nil
}
