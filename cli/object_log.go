package cli

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
	Exhausted    bool               `json:"exhausted"`
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

func collectObjectLogPage(
	home string,
	flags map[string]FlagValue,
	objectID knowledge.ObjectID,
	pin map[kernel.RepositoryID]kernel.CommitID,
	limit int,
	continuation string,
	readLog func(kernel.RepositoryID, kernel.CommitID, knowledge.ObjectLogQuery) ([]knowledge.ObjectRevision, error),
) (objectLogPage, error) {
	basis := objectLogBasis(pin)
	after, err := decodeObjectLogCursor(continuation, objectID, basis)
	if err != nil {
		return objectLogPage{}, err
	}
	logs := []reader.ObjectLog{}
	nextAfter := map[string]string{}
	for _, repositoryID := range pinnedRepositoryIDs(pin) {
		if !allowedRepoRead(home, flags, string(repositoryID), string(objectID)) {
			continue
		}
		repoKey := string(repositoryID)
		if after != nil {
			if _, open := after[repoKey]; !open {
				continue
			}
		}
		query := knowledge.ObjectLogQuery{Limit: limit + 1}
		if after != nil {
			query.After = kernel.CommitID(after[repoKey])
		}
		revisions, logErr := readLog(repositoryID, pin[repositoryID], query)
		if logErr != nil {
			if kernel.CodeOf(logErr) == kernel.ErrKnowledgeRefUnresolved {
				continue
			}
			return objectLogPage{}, logErr
		}
		if len(revisions) == 0 {
			continue
		}
		more := len(revisions) > limit
		if more {
			revisions = revisions[:limit]
			nextAfter[repoKey] = string(revisions[len(revisions)-1].Commit)
		}
		logs = append(logs, reader.ObjectLog{
			Repository: repositoryID,
			ObjectID:   objectID,
			Commit:     pin[repositoryID],
			Revisions:  revisions,
		})
	}
	page := objectLogPage{Logs: logs, Exhausted: len(nextAfter) == 0}
	if !page.Exhausted {
		page.Continuation = encodeObjectLogCursor(objectID, basis, nextAfter)
	}
	return page, nil
}
