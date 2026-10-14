package evidence

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"kc/internal/jsonfile"
	"kc/kernel"
)

const expiryLayout = "20060102T150405.000000000Z"

var _ Store = (*FileStore)(nil)

// FileStore retains results independently of the pointer-only access ledger.
// The host supplies its existing retention, capacity and flood-stage policy.
type FileStore struct {
	directory         string
	retention         time.Duration
	MaxBytes          int64
	FloodStage        float64
	FloodUsedFraction func() float64
	Now               func() time.Time
}

func NewFileStore(directory string, retention time.Duration) (*FileStore, error) {
	if strings.TrimSpace(directory) == "" || retention <= 0 {
		return nil, kernel.Fail(kernel.ErrUsageInvalid, "retained results require a directory and explicit positive retention")
	}
	return &FileStore{directory: directory, retention: retention}, nil
}

type diskRecord struct {
	ID          string `json:"id"`
	OccurredAt  string `json:"occurredAt"`
	RetainUntil string `json:"retainUntil"`
	Record      Record `json:"record"`
}

func (s *FileStore) Record(ctx context.Context, record Record) (Reference, error) {
	if err := ctx.Err(); err != nil {
		return Reference{}, err
	}
	if err := record.Validate(); err != nil {
		return Reference{}, err
	}
	if s.MaxBytes <= 0 || (s.FloodUsedFraction != nil && (s.FloodStage <= 0 || s.FloodStage > 1)) {
		return Reference{}, kernel.Fail(kernel.ErrUsageInvalid, "retained results require an explicit valid capacity policy")
	}
	stored, raw, reference, err := s.prepare(record)
	if err != nil {
		return Reference{}, err
	}
	err = s.withLock(ctx, func() error {
		if err := s.pruneExpired(ctx); err != nil {
			return err
		}
		if err := s.checkCapacity(int64(len(raw))); err != nil {
			return err
		}
		return jsonfile.Write(s.path(reference.ID), stored)
	})
	if err != nil {
		return Reference{}, unavailable("retain adopted results", err)
	}
	return reference, nil
}

func (s *FileStore) prepare(record Record) (diskRecord, []byte, Reference, error) {
	// A private JSON copy also preserves exact numeric values across restart.
	input, err := json.Marshal(record)
	if err != nil {
		return diskRecord{}, nil, Reference{}, err
	}
	var frozen Record
	if err := kernel.UnmarshalJSON(input, &frozen); err != nil {
		return diskRecord{}, nil, Reference{}, err
	}
	at := s.now()
	until := at.Add(s.retention)
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return diskRecord{}, nil, Reference{}, unavailable("allocate retained result identity", err)
	}
	stored := diskRecord{ID: "la_" + until.Format(expiryLayout) + "_" + hex.EncodeToString(nonce),
		OccurredAt: at.Format(time.RFC3339Nano), RetainUntil: until.Format(time.RFC3339Nano), Record: frozen}
	body, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return diskRecord{}, nil, Reference{}, err
	}
	raw := append(body, '\n')
	ref := Reference{ID: stored.ID, Digest: digest(raw), OccurredAt: stored.OccurredAt, RetainUntil: stored.RetainUntil}
	return stored, raw, ref, nil
}

func (s *FileStore) Read(ctx context.Context, reference Reference) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	until, err := validateReference(reference)
	if err != nil {
		return Record{}, unavailable("invalid retained result reference", err)
	}
	if !s.now().Before(until) {
		return Record{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "retained result has reached its retention deadline")
	}
	raw, err := os.ReadFile(s.path(reference.ID))
	if err != nil {
		return Record{}, unavailable("retained result is unavailable within its retention period", err)
	}
	if digest(raw) != reference.Digest {
		return Record{}, unavailable("retained result content does not match its original digest", nil)
	}
	var stored diskRecord
	if err := kernel.UnmarshalJSON(raw, &stored); err != nil {
		return Record{}, unavailable("retained result content is invalid", err)
	}
	if stored.ID != reference.ID || stored.OccurredAt != reference.OccurredAt || stored.RetainUntil != reference.RetainUntil {
		return Record{}, unavailable("retained result metadata does not match its reference", nil)
	}
	if err := stored.Record.Validate(); err != nil {
		return Record{}, unavailable("retained result is incomplete", err)
	}
	return stored.Record, nil
}

func (s *FileStore) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *FileStore) path(id string) string { return filepath.Join(s.directory, id+".json") }

func digest(raw []byte) kernel.Digest {
	hash := sha256.Sum256(raw)
	return kernel.Digest(hex.EncodeToString(hash[:]))
}

func expiryFromID(id string) (time.Time, error) {
	parts := strings.Split(id, "_")
	if len(parts) != 3 || parts[0] != "la" || len(parts[2]) != 32 {
		return time.Time{}, fmt.Errorf("invalid retained result identity")
	}
	if _, err := hex.DecodeString(parts[2]); err != nil {
		return time.Time{}, err
	}
	return time.Parse(expiryLayout, parts[1])
}

func validateReference(reference Reference) (time.Time, error) {
	until, err := expiryFromID(reference.ID)
	if err != nil {
		return time.Time{}, err
	}
	declared, err := time.Parse(time.RFC3339Nano, reference.RetainUntil)
	if err != nil || !until.Equal(declared) {
		return time.Time{}, fmt.Errorf("retention deadline does not match retained result identity")
	}
	at, err := time.Parse(time.RFC3339Nano, reference.OccurredAt)
	if err != nil || !at.Before(until) || len(reference.Digest) != 64 {
		return time.Time{}, fmt.Errorf("invalid retained result reference metadata")
	}
	if _, err := hex.DecodeString(string(reference.Digest)); err != nil {
		return time.Time{}, err
	}
	return until, nil
}

func unavailable(message string, err error) error {
	if err == nil {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "%s", message)
	}
	return kernel.Fail(kernel.ErrTemporaryUnavailable, "%s: %v", message, err)
}

func (s *FileStore) withLock(ctx context.Context, operation func() error) error {
	if err := os.MkdirAll(s.directory, 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(s.directory, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if err := ctx.Err(); err != nil {
		return err
	}
	return operation()
}
