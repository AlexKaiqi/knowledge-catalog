package integrationruntime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
	"kc/connector"
	"kc/kernel"
	"kc/snapshot"
)

type Runtime struct {
	Directory string
	Factory   WriterFactory
}

func New(directory string, factory WriterFactory) (*Runtime, error) {
	if err := initialize(directory); err != nil {
		return nil, err
	}
	return &Runtime{Directory: directory, Factory: factory}, nil
}

func (r *Runtime) writer(ctx context.Context, server string) (Writer, string, error) {
	if r.Factory == nil {
		return nil, "", kernel.Fail(kernel.ErrUnauthenticated, "KC login is required")
	}
	writer, owner, err := r.Factory(ctx, server)
	if err != nil {
		return nil, "", err
	}
	if writer == nil || strings.TrimSpace(owner) == "" {
		return nil, "", kernel.Fail(kernel.ErrUnauthenticated, "KC identity was not verified")
	}
	return writer, owner, nil
}

func (r *Runtime) Activate(ctx context.Context, manifest Manifest) (Status, error) {
	if strings.TrimSpace(manifest.ID) == "" || manifest.ArtifactID == "" || manifest.Server == "" || manifest.Repository == "" || manifest.IntervalSeconds < 1 || manifest.IntervalSeconds > 86400 || manifest.TimeoutSeconds < 0 || manifest.TimeoutSeconds > 600 {
		return Status{}, kernel.Fail(kernel.ErrUsageInvalid, "integration requires id, artifact, server, repository and a 1..86400 second interval")
	}
	if manifest.Mode != connector.ModePatch && manifest.Mode != connector.ModeReconcile {
		return Status{}, kernel.Fail(kernel.ErrUsageInvalid, "integration mode must be patch or reconcile")
	}
	if err := manifest.Scope.Validate(); err != nil {
		return Status{}, err
	}
	manifest.Ref = snapshot.RefOrDefault(manifest.Ref)
	if manifest.TimeoutSeconds == 0 {
		manifest.TimeoutSeconds = 30
	}
	artifact, err := r.artifact(manifest.ArtifactID)
	if err != nil {
		return Status{}, err
	}
	writer, owner, err := r.writer(ctx, manifest.Server)
	if err != nil {
		return Status{}, err
	}
	if _, err := writer.Head(ctx, manifest.Repository, manifest.Ref); err != nil {
		return Status{}, err
	}
	err = r.transaction(true, func(tx *bolt.Tx) error {
		bucket := tx.Bucket(statesBucket)
		value := state{Manifest: manifest, Artifact: artifact, Owner: owner, Active: true, Phase: "ACTIVE", NextRunAt: time.Now()}
		if raw := bucket.Get([]byte(manifest.ID)); raw != nil {
			var previous state
			if err := decode(raw, &previous); err != nil {
				return err
			}
			if previous.Owner != owner || previous.Manifest.Repository != manifest.Repository || previous.Manifest.Server != manifest.Server || previous.Manifest.Ref != manifest.Ref {
				return kernel.Fail(kernel.ErrTargetRepositoryDenied, "an integration cannot change its owner or target; activate a new integration id")
			}
			if previous.Pending != nil || previous.LeaseID != "" && time.Now().Before(previous.LeaseUntil) {
				return kernel.Fail(kernel.ErrPreconditionFailed, "recover the pending integration run before reactivation")
			}
			value.Checkpoint = previous.Checkpoint
			value.Runs = previous.Runs
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(manifest.ID), raw)
	})
	if err != nil {
		return Status{}, err
	}
	return r.Status(manifest.ID)
}

func (r *Runtime) Status(id string) (Status, error) {
	value, err := r.load(id)
	if err != nil {
		return Status{}, err
	}
	return statusOf(value), nil
}
func statusOf(value state) Status {
	result := Status{Manifest: value.Manifest, Owner: value.Owner, Active: value.Active, Phase: value.Phase, Checkpoint: value.Checkpoint, LastError: value.LastError, LastRunAt: value.LastRunAt, NextRunAt: value.NextRunAt, Runs: value.Runs}
	if value.Pending != nil {
		result.PendingCommandID = value.Pending.CommandID
	}
	return result
}

func (r *Runtime) Pause(ctx context.Context, id string, paused bool) (Status, error) {
	value, err := r.load(id)
	if err != nil {
		return Status{}, err
	}
	_, owner, err := r.writer(ctx, value.Manifest.Server)
	if err != nil {
		return Status{}, err
	}
	if owner != value.Owner {
		return Status{}, kernel.Fail(kernel.ErrTargetRepositoryDenied, "integration belongs to another principal")
	}
	err = r.edit(id, func(value *state) error {
		value.Active = !paused
		if !paused {
			value.NextRunAt = time.Now()
		}
		return nil
	})
	if err != nil {
		return Status{}, err
	}
	return r.Status(id)
}

