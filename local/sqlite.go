package local

import (
	"database/sql"
	"os"
	"path/filepath"

	"kc/index"
	"kc/kernel"
	"kc/reader"

	_ "modernc.org/sqlite"
)

type sqliteEngine struct {
	db *sql.DB
}

func OpenSQLite(dir string, id kernel.RepositoryID) (index.Engine, error) {
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
			PRIMARY KEY (object_id, path)
		);
		CREATE INDEX IF NOT EXISTS fields_path_value ON fields(path, value);
		CREATE VIRTUAL TABLE IF NOT EXISTS fts USING fts5(object_id UNINDEXED, value_text);
	`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &sqliteEngine{db: db}, nil
}

func (s *sqliteEngine) Close() error { return s.db.Close() }

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
		case "digest":
			meta.Digest = kernel.Digest(v)
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

func (s *sqliteEngine) Search(req reader.SearchRequest, spec reader.IndexSpec) ([]kernel.ObjectID, error) {
	return searchIDs(s.db, req, spec)
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

func (s *sqliteEngine) Apply(upserts []index.CompiledDoc, deletes []kernel.ObjectID, meta index.Meta) error {
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
		if _, err := tx.Exec(`INSERT INTO fields(object_id, path, value) VALUES (?, ?, ?)`, string(doc.ObjectID), pair[0], pair[1]); err != nil {
			return err
		}
	}
	return nil
}

func deleteRow(tx *sql.Tx, id kernel.ObjectID) error {
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
		{"digest", string(meta.Digest)},
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
