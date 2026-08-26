// Package sqlite implements the local layer ③ retrieval projection.
package sqlite

import (
	"database/sql"
	"kc/retrieval"
	"os"
	"path/filepath"
	"strconv"

	"kc/index"
	"kc/kernel"
	"kc/knowledge"

	_ "modernc.org/sqlite"
)

type sqliteEngine struct {
	db *sql.DB
	id kernel.RepositoryID
}

func Open(dir string, id kernel.RepositoryID) (index.Engine, error) {
	dsn := "file:kc-" + index.SanitizeID(string(id)) + "?mode=memory&cache=shared"
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		dsn = filepath.Join(dir, index.SanitizeID(string(id))+".sqlite")
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
		PRAGMA busy_timeout = 5000;
		CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS objects (object_id TEXT PRIMARY KEY);
		CREATE TABLE IF NOT EXISTS fields (
			object_id TEXT NOT NULL,
			path TEXT NOT NULL,
			value TEXT NOT NULL,
			PRIMARY KEY (object_id, path, value)
		);
		CREATE INDEX IF NOT EXISTS fields_path_value ON fields(path, value);
		CREATE VIRTUAL TABLE IF NOT EXISTS fts USING fts5(object_id UNINDEXED, value_text);
	`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrateSQLiteFields(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &sqliteEngine{db: db, id: id}, nil
}

func migrateSQLiteFields(db *sql.DB) error {
	var valuePK int
	if err := db.QueryRow(`SELECT pk FROM pragma_table_info('fields') WHERE name = 'value'`).Scan(&valuePK); err != nil {
		return err
	}
	if valuePK != 0 {
		return nil
	}
	_, err := db.Exec(`
		DROP INDEX IF EXISTS fields_path_value;
		ALTER TABLE fields RENAME TO fields_v2;
		CREATE TABLE fields (
			object_id TEXT NOT NULL,
			path TEXT NOT NULL,
			value TEXT NOT NULL,
			PRIMARY KEY (object_id, path, value)
		);
		INSERT OR IGNORE INTO fields(object_id, path, value) SELECT object_id, path, value FROM fields_v2;
		DROP TABLE fields_v2;
		CREATE INDEX fields_path_value ON fields(path, value);
	`)
	return err
}

func (s *sqliteEngine) Close() error { return s.db.Close() }

func (s *sqliteEngine) ProviderID() string       { return "sqlite" }
func (s *sqliteEngine) ProviderRevision() string { return "sqlite-v4-search-mvp" }
func (s *sqliteEngine) PhysicalDigest() kernel.Digest {
	return kernel.CanonicalDigest(map[string]any{"provider": s.ProviderID(), "revision": s.ProviderRevision()})
}

func (s *sqliteEngine) LoadMeta() (index.Meta, error) {
	meta := index.Meta{}
	rows, err := s.db.Query(`SELECT k, v FROM meta`)
	if err != nil {
		return meta, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return meta, err
		}
		switch k {
		case "basis":
			meta.Basis = kernel.CommitID(v)
		case "digest", "access_digest":
			meta.AccessDigest = kernel.Digest(v)
		case "physical_digest":
			meta.PhysicalDigest = kernel.Digest(v)
		case "provider_revision":
			meta.ProviderRevision = v
		case "generation":
			meta.Generation = v
		case "state":
			meta.State = v
		case "coverage":
			meta.Coverage, _ = strconv.ParseFloat(v, 64)
		case "mode":
			meta.Mode = v
		case "cause":
			meta.Cause = v
		}
	}
	return meta, rows.Err()
}

func (s *sqliteEngine) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM objects`).Scan(&n)
	return n, err
}

func (s *sqliteEngine) Probe(clause retrieval.SearchClause, spec retrieval.AccessSpec) index.Capability {
	if _, err := retrieval.ResolveSearchClause(clause, spec); err != nil {
		return index.Capability{Guarantee: index.GuaranteeUnsupported, Reason: err.Error()}
	}
	return index.Capability{Guarantee: index.GuaranteeExact, Coverage: 1}
}

