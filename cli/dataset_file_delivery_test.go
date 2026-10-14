package cli_test

import (
	"context"
	"encoding/json"
	snapshotclient "kc/snapshot/client"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	apphome "kc/home"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/access"
	"kc/snapshot/dataset"
)

// doc/design/dataset.md U10: per-file entries publish as item truth and
// the File Gateway enumerates and reads the composed delivered tree — renamed
// files, files from another source mixed into one delivered directory, and
// file entries outside every mount. Delivery-time content conflicts refuse
// instead of picking a winner by source order.

func fileDeliverySources(paths ...string) []dataset.KnowledgeSetSource {
	sources := directoryDeliverySources(paths...)
	return append(sources,
		dataset.KnowledgeSetSource{
			Repository: deliveryData, Selector: snapshot.DefaultRef,
			File: "semantic/metrics/README.md", Target: "reference/policies/metric.md",
		},
		dataset.KnowledgeSetSource{
			Repository: deliveryDocs, Selector: snapshot.DefaultRef,
			File: "docs/guides/summary.md", Target: "data/exports/summary.txt",
		},
	)
}

func publishFileDelivery(t *testing.T, f *directoryDelivery, revision int, paths ...string) dataset.ResolvedKnowledgeSet {
	t.Helper()
	f.write(t, deliveryDocs, "docs-v"+string(rune('0'+revision)), []snapshot.TreeChange{
		{Path: "docs/guides/summary.md", Content: []byte("guide v1 summary\n")},
	})
	if err := f.publish(revision, fileDeliverySources(paths...)); err != nil {
		t.Fatal(err)
	}
	pin := f.mounts(t, nil).Pin
	return pin
}

func TestDatasetFileDeliveryServesRenamedAndMixedEntries(t *testing.T) {
	f := newDirectoryDelivery(t)
	publishFileDelivery(t, f, 2, "reference/policies", "reference/guides", "data/metrics")
	opened := f.mounts(t, nil)
	if len(opened.Mounts) != 3 {
		t.Fatalf("per-file entries must not become mounts: %#v", opened.Mounts)
	}
	files := 0
	for _, item := range opened.Pin.Items {
		if item.Kind == dataset.DatasetItemFile {
			files++
		}
	}
	if files != 2 {
		t.Fatalf("pin must carry the per-file entries: %#v", opened.Pin.Items)
	}
	t.Run("delivered tree listing", func(t *testing.T) {
		assertListing(t, f, opened.Pin, "", "reference", "data")
		assertListing(t, f, opened.Pin, "reference", "policies", "guides")
		assertListing(t, f, opened.Pin, "reference/policies", "README.md", "metric.md")
		assertListing(t, f, opened.Pin, "data", "metrics", "exports")
		assertListing(t, f, opened.Pin, "data/exports", "summary.txt")
	})
	t.Run("renamed and mixed reads", func(t *testing.T) {
		f.assertDeliveredRead(t, opened.Pin, "reference/policies/metric.md", "metric v1\n")
		f.assertDeliveredRead(t, opened.Pin, "data/exports/summary.txt", "guide v1 summary\n")
		f.assertRead(t, opened.Pin, "reference/policies", "policy v1\n")
	})
	t.Run("scope does not widen through delivered paths", func(t *testing.T) {
		if f.sourceReads.Load() == 0 {
			t.Fatal("accepted publication and reads must reach the source object endpoint")
		}
		for _, tc := range []struct {
			path string
			code kernel.ErrorCode
		}{
			{"handbook/secret.txt", kernel.ErrForbidden},
			{"reference/policies/../../../handbook/secret.txt", kernel.ErrUsageInvalid},
			{"semantic/secret.txt", kernel.ErrForbidden},
		} {
			before := f.sourceReads.Load()
			var out snapshotclient.WorkspaceFileReadResponse
			err := f.reader.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
				WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), Path: tc.path,
			}, snapshotclient.RequestOptions{}, &out)
			if err == nil || kernel.CodeOf(err) != tc.code || len(out.Content) != 0 || f.sourceReads.Load() != before {
				t.Fatalf("delivered path %q: err=%v content=%q source reads=%d", tc.path, err, out.Content, f.sourceReads.Load()-before)
			}
		}
	})
	t.Run("pin replay keeps the accepted delivered tree", func(t *testing.T) {
		old := opened.Pin
		if err := f.publish(3, directoryDeliverySources("reference/policies", "reference/guides", "data/metrics")); err != nil {
			t.Fatal(err)
		}
		f.assertDeliveredRead(t, old, "reference/policies/metric.md", "metric v1\n")
		current := f.mounts(t, nil)
		var out snapshotclient.WorkspaceFileReadResponse
		err := f.reader.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
			WorkspaceFileCoordinate: f.coordinate(t, &current.Pin), Path: "reference/policies/metric.md",
		}, snapshotclient.RequestOptions{}, &out)
		// The delivered namespace is the union of per-file entries and mount
		// content. The serving version dropped the entry, and the mapped
		// source file does not exist, so the miss answers exactly like a
		// mount-relative read of the same path — no content, no leak.
		if err == nil || kernel.CodeOf(err) != kernel.ErrKnowledgeRefUnresolved || len(out.Content) != 0 {
			t.Fatalf("the serving version dropped the per-file entry; reads must follow the version: %#v err=%v", out, err)
		}
	})
}

