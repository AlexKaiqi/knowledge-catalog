package cli

import (
	"encoding/base64"
	"encoding/json"
	"sort"

	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/reader"
)

const (
	defaultSchemaPageSize = 50
	maxSchemaPageSize     = 200
)

type schemaPageCursor struct {
	Version    int                 `json:"version"`
	Repository kernel.RepositoryID `json:"repository"`
	Commit     kernel.CommitID     `json:"commit"`
	After      knowledge.ObjectID  `json:"after"`
	Check      kernel.Digest       `json:"check"`
}

type schemaPageCoverage struct {
	Enumerated int  `json:"enumerated"`
	Total      int  `json:"total"`
	Complete   bool `json:"complete"`
}

type schemaPageResponse struct {
	Repository   kernel.RepositoryID        `json:"repository"`
	Commit       kernel.CommitID            `json:"commit"`
	Schemas      []reader.SchemaDescription `json:"schemas"`
	Coverage     schemaPageCoverage         `json:"coverage"`
	Continuation string                     `json:"continuation,omitempty"`
	Exhausted    bool                       `json:"exhausted"`
}

func schemaCursorCheck(cursor schemaPageCursor) kernel.Digest {
	return kernel.CanonicalDigest(struct {
		Version    int
		Repository kernel.RepositoryID
		Commit     kernel.CommitID
		After      knowledge.ObjectID
	}{cursor.Version, cursor.Repository, cursor.Commit, cursor.After})
}

func encodeSchemaPageCursor(repository kernel.RepositoryID, commit kernel.CommitID, after knowledge.ObjectID) string {
	cursor := schemaPageCursor{Version: 1, Repository: repository, Commit: commit, After: after}
	cursor.Check = schemaCursorCheck(cursor)
	raw, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeSchemaPageCursor(raw string, repository kernel.RepositoryID, commit kernel.CommitID) (knowledge.ObjectID, error) {
	if raw == "" {
		return "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "schema continuation is invalid")
	}
	var cursor schemaPageCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Version != 1 ||
		cursor.Repository != repository || cursor.Commit != commit || cursor.Check != schemaCursorCheck(cursor) {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "schema continuation does not match the fixed Repository basis")
	}
	return cursor.After, nil
}

// verbBrowseSchemas is bounded discovery over one explicitly pinned
// Repository. It is intentionally separate from DESCRIBE_SCHEMA over a
// Workspace: discovery may be used before a consumer chooses a knowledge set.
func verbBrowseSchemas(cx *invocation) (any, error) {
	repositoryID, commitID, err := pinCommit(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	limit, err := limitFrom(cx.Flags, defaultSchemaPageSize)
	if err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = defaultSchemaPageSize
	}
	if limit > maxSchemaPageSize {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "schema page limit cannot exceed %d", maxSchemaPageSize)
	}
	after, err := decodeSchemaPageCursor(cx.flag("continuation"), repositoryID, commitID)
	if err != nil {
		return nil, err
	}
	repo, err := cx.WS.Reader.Require(repositoryID, kernel.ErrCapabilityUnsatisfied)
	if err != nil {
		return nil, err
	}
	store, ok := repo.(knowledge.SchemaStore)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"repository %s does not provide bounded schema discovery", repositoryID)
	}
	ids, err := store.SchemaObjectIDs(commitID)
	if err != nil {
		return nil, err
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	start := 0
	if after != "" {
		start = sort.Search(len(ids), func(i int) bool { return ids[i] > after })
	}
	end := start + limit
	if end > len(ids) {
		end = len(ids)
	}
	descriptions := make([]reader.SchemaDescription, 0, end-start)
	for _, objectID := range ids[start:end] {
		report, describeErr := reader.DescribeRepoSchema(repo, commitID, objectID)
		if describeErr != nil {
			return nil, describeErr
		}
		descriptions = append(descriptions, report.Schemas...)
	}
	exhausted := end == len(ids)
	continuation := ""
	if !exhausted && end > start {
		continuation = encodeSchemaPageCursor(repositoryID, commitID, ids[end-1])
	}
	return schemaPageResponse{
		Repository: repositoryID, Commit: commitID, Schemas: descriptions,
		Coverage:     schemaPageCoverage{Enumerated: len(descriptions), Total: len(ids), Complete: exhausted},
		Continuation: continuation, Exhausted: exhausted,
	}, nil
}