func (s *sqliteEngine) Retrieve(req index.RetrieveRequest) (index.CandidatePage, error) {
	for _, clause := range req.Search.Clauses {
		if capability := s.Probe(clause, req.Spec); capability.Guarantee == index.GuaranteeUnsupported {
			return index.CandidatePage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "%s", capability.Reason)
		}
	}
	ids, err := searchIDs(s.db, req.Search, req.Spec)
	if err != nil {
		return index.CandidatePage{}, err
	}
	offset := 0
	if req.Continuation != "" {
		offset, err = strconv.Atoi(req.Continuation)
		if err != nil || offset < 0 || offset > len(ids) {
			return index.CandidatePage{}, kernel.Fail(kernel.ErrPreconditionFailed, "invalid sqlite continuation")
		}
	}
	limit := req.Search.Limit
	if limit <= 0 || offset+limit > len(ids) {
		limit = len(ids) - offset
	}
	meta, err := s.LoadMeta()
	if err != nil {
		return index.CandidatePage{}, err
	}
	page := index.CandidatePage{Exhausted: offset+limit >= len(ids)}
	for i, id := range ids[offset : offset+limit] {
		page.Candidates = append(page.Candidates, index.CandidateRef{
			ObjectID: id, Basis: meta.Basis,
			Evidence: []retrieval.LaneEvidence{{Provider: "sqlite", Lane: searchLane(req.Search), Guarantee: string(index.GuaranteeExact), LocalRank: offset + i + 1}},
		})
	}
	if !page.Exhausted {
		page.Continuation = strconv.Itoa(offset + limit)
	}
	return page, nil
}

func (s *sqliteEngine) Rebuild(docs []index.CompiledDoc, meta index.Meta) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM fts; DELETE FROM fields; DELETE FROM objects;`); err != nil {
		return err
	}
	for _, doc := range docs {
		if err := insertDoc(tx, doc); err != nil {
			return err
		}
	}
	if err := saveMeta(tx, meta); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *sqliteEngine) Apply(upserts []index.CompiledDoc, deletes []knowledge.ObjectID, meta index.Meta) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range deletes {
		if err := deleteRow(tx, id); err != nil {
			return err
		}
	}
	for _, doc := range upserts {
		if err := deleteRow(tx, doc.ObjectID); err != nil {
			return err
		}
		if err := insertDoc(tx, doc); err != nil {
			return err
		}
	}
	if err := saveMeta(tx, meta); err != nil {
		return err
	}
	return tx.Commit()
}

func insertDoc(tx *sql.Tx, doc index.CompiledDoc) error {
	if _, err := tx.Exec(`INSERT INTO objects(object_id) VALUES (?)`, string(doc.ObjectID)); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO fts(object_id, value_text) VALUES (?, ?)`, string(doc.ObjectID), doc.Text); err != nil {
		return err
	}
	for _, pair := range doc.Fields {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO fields(object_id, path, value) VALUES (?, ?, ?)`, string(doc.ObjectID), pair[0], pair[1]); err != nil {
			return err
		}
	}
	return nil
}

func deleteRow(tx *sql.Tx, id knowledge.ObjectID) error {
	if _, err := tx.Exec(`DELETE FROM fts WHERE object_id = ?`, string(id)); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM fields WHERE object_id = ?`, string(id)); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM objects WHERE object_id = ?`, string(id))
	return err
}

func saveMeta(tx *sql.Tx, meta index.Meta) error {
	pairs := [][2]string{
		{"basis", string(meta.Basis)},
		{"access_digest", string(meta.AccessDigest)},
		{"physical_digest", string(meta.PhysicalDigest)},
		{"provider_revision", meta.ProviderRevision},
		{"generation", meta.Generation},
		{"state", meta.State},
		{"coverage", strconv.FormatFloat(meta.Coverage, 'g', -1, 64)},
		{"mode", meta.Mode},
		{"cause", meta.Cause},
	}
	for _, pair := range pairs {
		if _, err := tx.Exec(`INSERT INTO meta(k, v) VALUES (?, ?) ON CONFLICT(k) DO UPDATE SET v = excluded.v`, pair[0], pair[1]); err != nil {
			return err
		}
	}
	return nil
}

func searchLane(req retrieval.SearchRequest) string {
	for _, clause := range req.Clauses {
		if clause.Op == retrieval.OpMatch {
			return "text"
		}
	}
	return "filter"
}
