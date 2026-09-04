package index

import (
	"encoding/json"
	"strings"

	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
)

// ChangeNotice is the inbound invalidation for Bound State. It locates what
// to refresh and never carries an observation value. Policy:
// docs/PROJECTION_CONTROLLER.md §3.2.
type ChangeNotice struct {
	Repository     kernel.RepositoryID `json:"repository"`
	Ref            string              `json:"ref,omitempty"`
	Address        *knowledge.Address  `json:"address,omitempty"`
	SourceRevision string              `json:"sourceRevision,omitempty"`
}

var changeNoticeFields = map[string]struct{}{
	"repository":     {},
	"ref":            {},
	"address":        {},
	"sourceRevision": {},
}

// ParseChangeNotice decodes one observer payload. Unknown fields — including
// value/body/payload — are USAGE_INVALID so a notice cannot become a write.
func ParseChangeNotice(raw []byte) (ChangeNotice, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ChangeNotice{}, kernel.Fail(kernel.ErrUsageInvalid, "change notice must be a JSON object")
	}
	for key := range probe {
		if _, ok := changeNoticeFields[key]; !ok {
			return ChangeNotice{}, kernel.Fail(kernel.ErrUsageInvalid, "change notice must not carry %s", key)
		}
	}
	var notice ChangeNotice
	if err := json.Unmarshal(raw, &notice); err != nil {
		return ChangeNotice{}, kernel.Fail(kernel.ErrUsageInvalid, "change notice is invalid")
	}
	return notice, ValidateChangeNotice(notice)
}

func ValidateChangeNotice(notice ChangeNotice) error {
	if strings.TrimSpace(string(notice.Repository)) == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "change notice requires repository")
	}
	if notice.Address != nil && strings.TrimSpace(string(notice.Address.ObjectID)) == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "change notice address requires objectId")
	}
	return nil
}

func (n ChangeNotice) refOrDefault() string {
	return snapshot.RefOrDefault(n.Ref)
}
