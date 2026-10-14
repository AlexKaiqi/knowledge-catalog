package evidence

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"kc/kernel"
)

func (s *FileStore) checkCapacity(encodedSize int64) error {
	if s.FloodUsedFraction != nil && s.FloodUsedFraction() >= s.FloodStage {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "retained result storage reached flood stage")
	}
	var used int64
	if err := filepath.WalkDir(s.directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > s.MaxBytes-used {
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "retained result storage capacity is exhausted")
		}
		used += info.Size()
		return nil
	}); err != nil {
		return err
	}
	if encodedSize > s.MaxBytes || used > s.MaxBytes-encodedSize {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "retained result storage capacity is exhausted")
	}
	return nil
}

// Prune only removes records whose recorder-managed identity proves their
// deadline has passed. Corrupt payloads cannot bring that deadline forward.
func (s *FileStore) Prune(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := s.withLock(ctx, func() error { return s.pruneExpired(ctx) })
	if err != nil {
		return unavailable("prune expired retained results", err)
	}
	return nil
}

func (s *FileStore) pruneExpired(ctx context.Context) error {
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return err
	}
	now, removed := s.now(), false
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		until, err := expiryFromID(strings.TrimSuffix(entry.Name(), ".json"))
		if err == nil && !now.Before(until) {
			if err := os.Remove(filepath.Join(s.directory, entry.Name())); err != nil {
				return err
			}
			removed = true
		}
	}
	if !removed {
		return nil
	}
	directory, err := os.Open(s.directory)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
