package dolt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"kc/kernel"
)

// OpenDolt opens or initializes a native Dolt database. KC_DOLT_BIN may name
// a dolt executable. When none is installed, Docker is used so the reference
// implementation can still exercise a real Dolt engine in a clean room.
func OpenDolt(rootDir string, id kernel.RepositoryID) (*DoltRepository, error) {
	abs, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	value, _ := doltRootLocks.LoadOrStore(abs, &sync.Mutex{})
	repo := &DoltRepository{repositoryID: id, rootDir: abs, lock: value.(*sync.Mutex)}
	repo.lock.Lock()
	defer repo.lock.Unlock()
	if err := repo.ensure(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *DoltRepository) ID() kernel.RepositoryID { return r.repositoryID }

// ReadDoltStamp identifies a native Dolt repository during home discovery.
func ReadDoltStamp(rootDir string) (kernel.RepositoryID, error) {
	if _, err := os.Stat(filepath.Join(rootDir, ".dolt")); err != nil {
		return "", err
	}
	raw, err := os.ReadFile(filepath.Join(rootDir, doltStamp))
	if err != nil {
		return "", err
	}
	id := kernel.RepositoryID(strings.TrimSpace(string(raw)))
	if id == "" {
		return "", fmt.Errorf("empty Dolt repository stamp in %s", rootDir)
	}
	return id, nil
}

func (r *DoltRepository) ensure() error {
	stampPath := filepath.Join(r.rootDir, doltStamp)
	if raw, err := os.ReadFile(stampPath); err == nil {
		if strings.TrimSpace(string(raw)) != string(r.repositoryID) {
			return fmt.Errorf("Dolt database %s is stamped as %s, not %s", r.rootDir, strings.TrimSpace(string(raw)), r.repositoryID)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(filepath.Join(r.rootDir, ".dolt")); os.IsNotExist(err) {
		if _, err := r.run("init", "--name", "knowledge-catalog", "--email", "kc@local"); err != nil {
			return err
		}
		if _, err := r.run("sql", "-q", "CREATE TABLE kc_files (path VARCHAR(1024) PRIMARY KEY, content LONGBLOB NOT NULL)"); err != nil {
			return err
		}
		if _, err := r.run("add", "."); err != nil {
			return err
		}
		if _, err := r.run("commit", "-m", "root"); err != nil {
			return err
		}
	}
	if err := os.WriteFile(stampPath, []byte(string(r.repositoryID)+"\n"), 0o600); err != nil {
		return err
	}
	if _, err := r.queryHash("main"); err != nil {
		return err
	}
	rows, err := r.query("SELECT hash FROM dolt_branches WHERE name=" + sqlString("kc-archived"))
	if err != nil {
		return err
	}
	r.archived = len(rows) == 1
	return nil
}
