package projection

import (
	"context"
	"errors"
	"testing"

	"kc/declarativeindex/internal/testfixture"
	ports "kc/discovery/projection"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	objectmaintenance "kc/object/maintenance"
	"kc/snapshot"
)

// cancellingScanRepository cancels maintenance after the first object page or
// batch has been read, i.e. while a rebuild/apply is consuming objects.
type cancellingScanRepository struct {
	*testkit.ObjectRepository
	cancel context.CancelFunc
	onScan bool
	onRead bool
}

func (r *cancellingScanRepository) ScanSnapshotPage(commit kernel.CommitID, request objectmaintenance.ScanRequest) (objectmaintenance.ScanPage, error) {
	page, err := r.ObjectRepository.ScanSnapshotPage(commit, request)
	if r.onScan {
		r.cancel()
	}
	return page, err
}

func (r *cancellingScanRepository) ReadMany(ids []objectmodel.ObjectID, commit kernel.CommitID) (map[objectmodel.ObjectID]objectmodel.ObjectValue, error) {
	values, err := r.ObjectRepository.ReadMany(ids, commit)
	if r.onRead {
		r.cancel()
	}
	return values, err
}

type abortRecordingEngine struct {
	*testfixture.StateEngine
	aborted []error
}

func (e *abortRecordingEngine) BeginRebuild(meta ports.Meta) (ports.RebuildSession, error) {
	inner, err := e.StateEngine.BeginRebuild(meta)
	if err != nil {
		return nil, err
	}
	return &abortRecordingSession{RebuildSession: inner, engine: e}, nil
}

type abortRecordingSession struct {
	ports.RebuildSession
	engine *abortRecordingEngine
}

func (s *abortRecordingSession) Abort(err error) error {
	s.engine.aborted = append(s.engine.aborted, err)
	return s.RebuildSession.Abort(err)
}

func TestMaintenanceCancellationStopsInFlightProjectionWithoutAdvancingMeta(t *testing.T) {
	repo := testkit.MakeRepository(t, "kr://projection/cancel")
	commit := func(base kernel.CommitID, ops ...objectmodel.Operation) kernel.CommitID {
		t.Helper()
		result, err := repo.ApplyObjectCommit(objectmodel.CommitChangeSet{TargetRepository: repo.ID(), TargetRef: snapshot.DefaultRef, BaseCommit: base, ExpectedTargetCommit: base, Operations: ops})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	put := func(id, body string) objectmodel.Operation {
		return objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: objectmodel.ObjectID(id)}, SchemaRef: "schema/article", Value: map[string]any{"body": body}}
	}
	root := testkit.MustHead(t, repo, snapshot.DefaultRef)
	first := commit(root,
		objectmodel.Operation{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/article"}, Value: map[string]any{"entity": "Article", "fields": map[string]any{"body": map[string]any{"type": "string"}}}},
		testkit.IndexText("schema/article", "body"),
		put("article/a", "alpha"), put("article/b", "beta"),
	)

	t.Run("rebuild", func(t *testing.T) {
		engine := &abortRecordingEngine{StateEngine: &testfixture.StateEngine{}}
		idx := NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
		t.Cleanup(func() { _ = idx.Close() })
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		scanning := &cancellingScanRepository{ObjectRepository: repo, cancel: cancel, onScan: true}
		_, err := idx.Ensure(ctx, scanning, first)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled rebuild returned %v, want context.Canceled", err)
		}
		if engine.Meta.Basis != "" || len(engine.Docs) != 0 {
			t.Fatalf("cancelled rebuild advanced engine meta/docs: meta=%+v docs=%d", engine.Meta, len(engine.Docs))
		}
		if len(engine.aborted) != 1 || !errors.Is(engine.aborted[0], context.Canceled) {
			t.Fatalf("streaming rebuild session must be aborted with the cancellation: %v", engine.aborted)
		}
	})

	t.Run("incremental", func(t *testing.T) {
		engine := &testfixture.StateEngine{}
		idx := NewIndexEngine("", func(string, kernel.RepositoryID) (ports.Engine, error) { return engine, nil })
		t.Cleanup(func() { _ = idx.Close() })
		if _, err := idx.Ensure(context.Background(), repo, first); err != nil {
			t.Fatal(err)
		}
		ready := engine.Meta
		second := commit(first, put("article/a", "changed"))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		reading := &cancellingScanRepository{ObjectRepository: repo, cancel: cancel, onRead: true}
		_, err := idx.Ensure(ctx, reading, second)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled incremental apply returned %v, want context.Canceled", err)
		}
		if engine.Meta.Basis != ready.Basis || engine.Meta.Basis != first {
			t.Fatalf("cancelled apply moved READY meta: before=%+v after=%+v", ready, engine.Meta)
		}
	})
}
