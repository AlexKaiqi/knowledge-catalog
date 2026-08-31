package dolt

import (
	"sort"
	"strconv"

	"kc/kernel"
	"kc/knowledge"
)

func (r *Repository) Diff(objectID knowledge.ObjectID, from, to kernel.CommitID) (knowledge.ObjectDiff, error) {
	read := func(commit kernel.CommitID) (*knowledge.KnowledgeValue, error) {
		value, err := r.Read(objectID, commit)
		if kernel.CodeOf(err) == kernel.ErrKnowledgeRefUnresolved {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &value, nil
	}
	left, err := read(from)
	if err != nil {
		return knowledge.ObjectDiff{}, err
	}
	right, err := read(to)
	if err != nil {
		return knowledge.ObjectDiff{}, err
	}
	return knowledge.ObjectDiff{ObjectID: objectID, FromCommit: from, ToCommit: to, From: left, To: right}, nil
}

func (r *Repository) FastChangedObjectIDs(from, to kernel.CommitID) ([]knowledge.ObjectID, error) {
	query := `SELECT DISTINCT TO_BASE64(CAST(COALESCE(to_object_id, from_object_id) AS BINARY)) AS object_id64
        FROM DOLT_DIFF(` + sqlString(string(from)) + "," + sqlString(string(to)) + `, 'kc_objects')
        WHERE COALESCE(to_status,'') <> COALESCE(from_status,'')
           OR COALESCE(to_object_digest,'') <> COALESCE(from_object_digest,'')
           OR COALESCE(to_declaration_digest,'') <> COALESCE(from_declaration_digest,'')`
	rows, err := r.base.NativeQuery(query)
	if err != nil {
		return nil, err
	}
	ids := make([]knowledge.ObjectID, 0, len(rows))
	for _, row := range rows {
		value, err := rowText64(row, "object_id64")
		if err != nil {
			return nil, err
		}
		if value != "" {
			ids = append(ids, knowledge.ObjectID(value))
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (r *Repository) Log(objectID knowledge.ObjectID, commit kernel.CommitID, limit int) ([]knowledge.ObjectRevision, error) {
	if !r.base.HasCommit(commit) {
		return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commit)
	}
	if limit <= 0 {
		limit = 50
	}
	key := objectKey(objectID)
	query := `SELECT to_commit AS commit_hash, to_status AS status,
            to_object_digest AS object_digest, to_declaration_digest AS declaration_digest
        FROM dolt_diff_kc_objects
        WHERE (to_object_key=` + sqlString(key) + ` OR from_object_key=` + sqlString(key) + `)
          AND to_commit IN (SELECT commit_hash FROM DOLT_LOG(` + sqlString(string(commit)) + `))
        ORDER BY to_commit_date DESC LIMIT ` + strconv.Itoa(limit)
	rows, err := r.base.NativeQuery(query)
	if err != nil {
		return nil, err
	}
	out := make([]knowledge.ObjectRevision, 0, len(rows))
	for _, row := range rows {
		status := knowledge.ResolutionStatus(rowString(row, "status"))
		if status == "" {
			status = knowledge.StatusRemoved
		}
		out = append(out, knowledge.ObjectRevision{
			Commit: kernel.CommitID(rowString(row, "commit_hash")), Status: status,
			Digest:            kernel.Digest(rowString(row, "object_digest")),
			DeclarationDigest: kernel.Digest(rowString(row, "declaration_digest")),
		})
	}
	return out, nil
}
