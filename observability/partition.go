package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"kc/internal/jsonfile"
	"kc/kernel"
)

// Date-partitioned JSON Lines plus ILM-style hot/delete. This is the same
// layout log ships, Hive/S3 dt= partitions, and Elasticsearch ILM use:
// one append-only file per UTC day, drop closed days after the hot window,
// fail closed at a byte cap and disk flood-stage watermark. No extra log library.
const (
	DefaultHotRetention = 30 * 24 * time.Hour
	MinHotRetention     = 24 * time.Hour
	MaxHotRetention     = 180 * 24 * time.Hour
	DefaultMaxBytes     = int64(1) << 30
	DefaultFloodStage   = 0.95
	DefaultRetention    = "30d"
	partitionLayout     = "2006-01-02"
	policyFileName      = "evidence-policy.json"
)

const (
	StreamAccess    = "access"
	StreamFeedback  = "feedback"
	StreamRetrieval = "retrieval"
	StreamRefine    = "refine"
)

var EvidenceStreams = []string{StreamAccess, StreamFeedback, StreamRetrieval, StreamRefine}

var partitionFileName = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.jsonl$`)

type FileStorePolicy struct {
	HotRetention string `json:"hotRetention,omitempty"`
	MaxBytes     int64  `json:"maxBytes,omitempty"`
}

func DefaultFileStorePolicy() FileStorePolicy {
	return FileStorePolicy{HotRetention: DefaultRetention, MaxBytes: DefaultMaxBytes}
}

func ParseRetention(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	if strings.HasSuffix(value, "d") {
		days := strings.TrimSuffix(value, "d")
		if days != "" && !strings.ContainsAny(days, "hmsu") {
			n, err := strconv.Atoi(days)
			if err != nil || n < 0 {
				return 0, kernel.Fail(kernel.ErrUsageInvalid, "evidence hotRetention is invalid")
			}
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, kernel.Fail(kernel.ErrUsageInvalid, "evidence hotRetention is invalid")
	}
	return d, nil
}

func ValidateFileStorePolicy(policy FileStorePolicy) error {
	d, err := ParseRetention(policy.HotRetention)
	if err != nil {
		return err
	}
	if d != 0 && (d < MinHotRetention || d > MaxHotRetention) {
		return kernel.Fail(kernel.ErrUsageInvalid, "evidence hotRetention must be between 24h and 180d")
	}
	if policy.MaxBytes < 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "evidence maxBytes must be non-negative")
	}
	return nil
}

func WriteFileStorePolicy(home string, policy FileStorePolicy) error {
	if policy.HotRetention == "" {
		policy.HotRetention = DefaultRetention
	}
	if policy.MaxBytes == 0 {
		policy.MaxBytes = DefaultMaxBytes
	}
	if err := ValidateFileStorePolicy(policy); err != nil {
		return err
	}
	return jsonfile.Write(filepath.Join(home, policyFileName), policy)
}

func (s *FileStore) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *FileStore) hotRetention() time.Duration {
	if s.HotRetention > 0 {
		return s.HotRetention
	}
	return DefaultHotRetention
}

func (s *FileStore) maxBytes() int64 {
	if s.MaxBytes > 0 {
		return s.MaxBytes
	}
	return DefaultMaxBytes
}

func (s *FileStore) floodStage() float64 {
	if s.FloodStage > 0 {
		return s.FloodStage
	}
	return DefaultFloodStage
}

func (s *FileStore) policyHome() string {
	if s.Home != "" {
		return s.Home
	}
	if s.AccessPath != "" {
		return filepath.Dir(s.AccessPath)
	}
	return ""
}

func (s *FileStore) loadPolicy() {
	home := s.policyHome()
	if home == "" {
		return
	}
	var policy FileStorePolicy
	if err := jsonfile.Read(filepath.Join(home, policyFileName), &policy); err != nil {
		return
	}
	if s.HotRetention == 0 && policy.HotRetention != "" {
		if d, err := ParseRetention(policy.HotRetention); err == nil && d > 0 {
			s.HotRetention = d
		}
	}
	if s.MaxBytes == 0 && policy.MaxBytes > 0 {
		s.MaxBytes = policy.MaxBytes
	}
}

func (s *FileStore) legacyPath(kind string) string {
	switch kind {
	case StreamAccess:
		if s.AccessPath != "" {
			return s.AccessPath
		}
	case StreamFeedback:
		if s.FeedbackPath != "" {
			return s.FeedbackPath
		}
	case StreamRetrieval:
		if s.RetrievalPath != "" {
			return s.RetrievalPath
		}
	case StreamRefine:
		if s.RefinePath != "" {
			return s.RefinePath
		}
	}
	if home := s.policyHome(); home != "" {
		return filepath.Join(home, kind+".jsonl")
	}
	return ""
}

func (s *FileStore) streamDir(kind string) string {
	if legacy := s.legacyPath(kind); legacy != "" {
		return filepath.Join(filepath.Dir(legacy), kind)
	}
	return ""
}

func evidenceUnavailable(err error) error {
	if err == nil {
		return nil
	}
	if kernel.CodeOf(err) != "" {
		return err
	}
	return kernel.Fail(kernel.ErrTemporaryUnavailable, "evidence store is unavailable")
}

func (s *FileStore) appendEvent(kind, occurredAt string, value any) error {
	if err := s.prepareWrite(kind); err != nil {
		return err
	}
	path, err := s.partitionFile(kind, occurredAt)
	if err != nil {
		return evidenceUnavailable(err)
	}
	body, err := json.Marshal(value)
	if err != nil {
		return evidenceUnavailable(err)
	}
	if err := jsonfile.AppendJSONL(path, value); err != nil {
		return evidenceUnavailable(err)
	}
	s.lastAppendBytes = int64(len(body) + 1)
	return nil
}

func (s *FileStore) prepareWrite(kind string) error {
	if err := s.migrateLegacy(kind); err != nil {
		return evidenceUnavailable(err)
	}
	if err := s.Prune(); err != nil {
		return err
	}
	if err := s.enforceFloodStage(); err != nil {
		return err
	}
	return s.enforceQuota(kind)
}

func (s *FileStore) partitionFile(kind, occurredAt string) (string, error) {
	day, err := partitionDay(occurredAt, s.now())
	if err != nil {
		return "", kernel.Fail(kernel.ErrUsageInvalid, "evidence occurredAt is invalid")
	}
	dir := s.streamDir(kind)
	if dir == "" {
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "evidence store is unavailable")
	}
	return filepath.Join(dir, day.Format(partitionLayout)+".jsonl"), nil
}

func partitionDay(occurredAt string, now time.Time) (time.Time, error) {
	t := now.UTC()
	if occurredAt != "" {
		parsed, err := parseOccurredAt(occurredAt)
		if err != nil {
			return time.Time{}, err
		}
		t = parsed.UTC()
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
}

func (s *FileStore) evidenceFiles(kind, since, until string) ([]string, error) {
	var paths []string
	legacy := s.legacyPath(kind)
	if legacy != "" {
		info, err := os.Stat(legacy)
		if err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			paths = append(paths, legacy)
		} else if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	dir := s.streamDir(kind)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return paths, nil
		}
		return nil, err
	}
	startDay, endDay, err := partitionRange(since, until)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !partitionFileName.MatchString(name) {
			continue
		}
		day, err := time.ParseInLocation(partitionLayout, strings.TrimSuffix(name, ".jsonl"), time.UTC)
		if err != nil {
			continue
		}
		if !startDay.IsZero() && day.Before(startDay) {
			continue
		}
		if !endDay.IsZero() && day.After(endDay) {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	sort.Strings(paths)
	return paths, nil
}

func partitionRange(since, until string) (time.Time, time.Time, error) {
	var startDay, endDay time.Time
	if since != "" {
		start, err := parseOccurredAt(since)
		if err != nil {
			return time.Time{}, time.Time{}, kernel.Fail(kernel.ErrUsageInvalid, "access since is invalid")
		}
		start = start.UTC()
		startDay = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	}
	if until != "" {
		end, err := parseOccurredAt(until)
		if err != nil {
			return time.Time{}, time.Time{}, kernel.Fail(kernel.ErrUsageInvalid, "access until is invalid")
		}
		end = end.UTC()
		endDay = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	}
	return startDay, endDay, nil
}

func readRetained[T any](s *FileStore, kind string) ([]T, error) {
	if err := s.migrateLegacy(kind); err != nil {
		return nil, evidenceUnavailable(err)
	}
	if err := s.Prune(); err != nil {
		return nil, err
	}
	paths, err := s.evidenceFiles(kind, "", "")
	if err != nil {
		return nil, evidenceUnavailable(err)
	}
	return readJSONLFiles[T](paths)
}

func readWindow[T any](s *FileStore, kind, since, until string) ([]T, error) {
	if err := s.migrateLegacy(kind); err != nil {
		return nil, evidenceUnavailable(err)
	}
	if err := s.Prune(); err != nil {
		return nil, err
	}
	paths, err := s.evidenceFiles(kind, since, until)
	if err != nil {
		return nil, evidenceUnavailable(err)
	}
	return readJSONLFiles[T](paths)
}

func (s *FileStore) hotSince() string {
	return s.now().Add(-s.hotRetention()).Format(time.RFC3339Nano)
}

func (s *FileStore) applyHotWindow(query AccessQuery) AccessQuery {
	if query.Since == "" {
		query.Since = s.hotSince()
	}
	return query
}

// Prune drops closed UTC day files older than the hot window (ILM delete phase).
// The empty legacy `*.jsonl` markers are left in place for deployment volume checks.
func (s *FileStore) Prune() error {
	cutoff := s.now().Add(-s.hotRetention()).UTC()
	cutoffDay := time.Date(cutoff.Year(), cutoff.Month(), cutoff.Day(), 0, 0, 0, 0, time.UTC)
	for _, kind := range EvidenceStreams {
		dir := s.streamDir(kind)
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return evidenceUnavailable(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !partitionFileName.MatchString(name) {
				continue
			}
			day, err := time.ParseInLocation(partitionLayout, strings.TrimSuffix(name, ".jsonl"), time.UTC)
			if err != nil {
				continue
			}
			if !day.Before(cutoffDay) {
				continue
			}
			if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
				return evidenceUnavailable(err)
			}
		}
	}
	return nil
}

func (s *FileStore) migrateLegacy(kind string) error {
	path := s.legacyPath(kind)
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return nil
	}
	records, err := readJSONL[json.RawMessage](path)
	if err != nil {
		return err
	}
	for _, raw := range records {
		var envelope struct {
			OccurredAt string `json:"occurredAt"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return err
		}
		target, err := s.partitionFile(kind, envelope.OccurredAt)
		if err != nil {
			return err
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		if err := jsonfile.AppendJSONL(target, value); err != nil {
			return err
		}
	}
	return os.Truncate(path, 0)
}

func (s *FileStore) enforceQuota(kind string) error {
	size, err := s.streamBytes(kind)
	if err != nil {
		return evidenceUnavailable(err)
	}
	if size >= s.maxBytes() {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "evidence store %s exceeded its byte cap", kind)
	}
	return nil
}

func (s *FileStore) streamBytes(kind string) (int64, error) {
	var total int64
	add := func(path string) error {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	}
	if err := add(s.legacyPath(kind)); err != nil {
		return 0, err
	}
	dir := s.streamDir(kind)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return total, nil
	}
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return 0, err
		}
		total += info.Size()
	}
	return total, nil
}

func (s *FileStore) enforceFloodStage() error {
	used, ok := s.diskUsedFraction()
	if !ok {
		return nil
	}
	if used >= s.floodStage() {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "evidence store disk is above the flood-stage watermark")
	}
	return nil
}

func (s *FileStore) diskUsedFraction() (float64, bool) {
	if s.FloodUsedFraction != nil {
		return s.FloodUsedFraction(), true
	}
	home := s.policyHome()
	if home == "" {
		return 0, false
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(home, &st); err != nil || st.Blocks == 0 {
		return 0, false
	}
	return float64(st.Blocks-st.Bavail) / float64(st.Blocks), true
}