func TestDatasetFileDeliveryRejectsContentConflicts(t *testing.T) {
	f := newDirectoryDelivery(t)
	accepted := f.mounts(t, nil)
	beforeHead, beforeState, beforeHistory := f.registryState(t)
	sources := append(directoryDeliverySources("reference/policies", "reference/guides", "data/metrics"),
		dataset.KnowledgeSetSource{
			Repository: deliveryData, Selector: snapshot.DefaultRef,
			File: "semantic/metrics/README.md", Target: "reference/policies/README.md",
		})
	if err := f.publish(2, sources); kernel.CodeOf(err) != kernel.ErrKnowledgeSetInvalid {
		t.Fatalf("conflicting delivered content must be rejected before publication: %v", err)
	}
	afterHead, afterState, afterHistory := f.registryState(t)
	if afterHead != beforeHead || !reflect.DeepEqual(afterState, beforeState) || !reflect.DeepEqual(afterHistory, beforeHistory) {
		t.Fatalf("rejected content changed authority: HEAD %s -> %s, state %#v -> %#v, history %#v -> %#v",
			beforeHead, afterHead, beforeState, afterState, beforeHistory, afterHistory)
	}
	if current := f.mounts(t, nil); !reflect.DeepEqual(current, accepted) {
		t.Fatalf("rejected content replaced the accepted release: %#v", current)
	}
	f.assertRead(t, accepted.Pin, "reference/policies", "policy v1\n")
	assertListing(t, f, accepted.Pin, "reference/policies", "README.md")
}

