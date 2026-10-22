package projection

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
	ports "kc/discovery/projection"
	"kc/kernel"
	objectmodel "kc/object"
)

var stateDescriptorBucket = []byte("observation-descriptor-v1")
var stateDescriptorKey = []byte("identity")

// The descriptor selects values only after the provider has independently
// confirmed this exact observation. Its presence is never publication evidence.
type stateDescriptor struct {
	Version     int                 `json:"version"`
	Repository  kernel.RepositoryID `json:"repository"`
	Authority   kernel.RepositoryID `json:"authority"`
	Basis       kernel.CommitID     `json:"basis"`
	Revision    string              `json:"revision"`
	Access      kernel.Digest       `json:"access"`
	Observation kernel.Digest       `json:"observation"`
}

func stateServingDirectory(root string, key engineKey) string {
	identity := kernel.CanonicalDigest(map[string]any{"repository": key.repo, "commit": key.commit, "lane": key.lane})
	return filepath.Join(root, "state-serving", SanitizeID(string(key.repo))+"-"+string(identity)[:16])
}

func observationFile(root string, key engineKey, authority kernel.RepositoryID, revision string) string {
	dir := stateServingDirectory(root, key)
	name := kernel.CanonicalDigest(map[string]any{"authority": authority, "revision": revision})
	return filepath.Join(dir, string(name)+".db")
}

// seal makes values immutable and durable before any provider publication can
// begin. Concurrent publishers may reuse identical sealed values; they cannot
// replace an existing file or rewrite its descriptor.
func (s *stateServingStore) seal(key engineKey, authority kernel.RepositoryID, state *stateProjection) error {
	descriptor := stateDescriptor{1, key.repo, authority, key.commit, state.revision, state.accessDigest, state.observationDigest}
	raw, err := json.Marshal(descriptor)
	if err != nil {
		return err
	}
	if err = s.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucket(stateDescriptorBucket)
		if err != nil {
			return err
		}
		return bucket.Put(stateDescriptorKey, raw)
	}); err != nil {
		return err
	}
	if err = s.Close(); err != nil {
		return err
	}
	root := filepath.Dir(filepath.Dir(s.dir))
	file := observationFile(root, key, authority, state.revision)
	if err = os.Link(s.file, file); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	db, got, err := readObservationFile(context.Background(), file)
	if err != nil {
		return err
	}
	checkStore := &stateServingStore{db: db}
	digest, digestErr := checkStore.Digest(key.commit)
	if digestErr != nil {
		_ = db.Close()
		return digestErr
	}
	if digest != descriptor.Observation || !reflect.DeepEqual(got, descriptor) {
		_ = db.Close()
		return kernel.Fail(kernel.ErrPreconditionFailed, "immutable observation identity collision")
	}
	// Sync each directory entry created for the new immutable value file.
	for _, path := range []string{s.dir, filepath.Dir(s.dir), root} {
		dir, err := os.Open(path)
		if err == nil {
			err = dir.Sync()
			_ = dir.Close()
		}
		if err != nil {
			_ = db.Close()
			return err
		}
	}
	_ = os.Remove(s.file)
	s.file, s.db, s.sealed = file, db, true
	s.closeOnce = sync.Once{}
	s.closeErr = nil
	return nil
}

func readObservationFile(ctx context.Context, file string) (*bolt.DB, stateDescriptor, error) {
	var descriptor stateDescriptor
	// Read-only opens never create a missing candidate or descriptor bucket.
	if err := ctx.Err(); err != nil {
		return nil, descriptor, err
	}
	timeout := time.Second
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < timeout {
		timeout = time.Until(deadline)
		if timeout <= 0 {
			return nil, descriptor, context.DeadlineExceeded
		}
	}
	db, err := bolt.Open(file, 0o600, &bolt.Options{ReadOnly: true, Timeout: timeout})
	if err != nil {
		if canceled := ctx.Err(); canceled != nil {
			return nil, descriptor, canceled
		}
		if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
			return nil, descriptor, context.DeadlineExceeded
		}
		return nil, descriptor, err
	}
	err = db.View(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		bucket := tx.Bucket(stateDescriptorBucket)
		if bucket == nil || tx.Bucket(stateValueBucket) == nil {
			return kernel.Fail(kernel.ErrPreconditionFailed, "observation descriptor is missing")
		}
		return json.Unmarshal(bucket.Get(stateDescriptorKey), &descriptor)
	})
	if err != nil {
		_ = db.Close()
		return nil, descriptor, err
	}
	return db, descriptor, nil
}

