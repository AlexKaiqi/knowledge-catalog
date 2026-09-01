// Package dolt implements the native layer ② Knowledge Repository over Dolt
// versioned tables. Snapshot coordinates and refs remain owned by
// snapshot/dolt; no knowledge semantics are added to snapshot.Store.
package dolt

import (
	"os"
	"path/filepath"

	"kc/kernel"
	"kc/knowledge"
	"kc/snapshot"
	snapshotdolt "kc/snapshot/dolt"
)

const legacyRelationMigrationStamp = ".kc-knowledge-format-v2"

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
	_ snapshot.DirectoryReader   = (*Repository)(nil)
	_ snapshot.HistoryStore      = (*Repository)(nil)
)

func (*Repository) NativeKnowledgeRepository() {}

var nativeTables = []string{"kc_units", "kc_objects"}

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
        schema_object_key CHAR(64) NOT NULL DEFAULT '',
        value_source_json LONGTEXT,
        provenance_json LONGTEXT,
        value_json LONGTEXT NOT NULL,
        value_digest CHAR(64) NOT NULL,
        INDEX idx_kc_units_object (object_key),
        INDEX idx_kc_units_schema (schema_object_key)
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
        INDEX idx_kc_objects_schema (is_schema, status)
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
	stamp := filepath.Join(rootDir, legacyRelationMigrationStamp)
	if _, err := os.Stat(stamp); os.IsNotExist(err) {
		if err := removeLegacyRelationProjection(base); err != nil {
			return nil, err
		}
		if !base.Archived() {
			if err := os.WriteFile(stamp, []byte("relation-projection-removed-v1\n"), 0o600); err != nil {
				return nil, err
			}
		}
	} else if err != nil {
		return nil, err
	}
	if err := ensureSchemaReferrerIndex(base); err != nil {
		return nil, err
	}
	return &Repository{base: base}, nil
}

func Wrap(base *snapshotdolt.DoltRepository) (*Repository, error) {
	if _, err := base.EnsureNativeSchema(nativeTables, nativeSchema); err != nil {
		return nil, err
	}
	if err := removeLegacyRelationProjection(base); err != nil {
		return nil, err
	}
	if err := ensureSchemaReferrerIndex(base); err != nil {
		return nil, err
	}
	return &Repository{base: base}, nil
}

// ensureSchemaReferrerIndex adds the bounded reverse schema_ref column to a
// Repository created before it existed and backfills it from the canonical
// schema_ref already stored on each unit. It is derived data, so a failure
// leaves no partial state: the whole statement batch is one native commit.
func ensureSchemaReferrerIndex(base *snapshotdolt.DoltRepository) error {
	rows, err := base.NativeQuery("SHOW COLUMNS FROM kc_units LIKE 'schema_object_key'")
	if err != nil {
		return err
	}
	if len(rows) > 0 || base.Archived() {
		return nil
	}
	units, err := base.NativeQuery(`SELECT unit_key, TO_BASE64(CAST(schema_ref AS BINARY)) AS schema_ref64
        FROM kc_units WHERE schema_ref <> '' ORDER BY unit_key`)
	if err != nil {
		return err
	}
	statements := []string{
		"ALTER TABLE kc_units ADD COLUMN schema_object_key CHAR(64) NOT NULL DEFAULT ''",
		"ALTER TABLE kc_units ADD INDEX idx_kc_units_schema (schema_object_key)",
	}
	for _, row := range units {
		ref, err := rowText64(row, "schema_ref64")
		if err != nil {
			return err
		}
		parsed, ok := knowledge.ParseSchemaRef(ref)
		if !ok {
			continue
		}
		statements = append(statements, "UPDATE kc_units SET schema_object_key="+
			sqlString(objectKey(parsed.Object))+" WHERE unit_key="+sqlString(rowString(row, "unit_key")))
	}
	head, err := base.Head(snapshot.DefaultRef)
	if err != nil {
		return err
	}
	_, err = base.ApplyNativeCommit(snapshotdolt.NativeCommit{
		TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head,
		Statements: statements, Tables: []string{"."}, Message: "index schema_ref referrers",
	})
	return err
}

// removeLegacyRelationProjection drops data that was always a discardable
// relation locator. Canonical units remain untouched and are re-indexed by the
// configured layer ③ provider.
func removeLegacyRelationProjection(base *snapshotdolt.DoltRepository) error {
	statements := []string{}
	if rows, err := base.NativeQuery("SHOW TABLES LIKE 'kc_relation_endpoints'"); err != nil {
		return err
	} else if len(rows) > 0 {
		statements = append(statements, "DROP TABLE kc_relation_endpoints")
	}
	if rows, err := base.NativeQuery("SHOW COLUMNS FROM kc_objects LIKE 'relation_type'"); err != nil {
		return err
	} else if len(rows) > 0 {
		statements = append(statements, "ALTER TABLE kc_objects DROP COLUMN relation_type")
	}
	if len(statements) == 0 || base.Archived() {
		return nil
	}
	head, err := base.Head(snapshot.DefaultRef)
	if err != nil {
		return err
	}
	_, err = base.ApplyNativeCommit(snapshotdolt.NativeCommit{
		TargetRef: snapshot.DefaultRef, BaseCommit: head, ExpectedTargetCommit: head,
		Statements: statements, Tables: []string{"."}, Message: "remove legacy relation locator projection",
	})
	return err
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
func (r *Repository) ReadDirectory(request snapshot.DirectoryRequest) (snapshot.DirectoryPage, error) {
	return r.base.ReadDirectory(request)
}
func (r *Repository) ApplyTreeCommit(change snapshot.TreeChangeSet) (kernel.CommitID, error) {
	return r.base.ApplyTreeCommit(change)
}
func (r *Repository) CommitHistory(commit kernel.CommitID, limit int) ([]kernel.CommitID, error) {
	return r.base.CommitHistory(commit, limit)
}
