package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/reader"
)

type objectLogCursor struct {
	Version int                  `json:"version"`
	Object  objectmodel.ObjectID `json:"object"`
	Basis   map[string]string    `json:"basis"`
	After   map[string]string    `json:"after,omitempty"`
	Check   kernel.Digest        `json:"check"`
}

type ObjectLogPage struct {
	Logs         []reader.ObjectLog `json:"logs"`
	Continuation string             `json:"continuation,omitempty"`
}

func objectLogCursorCheck(cursor objectLogCursor) kernel.Digest {
	return kernel.CanonicalDigest(struct {
		Version int
		Object  objectmodel.ObjectID
		Basis   map[string]string
		After   map[string]string
	}{cursor.Version, cursor.Object, cursor.Basis, cursor.After})
}

func encodeObjectLogCursor(objectID objectmodel.ObjectID, basis, after map[string]string) string {
	cursor := objectLogCursor{Version: 1, Object: objectID, Basis: basis, After: after}
	cursor.Check = objectLogCursorCheck(cursor)
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeObjectLogCursor(raw string, objectID objectmodel.ObjectID, basis map[string]string) (map[string]string, error) {
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

type ObjectLogRequest struct {
	Object       objectmodel.ObjectID
	Pin          map[kernel.RepositoryID]kernel.CommitID
	Limit        int
	Continuation string
}
type ObjectLogExecutor struct {
	Allow   func(kernel.RepositoryID, objectmodel.ObjectID) bool
	ReadLog func(kernel.RepositoryID, kernel.CommitID, objectmodel.ObjectLogQuery) ([]objectmodel.ObjectRevision, error)
}

func (e ObjectLogExecutor) Execute(_ context.Context, request ObjectLogRequest) (ObjectLogPage, error) {
	if e.ReadLog == nil {
		return ObjectLogPage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "object log service is unavailable")
	}
	if request.Limit < 1 {
		return ObjectLogPage{}, kernel.Fail(kernel.ErrUsageInvalid, "object log limit must be positive")
	}
	objectID, pin, limit, continuation := request.Object, request.Pin, request.Limit, request.Continuation
	basis := objectLogBasis(pin)
	after, err := decodeObjectLogCursor(continuation, objectID, basis)
	if err != nil {
		return ObjectLogPage{}, err
	}
	logs := []reader.ObjectLog{}
	nextAfter := map[string]string{}
	for _, repositoryID := range pinnedRepositoryIDs(pin) {
		if e.Allow != nil && !e.Allow(repositoryID, objectID) {
			continue
		}
		repoKey := string(repositoryID)
		if after != nil {
			if _, open := after[repoKey]; !open {
				continue
			}
		}
		query := objectmodel.ObjectLogQuery{Limit: limit + 1}
		if after != nil {
			query.After = kernel.CommitID(after[repoKey])
		}
		revisions, logErr := e.ReadLog(repositoryID, pin[repositoryID], query)
		if logErr != nil {
			if kernel.CodeOf(logErr) == kernel.ErrObjectRefUnresolved {
				continue
			}
			return ObjectLogPage{}, logErr
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
	page := ObjectLogPage{Logs: logs}
	if len(nextAfter) > 0 {
		page.Continuation = encodeObjectLogCursor(objectID, basis, nextAfter)
	}
	return page, nil
}
