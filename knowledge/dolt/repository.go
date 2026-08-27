// Package dolt implements the native layer ② Knowledge Repository over Dolt
// versioned tables. Snapshot coordinates and refs remain owned by
// snapshot/dolt; no knowledge semantics are added to snapshot.Store.
package dolt

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
	snapshotdolt "kc/snapshot/dolt"
)

type Repository struct {
	base *snapshotdolt.DoltRepository
}

var (
	_ knowledge.Repository       = (*Repository)(nil)
	_ knowledge.NativeRepository = (*Repository)(nil)
	_ knowledge.BatchReadStore   = (*Repository)(nil)
	_ knowledge.ChangeStore      = (*Repository)(nil)
	_ knowledge.FastChanges      = (*Repository)(nil)
	_ snapshot.TreeStore         = (*Repository)(nil)
	_ snapshot.HistoryStore      = (*Repository)(nil)
)

func (*Repository) NativeKnowledgeRepository() {}

var nativeTables = []string{"kc_units", "kc_objects", "kc_relation_endpoints"}

var nativeSchema = []string{
	`CREATE TABLE IF NOT EXISTS kc_units (
        unit_key CHAR(64) PRIMARY KEY,
        object_key CHAR(64) NOT NULL,
        object_id LONGTEXT NOT NULL,
        kind VARCHAR(32) NOT NULL,
        aspect_name LONGTEXT NOT NULL,
        member_key LONGTEXT NOT NULL,
        path_hint LONGTEXT NOT NULL,
        storage_path LONGTEXT NOT NULL,
        schema_ref LONGTEXT NOT NULL,
        value_source_json LONGTEXT,
        provenance_json LONGTEXT,
        value_json LONGTEXT NOT NULL,
        value_digest CHAR(64) NOT NULL,
        INDEX idx_kc_units_object (object_key)
    )`,
	`CREATE TABLE IF NOT EXISTS kc_objects (
        object_key CHAR(64) PRIMARY KEY,
        object_id LONGTEXT NOT NULL,
        kind VARCHAR(32) NOT NULL,
        is_schema BOOLEAN NOT NULL,
        status VARCHAR(16) NOT NULL,
        unit_count BIGINT NOT NULL,
        object_digest CHAR(64) NOT NULL,
        declaration_digest CHAR(64) NOT NULL,
        relation_type LONGTEXT NOT NULL,
        INDEX idx_kc_objects_schema (is_schema, status)
    )`,
	`CREATE TABLE IF NOT EXISTS kc_relation_endpoints (
        endpoint_row_key CHAR(64) PRIMARY KEY,
        relation_key CHAR(64) NOT NULL,
        relation_object_id LONGTEXT NOT NULL,
        endpoint_key CHAR(64) NOT NULL,
        endpoint_object_id LONGTEXT NOT NULL,
        role LONGTEXT NOT NULL,
        ordinal BIGINT NOT NULL,
        INDEX idx_kc_relation (relation_key),
        INDEX idx_kc_endpoint (endpoint_key)
    )`,
}

func Open(rootDir string, id kernel.RepositoryID) (*Repository, error) {
	base, err := snapshotdolt.OpenDolt(rootDir, id)
	if err != nil {
		return nil, err
	}
	if _, err := base.EnsureNativeSchema(nativeTables, nativeSchema); err != nil {
		return nil, err
	}
	return &Repository{base: base}, nil
}

func Wrap(base *snapshotdolt.DoltRepository) (*Repository, error) {
	if _, err := base.EnsureNativeSchema(nativeTables, nativeSchema); err != nil {
		return nil, err
	}
	return &Repository{base: base}, nil
}

func (r *Repository) ID() kernel.RepositoryID                   { return r.base.ID() }
func (r *Repository) Head(ref string) (kernel.CommitID, error)  { return r.base.Head(ref) }
func (r *Repository) GetRef(ref string) (kernel.CommitID, bool) { return r.base.GetRef(ref) }
func (r *Repository) HasCommit(commit kernel.CommitID) bool     { return r.base.HasCommit(commit) }
func (r *Repository) CreateRef(ref string, commit kernel.CommitID) error {
	return r.base.CreateRef(ref, commit)
}
func (r *Repository) Merge(ref string, candidate, expected kernel.CommitID) (kernel.CommitID, error) {
	return r.base.Merge(ref, candidate, expected)
}
func (r *Repository) Archived() bool { return r.base.Archived() }
func (r *Repository) Archive() error { return r.base.Archive() }

func (r *Repository) ReadFile(path string, commit kernel.CommitID) ([]byte, error) {
	return r.base.ReadFile(path, commit)
}
func (r *Repository) ListFiles(commit kernel.CommitID) ([]string, error) {
	return r.base.ListFiles(commit)
}
func (r *Repository) ApplyTreeCommit(change snapshot.TreeChangeSet) (kernel.CommitID, error) {
	return r.base.ApplyTreeCommit(change)
}
func (r *Repository) CommitHistory(commit kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	return r.base.CommitHistory(commit, limit)
}
