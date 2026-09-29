package invocation

import (
	"encoding/base64"
	"encoding/json"
	"sort"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
)

type objectLogCursor struct {
	Version int                `json:"version"`
	Object  knowledge.ObjectID `json:"object"`
	Basis   map[string]string  `json:"basis"`
	After   map[string]string  `json:"after,omitempty"`
	Check   kernel.Digest      `json:"check"`
}

type objectLogPage struct {
	Logs         []reader.ObjectLog `json:"logs"`
	Continuation string             `json:"continuation,omitempty"`
}

func objectLogCursorCheck(cursor objectLogCursor) kernel.Digest {
	return kernel.CanonicalDigest(struct {
		Version int
		Object  knowledge.ObjectID
		Basis   map[string]string
		After   map[string]string
	}{cursor.Version, cursor.Object, cursor.Basis, cursor.After})
}

func encodeObjectLogCursor(objectID knowledge.ObjectID, basis, after map[string]string) string {
	cursor := objectLogCursor{Version: 1, Object: objectID, Basis: basis, After: after}
	cursor.Check = objectLogCursorCheck(cursor)
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeObjectLogCursor(raw string, objectID knowledge.ObjectID, basis map[string]string) (map[string]string, error) {
	if raw == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "log continuation is invalid")
	}
	var cursor objectLogCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Version != 1 ||
		cursor.Object != objectID || cursor.Check != objectLogCursorCheck(cursor) {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "log continuation does not match the fixed object basis")
	}
	if !stringMapsEqual(cursor.Basis, basis) {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "log continuation does not match the fixed object basis")
	}
	return cursor.After, nil
}

func stringMapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func objectLogBasis(pin map[kernel.RepositoryID]kernel.CommitID) map[string]string {
	basis := make(map[string]string, len(pin))
	for repositoryID, commitID := range pin {
		basis[string(repositoryID)] = string(commitID)
	}
	return basis
}

func pinnedRepositoryIDs(pin map[kernel.RepositoryID]kernel.CommitID) []kernel.RepositoryID {
	ids := make([]kernel.RepositoryID, 0, len(pin))
	for repositoryID := range pin {
		ids = append(ids, repositoryID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