func (f *directoryDelivery) registryState(t *testing.T) (string, dataset.State, dataset.History) {
	t.Helper()
	opened, err := apphome.OpenDeployment(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	head, err := opened.Registry.Head()
	if err != nil {
		t.Fatal(err)
	}
	history, err := opened.Datasets.Log(dataset.LogQuery{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	return head, dataset.NormalizeState(opened.Datasets.DumpState()), history
}

func assertListing(t *testing.T, f *directoryDelivery, pin dataset.ResolvedKnowledgeSet, dir string, want ...string) {
	t.Helper()
	var out snapshotclient.WorkspaceFileDirectoryResponse
	if err := f.reader.WorkspaceFilesService().Directory(context.Background(), snapshotclient.WorkspaceFileDirectoryRequest{
		WorkspaceFileCoordinate: f.coordinate(t, &pin), Path: dir,
	}, snapshotclient.RequestOptions{}, &out); err != nil {
		t.Fatalf("listing %q: %v", dir, err)
	}
	got := make([]string, 0, len(out.Entries))
	for _, entry := range out.Entries {
		got = append(got, entry.Name)
	}
	if len(got) != len(want) {
		t.Fatalf("listing %q = %v, want %v", dir, got, want)
	}
	for _, name := range want {
		found := false
		for _, entry := range out.Entries {
			if entry.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("listing %q misses %q: %v", dir, name, got)
		}
	}
	if !out.Exhausted {
		t.Fatalf("listing %q must exhaust without a mount page", dir)
	}
}

func (f *directoryDelivery) assertDeliveredRead(t *testing.T, pin dataset.ResolvedKnowledgeSet, path, content string) {
	t.Helper()
	var out snapshotclient.WorkspaceFileReadResponse
	if err := f.reader.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
		WorkspaceFileCoordinate: f.coordinate(t, &pin), Path: path,
	}, snapshotclient.RequestOptions{}, &out); err != nil {
		t.Fatal(err)
	}
	if string(out.Content) != content || out.Path != path || !out.EOF || out.TotalBytes != int64(len(content)) {
		t.Fatalf("wrong delivered bytes at %s: %#v", path, out)
	}
	if out.Item == nil || out.Item.Target != path {
		t.Fatalf("delivered read must name its per-file entry: %#v", out.Item)
	}
	if out.Mount.Commit == "" || out.Mount.Commit != pin.Repositories[out.Mount.Repository] {
		t.Fatalf("delivered read must name the frozen source commit: %#v", out.Mount)
	}
}

// A pin freezes content, never permission (dataset.md U3): every gateway
// request re-evaluates the current per-repository file.read grant, including
// pinned historical reads. B-24's revocation sequence: grant → publish →
// pinned reads OK → revoke → the same pin is dead for that principal.
func TestDatasetDeliveryRejectsReadsAfterGrantRevocation(t *testing.T) {
	f := newDirectoryDelivery(t)
	opened := f.mounts(t, nil)
	f.assertRead(t, opened.Pin, "reference/policies", "policy v1\n")
	// Delivered addressing serves mount members too (per-file entries only
	// carry Item metadata; mount members name their frozen Mount instead).
	var pre snapshotclient.WorkspaceFileReadResponse
	if err := f.reader.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
		WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), Path: "reference/policies/README.md",
	}, snapshotclient.RequestOptions{}, &pre); err != nil || string(pre.Content) != "policy v1\n" ||
		pre.Item != nil || pre.Mount.Commit != opened.Pin.Repositories[pre.Mount.Repository] {
		t.Fatalf("delivered mount-member read before revocation: %#v err=%v", pre, err)
	}

	// Revoke the consumer's rule through the admin surface, as an operator.
	var raw any
	if err := f.operator.AdminService().Grants(context.Background(), snapshotclient.RequestOptions{}, &raw); err != nil {
		t.Fatal(err)
	}
	blob, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Rules []struct {
			ID        string   `json:"id"`
			Principal string   `json:"principal"`
			Actions   []string `json:"actions"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(blob, &listed); err != nil {
		t.Fatalf("grant list shape: %v %s", err, blob)
	}
	revoked := 0
	for _, rule := range listed.Rules {
		if rule.Principal != "agent:consumer" {
			continue
		}
		if err := f.operator.AdminService().RemoveGrant(context.Background(), rule.ID, snapshotclient.RequestOptions{}, nil); err != nil {
			t.Fatal(err)
		}
		revoked++
	}
	if revoked == 0 {
		t.Fatalf("consumer grant not found: %s", blob)
	}

	// The saved pin is replayable but no longer consumable: mounts, listing,
	// mount-relative reads and delivered reads all answer with the current
	// authorization verdict, not the pin's.
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"mounts", func() error {
			var out snapshotclient.WorkspaceFileMountsResponse
			return f.reader.WorkspaceFilesService().Mounts(context.Background(), snapshotclient.WorkspaceFileMountsRequest{
				WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin),
			}, snapshotclient.RequestOptions{}, &out)
		}},
		{"directory", func() error {
			var out snapshotclient.WorkspaceFileDirectoryResponse
			return f.reader.WorkspaceFilesService().Directory(context.Background(), snapshotclient.WorkspaceFileDirectoryRequest{
				WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), MountPath: "reference/policies",
			}, snapshotclient.RequestOptions{}, &out)
		}},
		{"mount read", func() error {
			var out snapshotclient.WorkspaceFileReadResponse
			return f.reader.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
				WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), MountPath: "reference/policies", File: "README.md",
			}, snapshotclient.RequestOptions{}, &out)
		}},
		{"delivered read", func() error {
			var out snapshotclient.WorkspaceFileReadResponse
			return f.reader.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
				WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), Path: "reference/policies/README.md",
			}, snapshotclient.RequestOptions{}, &out)
		}},
	} {
		if err := tc.call(); err == nil || kernel.CodeOf(err) != kernel.ErrForbidden {
			t.Fatalf("revoked consumer %s must fail with FORBIDDEN: %#v err=%v", tc.name, err, kernel.CodeOf(err))
		}
	}

	// Positive control: another principal's grant is untouched.
	var out snapshotclient.WorkspaceFileMountsResponse
	if err := f.operator.WorkspaceFilesService().Mounts(context.Background(), snapshotclient.WorkspaceFileMountsRequest{
		WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin),
	}, snapshotclient.RequestOptions{}, &out); err != nil || len(out.Mounts) != 3 {
		t.Fatalf("operator keeps its own access after consumer revocation: %#v err=%v", out.Mounts, err)
	}
}

// Removing the whole consume rule proves the resolve gate, while this journey
// reaches the file gate through the real durable grant update surface.
func TestDatasetDeliveryRechecksFileReadAfterGrantUpdate(t *testing.T) {
	f := newDirectoryDelivery(t)
	opened := f.mounts(t, nil)
	f.assertRead(t, opened.Pin, "reference/policies", "policy v1\n")
	var first snapshotclient.WorkspaceFileDirectoryResponse
	request := snapshotclient.WorkspaceFileDirectoryRequest{
		WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), Path: "", Limit: 1,
	}
	if err := f.reader.WorkspaceFilesService().Directory(context.Background(), request, snapshotclient.RequestOptions{}, &first); err != nil || first.Continuation == "" {
		t.Fatalf("authorized bounded page has no continuation: %#v %v", first, err)
	}
	updateDeliveryConsumerGrant(t, f, []string{"dataset.resolve"})

	var resolved dataset.ResolvedKnowledgeSet
	if err := f.reader.SnapshotService().ResolveKnowledgeSet(context.Background(), "delivery", snapshotclient.KnowledgeSetResolveRequest{}, snapshotclient.RequestOptions{}, &resolved); err != nil || !reflect.DeepEqual(resolved, opened.Pin) {
		t.Fatalf("file-only revocation changed resolve or its fixed publication: %#v %v", resolved, err)
	}
	var mounts snapshotclient.WorkspaceFileMountsResponse
	if err := f.reader.WorkspaceFilesService().Mounts(context.Background(), snapshotclient.WorkspaceFileMountsRequest{WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin)}, snapshotclient.RequestOptions{}, &mounts); err != nil || len(mounts.Mounts) != 0 || !reflect.DeepEqual(mounts.Pin, opened.Pin) {
		t.Fatalf("current file revocation retained deliverable mounts or changed publication: %#v %v", mounts, err)
	}
	var directory snapshotclient.WorkspaceFileDirectoryResponse
	if err := f.reader.WorkspaceFilesService().Directory(context.Background(), request, snapshotclient.RequestOptions{}, &directory); err != nil || len(directory.Entries) != 0 || !directory.Exhausted || directory.Continuation != "" {
		t.Fatalf("current file revocation leaked directory entries: %#v %v", directory, err)
	}
	request.Continuation = first.Continuation
	if err := f.reader.WorkspaceFilesService().Directory(context.Background(), request, snapshotclient.RequestOptions{}, &directory); kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("old cursor ignored the changed deliverable scope: %#v %v", directory, err)
	}
	for _, address := range []snapshotclient.WorkspaceFileReadRequest{
		{WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), MountPath: "reference/policies", File: "README.md"},
		{WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), Path: "reference/policies/README.md"},
	} {
		var read snapshotclient.WorkspaceFileReadResponse
		if err := f.reader.WorkspaceFilesService().Read(context.Background(), address, snapshotclient.RequestOptions{}, &read); kernel.CodeOf(err) != kernel.ErrForbidden || len(read.Content) != 0 {
			t.Fatalf("old pin bypassed the current file gate: %#v %v", read, err)
		}
	}
	var control snapshotclient.WorkspaceFileReadResponse
	if err := f.operator.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
		WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), Path: "reference/policies/README.md",
	}, snapshotclient.RequestOptions{}, &control); err != nil || string(control.Content) != "policy v1\n" || control.Mount.Commit != opened.Pin.Repositories[deliveryDocs] {
		t.Fatalf("another principal lost its independent fixed read: %#v %v", control, err)
	}
}

func updateDeliveryConsumerGrant(t *testing.T, f *directoryDelivery, actions []string) {
	t.Helper()
	var inventory access.GrantList
	if err := f.operator.AdminService().Grants(context.Background(), snapshotclient.RequestOptions{}, &inventory); err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, rule := range inventory.Rules {
		if rule.Principal != "agent:consumer" || rule.Dataset != "delivery" {
			continue
		}
		found++
		observed := snapshotclient.GrantExpected{ID: rule.ID, SharedBy: rule.SharedBy, ShareID: rule.ShareID,
			GrantRequest: snapshotclient.GrantRequest{Principal: rule.Principal, Actions: slices.Clone(rule.Actions),
				Repository: rule.Repo, Dataset: rule.Dataset, Ref: rule.Ref, Object: rule.Object, Aspect: rule.Aspect}}
		replacement := observed.GrantRequest
		replacement.Actions = slices.Clone(actions)
		var updated access.Rule
		if err := f.operator.AdminService().UpdateGrant(context.Background(), rule.ID, snapshotclient.GrantUpdateRequest{
			GrantRequest: replacement, Expected: observed,
		}, snapshotclient.RequestOptions{}, &updated); err != nil {
			t.Fatal(err)
		}
		wanted := rule
		wanted.Actions = slices.Clone(actions)
		if !reflect.DeepEqual(updated, wanted) {
			t.Fatalf("grant update did not preserve identity and the full replacement: %#v want %#v", updated, wanted)
		}
	}
	if found != 1 {
		t.Fatalf("expected one consumer publication rule, found %d", found)
	}
}

func TestDatasetDeliveryFiltersUnauthorizedSourcesBeforeClone(t *testing.T) {
	f := newDirectoryDelivery(t)
	sources := append(directoryDeliverySources("reference/policies", "reference/guides", "data/metrics"),
		dataset.KnowledgeSetSource{Repository: deliveryDocs, Selector: snapshot.DefaultRef, File: "docs/guides/README.md", Target: "data/exports/guide.txt"},
		dataset.KnowledgeSetSource{Repository: deliveryData, Selector: snapshot.DefaultRef, File: "semantic/metrics/README.md", Target: "unreadable/metric.txt"},
	)
	if err := f.publish(2, sources); err != nil {
		t.Fatal(err)
	}
	opened := f.mounts(t, nil)
	f.assertDeliveredRead(t, opened.Pin, "unreadable/metric.txt", "metric v1\n")
	updateDeliveryConsumerGrant(t, f, []string{"dataset.resolve"})
	if err := f.operator.AdminService().AddGrant(context.Background(), snapshotclient.GrantRequest{
		Principal: "agent:consumer", Actions: []string{"file.read"}, Repository: deliveryDocs, Dataset: "delivery",
	}, snapshotclient.RequestOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	filtered := f.mounts(t, &opened.Pin)
	if !reflect.DeepEqual(filtered.Pin, opened.Pin) || len(filtered.Mounts) != 2 {
		t.Fatalf("authorization changed accepted pin or retained unreadable source: %#v", filtered)
	}
	for _, mount := range filtered.Mounts {
		if mount.Repository != deliveryDocs || mount.Commit != opened.Pin.Repositories[deliveryDocs] {
			t.Fatalf("unreadable source survived current file filtering: %#v", mount)
		}
	}
	assertListing(t, f, opened.Pin, "", "data", "reference")
	assertListing(t, f, opened.Pin, "data", "exports")
	assertListing(t, f, opened.Pin, "data/exports", "guide.txt")
	f.assertDeliveredRead(t, opened.Pin, "data/exports/guide.txt", "guide v1\n")
	for _, path := range []string{"unreadable/metric.txt", "data/metrics/README.md"} {
		var read snapshotclient.WorkspaceFileReadResponse
		if err := f.reader.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
			WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), Path: path,
		}, snapshotclient.RequestOptions{}, &read); kernel.CodeOf(err) != kernel.ErrForbidden || len(read.Content) != 0 {
			t.Fatalf("unreadable source returned bytes at %s: %#v %v", path, read, err)
		}
	}
	target := filepath.Join(t.TempDir(), "authorized-clone")
	receipt, err := f.clone(t, target)
	if err != nil || receipt["files"] != float64(3) {
		t.Fatalf("partial-scope clone did not materialize exactly the authorized tree: %#v %v", receipt, err)
	}
	wanted := map[string]string{"reference/policies/README.md": "policy v1\n", "reference/guides/README.md": "guide v1\n", "data/exports/guide.txt": "guide v1\n"}
	actual := map[string]string{}
	if err := filepath.WalkDir(target, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(target, path)
		actual[filepath.ToSlash(relative)] = string(content)
		return err
	}); err != nil || !reflect.DeepEqual(actual, wanted) {
		t.Fatalf("clone wrote an unauthorized file or wrong content: %#v want %#v error %v", actual, wanted, err)
	}
	var control snapshotclient.WorkspaceFileReadResponse
	if err := f.operator.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
		WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), Path: "unreadable/metric.txt",
	}, snapshotclient.RequestOptions{}, &control); err != nil || string(control.Content) != "metric v1\n" || control.Mount.Commit != opened.Pin.Repositories[deliveryData] {
		t.Fatalf("another principal lost its independently authorized source: %#v %v", control, err)
	}
}

// kc dataset clone materializes the composed delivered tree through the same
// closed gateway surface, refuses non-empty targets, and replays saved pins.
func TestDatasetCloneMaterializesDeliveredTree(t *testing.T) {
	f := newDirectoryDelivery(t)
	v2 := publishFileDelivery(t, f, 2, "reference/policies", "reference/guides", "data/metrics")
	opened := f.mounts(t, &v2)
	target := t.TempDir()
	receipt, err := f.clone(t, target)
	if err != nil {
		t.Fatal(err)
	}
	if receipt["pinId"] != opened.Pin.PinID {
		t.Fatalf("clone must materialize the resolved version: %#v", receipt)
	}
	for _, tc := range []struct{ path, content string }{
		{"reference/policies/README.md", "policy v1\n"},
		{"reference/guides/README.md", "guide v1\n"},
		// summary.md is delivered twice on purpose: it rides the
		// reference/guides mount subtree AND is renamed into data/exports.
		{"reference/guides/summary.md", "guide v1 summary\n"},
		{"data/metrics/README.md", "metric v1\n"},
		{"reference/policies/metric.md", "metric v1\n"},
		{"data/exports/summary.txt", "guide v1 summary\n"},
	} {
		raw, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(tc.path)))
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != tc.content {
			t.Fatalf("cloned %s = %q, want %q", tc.path, raw, tc.content)
		}
	}
	if got := receipt["files"]; got != float64(6) {
		t.Fatalf("clone receipt must count every delivered file: %#v", receipt)
	}
	// A clone never overwrites: a non-empty target is refused outright.
	busy := t.TempDir()
	if err := os.WriteFile(filepath.Join(busy, "keep.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.clone(t, busy); err == nil || kernel.CodeOf(err) != kernel.ErrPreconditionFailed {
		t.Fatalf("clone must refuse a non-empty target: %v", err)
	}
	// The clone consumes the current serving version, not a saved pin: after
	// the next release it delivers only the new file list.
	if err := f.publish(3, directoryDeliverySources("reference/policies", "reference/guides", "data/metrics")); err != nil {
		t.Fatal(err)
	}
	fresh := t.TempDir()
	receipt, err = f.clone(t, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if got := receipt["files"]; got != float64(4) {
		// Three mount files plus summary.md, which was committed to the source
		// repository and therefore rides the reference/guides mount in v3 too.
		// Only the two per-file entries were dropped by the new version.
		t.Fatalf("fresh clone must follow the current serving version: %#v", receipt)
	}
	if _, err := os.Stat(filepath.Join(fresh, "reference", "policies", "metric.md")); !os.IsNotExist(err) {
		t.Fatalf("fresh clone adopted a per-file entry the serving version dropped: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fresh, "data", "exports", "summary.txt")); !os.IsNotExist(err) {
		t.Fatalf("fresh clone adopted a per-file entry the serving version dropped: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "reference", "policies", "metric.md")); err != nil {
		t.Fatalf("the earlier clone keeps its own materialized bytes: %v", err)
	}
}

func (f *directoryDelivery) clone(t *testing.T, dir string) (map[string]any, error) {
	t.Helper()
	// Product argv rejects --pin (doc/design/affordance.md), so there is nothing to pass.
	// TestDatasetCloneJourney owns the literal-argv evidence; this helper keeps
	// the deep assertions compact by reusing the same production Run entry.
	result := kcRemote(t, f.serverURL, "agent:consumer", "dataset", "clone", "delivery", dir)
	if result.Status != 0 {
		var fault struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(result.Stdout), &fault); err != nil {
			t.Fatalf("clone failure without FaultJSON: %d %s", result.Status, result.Stdout)
		}
		return nil, kernel.Fail(kernel.ErrorCode(fault.Error.Code), "%s", fault.Error.Message)
	}
	var receipt map[string]any
	if err := json.Unmarshal([]byte(result.Stdout), &receipt); err != nil {
		t.Fatalf("clone receipt is not JSON: %s", result.Stdout)
	}
	return receipt, nil
}