func (r *Runtime) Run(ctx context.Context, id string) (result Status, err error) {
	var lease [16]byte
	if _, err = rand.Read(lease[:]); err != nil {
		return result, err
	}
	leaseID := hex.EncodeToString(lease[:])
	var value state
	err = r.edit(id, func(current *state) error {
		if !current.Active {
			return kernel.Fail(kernel.ErrPreconditionFailed, "integration is paused")
		}
		if current.LeaseID != "" && time.Now().Before(current.LeaseUntil) {
			return kernel.Fail(kernel.ErrPreconditionFailed, "integration run is already in progress")
		}
		current.LeaseID = leaseID
		current.LeaseUntil = time.Now().Add(time.Duration(current.Manifest.TimeoutSeconds+90) * time.Second)
		current.Phase = "RUNNING"
		current.Runs++
		current.LastRunAt = time.Now()
		value = *current
		return nil
	})
	if err != nil {
		return result, err
	}
	// Bound all network and collection work inside the durable lease. A lost
	// Writer response remains pending even after this context is cancelled.
	ctx, cancel := context.WithTimeout(ctx, time.Duration(value.Manifest.TimeoutSeconds+60)*time.Second)
	defer cancel()
	update := func(action func(*state)) error {
		return r.edit(id, func(current *state) error {
			if current.LeaseID != leaseID {
				return kernel.Fail(kernel.ErrPreconditionFailed, "integration run lease was superseded")
			}
			action(current)
			return nil
		})
	}
	defer func() {
		releaseErr := update(func(current *state) {
			current.LeaseID = ""
			current.LeaseUntil = time.Time{}
			current.NextRunAt = time.Now().Add(time.Duration(current.Manifest.IntervalSeconds) * time.Second)
			if err != nil {
				current.Phase = "FAILED"
				current.LastError = string(kernel.CodeOf(err))
				if current.LastError == "" {
					current.LastError = "INTEGRATION_FAILED"
				}
			}
		})
		err = errors.Join(err, releaseErr)
		if err == nil {
			result, _ = r.Status(id)
		}
	}()
	writer, owner, err := r.writer(ctx, value.Manifest.Server)
	if err != nil {
		return result, err
	}
	if owner != value.Owner {
		return result, kernel.Fail(kernel.ErrTargetRepositoryDenied, "integration owner no longer matches KC login")
	}
	pending := value.Pending
	if pending == nil {
		base, headErr := writer.Head(ctx, value.Manifest.Repository, value.Manifest.Ref)
		if headErr != nil {
			return result, headErr
		}
		collection, collectErr := collect(ctx, value.Artifact, CollectInput{Manifest: value.Manifest, Checkpoint: value.Checkpoint, BaseCommit: base})
		if collectErr != nil {
			return result, collectErr
		}
		preview, previewErr := connector.Preview(connector.Plan{ConnectorID: value.Manifest.ID, Mode: value.Manifest.Mode, Scope: value.Manifest.Scope, TargetRepository: value.Manifest.Repository, TargetRef: value.Manifest.Ref, BaseCommit: base, Desired: collection.Desired, Observed: collection.Observed, SourceRefs: collection.SourceRefs, ProducedAt: collection.ProducedAt, ActorRef: owner})
		if previewErr != nil {
			return result, previewErr
		}
		if preview.Empty {
			err = update(func(current *state) {
				current.Checkpoint = Checkpoint{Cursor: collection.Cursor, Commit: base}
				current.Phase = "UNCHANGED"
				current.LastError = ""
			})
			return result, err
		}
		pending = &Pending{CommandID: connector.CommandID(value.Manifest.ID, string(kernel.CanonicalDigest(preview.ChangeSet))), ChangeSet: preview.ChangeSet, Cursor: collection.Cursor}
		if err = update(func(current *state) { current.Pending = pending }); err != nil {
			return result, err
		}
	}
	commit, commitErr := writer.Commit(ctx, pending.CommandID, pending.ChangeSet)
	if commitErr != nil {
		if publicationWasRejected(commitErr) {
			if saveErr := update(func(current *state) { current.Pending = nil }); saveErr != nil {
				return result, errors.Join(commitErr, saveErr)
			}
		}
		return result, commitErr
	}
	if commit == "" {
		return result, kernel.Fail(kernel.ErrTemporaryUnavailable, "Writer returned no published commit")
	}
	err = update(func(current *state) {
		current.Checkpoint = Checkpoint{Cursor: pending.Cursor, Commit: commit}
		current.Pending = nil
		current.Phase = "PUBLISHED"
		current.LastError = ""
	})
	return result, err
}

// These Writer rejections occur after replay lookup and before publication.
// Do not classify by HTTP status or a generic request-error family: in
// particular PRECONDITION_FAILED includes an unresolved durable command, and
// authentication, authorization or archive checks may hide an earlier receipt.
func publicationWasRejected(err error) bool {
	switch kernel.CodeOf(err) {
	case kernel.ErrNonFastForward, kernel.ErrSchemaUnsupported,
		kernel.ErrSchemaRevisionUnresolved, kernel.ErrSchemaInstanceInvalid,
		kernel.ErrSchemaIncompatible:
		return true
	default:
		return false
	}
}

// Tick runs each due active integration once. A failed integration does not
// stop another, and each pending command is retried without collecting again.
func (r *Runtime) Tick(ctx context.Context) ([]Status, error) {
	var ids []string
	err := r.transaction(false, func(tx *bolt.Tx) error {
		return tx.Bucket(statesBucket).ForEach(func(key, raw []byte) error {
			var value state
			if err := decode(raw, &value); err != nil {
				return err
			}
			if value.Active && !time.Now().Before(value.NextRunAt) && (value.LeaseID == "" || !time.Now().Before(value.LeaseUntil)) {
				ids = append(ids, string(key))
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(ids)
	results := []Status{}
	var failures []error
	for _, id := range ids {
		result, err := r.Run(ctx, id)
		if err != nil {
			failures = append(failures, err)
			result, _ = r.Status(id)
		}
		results = append(results, result)
		if ctx.Err() != nil {
			break
		}
	}
	return results, errors.Join(failures...)
}
