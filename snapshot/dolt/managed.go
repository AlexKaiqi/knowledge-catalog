package dolt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"kc/kernel"
)

const managedMarker = ".kc-managed-allocation"

type managedOwnership struct {
	Repository kernel.RepositoryID `json:"repository"`
	Allocation string              `json:"allocation"`
}

// VerifyManaged is a read-only ownership check. A normal unstamped Dolt
// source, or a replacement directory, is never adopted as managed storage.
func VerifyManaged(root string, id kernel.RepositoryID, allocation string) error {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return kernel.Fail(kernel.ErrPreconditionFailed, "managed Dolt allocation directory is unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(root, managedMarker))
	var ownership managedOwnership
	if err != nil || json.Unmarshal(raw, &ownership) != nil || allocation == "" || ownership.Repository != id || ownership.Allocation != allocation {
		return kernel.Fail(kernel.ErrPreconditionFailed, "Dolt directory is not the expected managed allocation")
	}
	return nil
}

// CreateManaged initializes only an exclusively allocated directory. Retry
// requires the existing ownership marker; ordinary external directories fail.
func CreateManaged(root string, id kernel.RepositoryID, allocation string) (*DoltRepository, error) {
	if strings.TrimSpace(string(id)) == "" || strings.TrimSpace(allocation) == "" {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "repository and managed allocation identity are required")
	}
	if err := os.MkdirAll(filepath.Dir(root), 0700); err != nil {
		return nil, err
	}
	err := os.Mkdir(root, 0700)
	if err == nil {
		body, _ := json.Marshal(managedOwnership{Repository: id, Allocation: allocation})
		marker, writeErr := os.OpenFile(filepath.Join(root, managedMarker), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if writeErr != nil {
			_ = os.Remove(root)
			return nil, writeErr
		}
		_, writeErr = marker.Write(body)
		if writeErr == nil {
			writeErr = marker.Sync()
		}
		closeErr := marker.Close()
		if writeErr != nil {
			return nil, writeErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		for _, path := range []string{root, filepath.Dir(root)} {
			dir, syncErr := os.Open(path)
			if syncErr != nil {
				return nil, syncErr
			}
			syncErr = dir.Sync()
			_ = dir.Close()
			if syncErr != nil {
				return nil, syncErr
			}
		}
	} else if !os.IsExist(err) {
		return nil, err
	}
	if err := VerifyManaged(root, id, allocation); err != nil {
		return nil, err
	}
	repo, err := OpenDolt(root, id)
	if err != nil {
		return nil, err
	}
	repo.managedAllocation = allocation
	// A crash may occur after `dolt init` but before the raw table commit.
	// Only this owned-create path may resume that substrate initialization.
	if _, err := repo.EnsureNativeSchema([]string{"kc_files"}, []string{"CREATE TABLE IF NOT EXISTS kc_files (path VARCHAR(1024) PRIMARY KEY, content LONGBLOB NOT NULL)"}); err != nil {
		return nil, err
	}
	return repo, nil
}

// OpenManaged retains the ownership check on every engine invocation,
// including NativeQuery and native writes, without affecting external sources.
func OpenManaged(root string, id kernel.RepositoryID, allocation string) (*DoltRepository, error) {
	if err := VerifyManaged(root, id, allocation); err != nil {
		return nil, err
	}
	repo, err := OpenExisting(root, id)
	if err != nil {
		return nil, err
	}
	repo.managedAllocation = allocation
	return repo, nil
}