// recoverState checks the physical publication before opening its exact values.
// It does not adopt orphan candidates, refresh runtimes, or fall back to HEAD.
// Caller holds stateMu exclusively; live readers retain their existing handle.
func (idx *Index) recoverState(ctx context.Context, key engineKey, authority kernel.RepositoryID, eng ports.Engine) error {
	if idx.states[key] != nil || idx.stateDirectory == "" {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	meta, err := loadMetaContext(ctx, eng)
	if err != nil {
		return err
	}
	if meta.State != ports.ProjectionStateReady || meta.Basis != key.commit || meta.Revision == "" || meta.ObservationDigest == "" {
		return nil
	}
	// Physical incompatibility is repairable readiness, not corrupt values.
	if !idx.physicalMatches(eng, meta) {
		return nil
	}
	state, err := idx.openObservationState(ctx, key, authority, eng, meta)
	if err != nil {
		return err
	}
	if state != nil {
		idx.states[key] = state
	}
	return nil
}

// Caller holds stateMu while using the selected storage directory.
func (idx *Index) openObservationState(ctx context.Context, key engineKey, authority kernel.RepositoryID, eng ports.Engine, meta ports.Meta) (*stateProjection, error) {
	if idx.stateDirectory == "" {
		return nil, nil
	}
	file := observationFile(idx.stateDirectory, key, authority, meta.Revision)
	db, descriptor, err := readObservationFile(ctx, file)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	store := &stateServingStore{db: db, file: file, dir: filepath.Dir(file), sealed: true}
	state := &stateProjection{authority: descriptor.Authority, revision: descriptor.Revision, accessDigest: descriptor.Access, observationDigest: descriptor.Observation, store: store}
	if descriptor.Version != 1 || descriptor.Repository != key.repo || descriptor.Authority != authority || descriptor.Basis != key.commit || !idx.stateProjectionMatches(eng, meta, key.commit, state) {
		_ = store.Close()
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "retained observation does not match physical publication")
	}
	digest, err := store.DigestContext(ctx, key.commit)
	if err != nil || digest != state.observationDigest {
		_ = store.Close()
		if err != nil {
			return nil, err
		}
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "retained observation values failed integrity validation")
	}
	if err := ctx.Err(); err != nil {
		_ = store.Close()
		return nil, err
	}
	// Displayed age is derived from the same authenticated observation rows,
	// never trusted as independently persisted metadata.
	age := observationTime{}
	if err := store.WalkRecords(500, func(records map[objectmodel.ObjectID]stateRecord) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, record := range records {
			age.include(record.Observations)
		}
		return nil
	}); err != nil {
		_ = store.Close()
		return nil, err
	}
	state.oldestObservedAt = age.raw
	return state, nil
}

func (idx *Index) restoreState(ctx context.Context, key engineKey, authority kernel.RepositoryID) error {
	idx.stateMu.Lock()
	defer idx.stateMu.Unlock()
	if state := idx.states[key]; state != nil {
		if state.authority != authority {
			return kernel.Fail(kernel.ErrPreconditionFailed, "resident observation belongs to another source binding")
		}
		return nil
	}
	if idx.stateDirectory == "" {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Dir(observationFile(idx.stateDirectory, key, authority, ""))); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	eng, err := idx.stateEngineAt(authority, key.commit)
	if err != nil {
		return err
	}
	return idx.recoverState(ctx, key, authority, eng)
}

// Allocate anonymous observation storage lazily. It belongs to this Index, not
// one replaced generation, so valid continuations retain their original values.
func (idx *Index) openObservationStore(key engineKey) (*stateServingStore, error) {
	idx.stateMu.Lock()
	defer idx.stateMu.Unlock()
	if idx.stateDirectory == "" {
		directory, err := os.MkdirTemp("", "kc-index-observations-")
		if err != nil {
			return nil, err
		}
		idx.stateDirectory, idx.temporaryStateDirectory = directory, true
	}
	return openStateServingStore(idx.stateDirectory, key, "building")
}
