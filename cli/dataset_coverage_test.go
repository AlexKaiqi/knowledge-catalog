package cli_test

import (
	"context"
	snapshotclient "kc/snapshot/client"
	"testing"

	"kc/client"
	apphome "kc/home"
	"kc/kernel"
	"kc/snapshot"
	"kc/snapshot/dataset"
)

// dataset.md 场景覆盖补全：U1 同名文件映射到不同目标、U2 来源版本不可用时
// 旧版保留、U4 退役拒绝消费且不偷删源内容、U5 并发发布的 revision CAS 与
// 完整清单、U6 已交付引用不补齐未选中文件、U7 没有对象层的纯文件交付。
// 全部复用 directoryDelivery 夹具（普通 TreeWriter 文件提交，无对象声明）。

// U1：两个来源的同名文件映射到不同目标位置，交付字节与来源身份都保留。
func TestDatasetFileDeliveryMapsSameNameFilesToDistinctTargets(t *testing.T) {
	f := newDirectoryDelivery(t)
	f.write(t, deliveryData, "data-v2", []snapshot.TreeChange{
		{Path: "inbox/report.md", Content: []byte("data report\n")},
	})
	f.write(t, deliveryDocs, "docs-v2", []snapshot.TreeChange{
		{Path: "inbox/report.md", Content: []byte("docs report\n")},
	})
	sources := []dataset.DatasetSource{
		{Repository: deliveryData, Selector: snapshot.DefaultRef, File: "inbox/report.md", Target: "combined/data-report.md"},
		{Repository: deliveryDocs, Selector: snapshot.DefaultRef, File: "inbox/report.md", Target: "combined/docs-report.md"},
	}
	if err := f.publish(2, sources); err != nil {
		t.Fatal(err)
	}
	opened := f.mounts(t, nil)
	if len(opened.Mounts) != 0 {
		t.Fatalf("per-file entries must not become mounts: %#v", opened.Mounts)
	}
	assertListing(t, f, opened.Pin, "", "combined")
	assertListing(t, f, opened.Pin, "combined", "data-report.md", "docs-report.md")
	f.assertDeliveredRead(t, opened.Pin, "combined/data-report.md", "data report\n")
	f.assertDeliveredRead(t, opened.Pin, "combined/docs-report.md", "docs report\n")
	sources0, sources1 := 0, 0
	for _, item := range opened.Pin.Items {
		if item.Kind != dataset.DatasetItemFile {
			continue
		}
		switch item.Repository {
		case deliveryData:
			if item.Target == "combined/data-report.md" {
				sources0++
			}
		case deliveryDocs:
			if item.Target == "combined/docs-report.md" {
				sources1++
			}
		}
	}
	if sources0 != 1 || sources1 != 1 {
		t.Fatalf("pin items must keep each target bound to its own source: %#v", opened.Pin.Items)
	}
}

// U2 gap：来源 selector 指向已消失版本时发布被拒绝，已接受版本继续服务；
// 之后换回有效 selector 仍可正常发布新版。
func TestDatasetRepublishWithVanishedSelectorKeepsAcceptedVersion(t *testing.T) {
	f := newDirectoryDelivery(t)
	accepted := f.mounts(t, nil)
	sources := directoryDeliverySources("reference/policies", "reference/guides", "data/metrics")
	sources[0].Selector = "refs/heads/gone-v2"
	if err := f.publish(2, sources); kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
		t.Fatalf("vanished selector must be rejected as DATASET_INVALID, got %v", err)
	}
	current := f.mounts(t, nil)
	if current.Pin.Revision != accepted.Pin.Revision || current.Pin.PinID != accepted.Pin.PinID {
		t.Fatalf("failed publish must not move the accepted version: %#v vs %#v", current.Pin, accepted.Pin)
	}
	f.assertRead(t, accepted.Pin, "reference/policies", "policy v1\n")
	if err := f.publish(2, directoryDeliverySources("reference/policies", "reference/guides", "data/metrics")); err != nil {
		t.Fatal(err)
	}
	f.assertRead(t, accepted.Pin, "reference/policies", "policy v1\n")
}

// U4：退役后按当前生命周期拒绝新消费与再发布，但源仓 HEAD、提交与文件字节
// 保持原样；同一来源仍能支撑新 Dataset 的完整交付。
func TestDatasetRetireRejectsConsumptionAndPreservesSources(t *testing.T) {
	f := newDirectoryDelivery(t)
	accepted := f.mounts(t, nil)
	headDocs, headData, readDocs, readData := sourceReaders(t, f)
	f.assertRead(t, accepted.Pin, "reference/policies", "policy v1\n")

	if err := f.operator.SnapshotService().RetireDataset(context.Background(), "delivery", snapshotclient.RequestOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	var retiredMounts snapshotclient.WorkspaceFileMountsResponse
	err := f.reader.WorkspaceFilesService().Mounts(context.Background(), snapshotclient.WorkspaceFileMountsRequest{
		WorkspaceFileCoordinate: f.coordinate(t, nil),
	}, snapshotclient.RequestOptions{}, &retiredMounts)
	if kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
		t.Fatalf("retired dataset must refuse consumption, got %v", err)
	}
	// Pinned reads re-evaluate current state per request (dataset.md U3):
	// DatasetVersion honors current retirement, so the pinned read fails with
	// the same explicit lifecycle code as the unpinned resolve above — not a
	// generic authorization refusal. The pre-retire read of the same pin proves
	// the refusal is lifecycle-caused.
	var out snapshotclient.WorkspaceFileReadResponse
	err = f.reader.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
		WorkspaceFileCoordinate: f.coordinate(t, &accepted.Pin), Path: "reference/policies/README.md",
	}, snapshotclient.RequestOptions{}, &out)
	if kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
		t.Fatalf("retired dataset must refuse pinned reads with the lifecycle code, got %v", err)
	}
	if err := f.publish(2, directoryDeliverySources("reference/policies", "reference/guides", "data/metrics")); kernel.CodeOf(err) != kernel.ErrDatasetInvalid {
		t.Fatalf("retired dataset must refuse republish, got %v", err)
	}

	afterDocs, afterData, _, _ := sourceReaders(t, f)
	if afterDocs != headDocs || afterData != headData {
		t.Fatalf("retirement must not move source history: %s/%s vs %s/%s", headDocs, headData, afterDocs, afterData)
	}
	content, err := readDocs.ReadFile("handbook/policies/README.md", headDocs)
	if err != nil || string(content) != "policy v1\n" {
		t.Fatalf("retirement must keep source bytes: %q err=%v", content, err)
	}
	content, err = readData.ReadFile("semantic/metrics/README.md", headData)
	if err != nil || string(content) != "metric v1\n" {
		t.Fatalf("retirement must keep source bytes: %q err=%v", content, err)
	}
	sources := []dataset.DatasetSource{
		{Repository: deliveryDocs, Selector: snapshot.DefaultRef, Path: ptr("archive/policy"), SubPath: "handbook/policies"},
	}
	if err := f.operator.SnapshotService().DefineDataset(context.Background(),
		snapshotclient.DatasetRequest{Dataset: "delivery-archive", Revision: 1, Sources: sources}, snapshotclient.RequestOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	archive := f.coordinate(t, nil)
	archive.Dataset = "delivery-archive"
	// The consumer's grant is scoped to Dataset "delivery" (retired above), so
	// the source-preservation proof runs on the operator client: the same
	// sources still support a fresh, fully resolvable dataset.
	var mounts snapshotclient.WorkspaceFileMountsResponse
	if err := f.operator.WorkspaceFilesService().Mounts(context.Background(), snapshotclient.WorkspaceFileMountsRequest{
		WorkspaceFileCoordinate: archive,
	}, snapshotclient.RequestOptions{}, &mounts); err != nil {
		t.Fatal(err)
	}
	if len(mounts.Mounts) != 1 {
		t.Fatalf("the same sources must keep supporting a fresh dataset: %#v", mounts.Mounts)
	}
}

// U5：两个发布者并发发布同一 revision 时恰好一个被接受，另一方以
// NonFastForward 拒绝；消费方始终看到完整清单而非半份；旧 pin 不漂移；
// 落选方重试新 revision 后正常被接受。
func TestDatasetConcurrentPublishersCannotOverwriteOrRollBack(t *testing.T) {
	f := newDirectoryDelivery(t)
	first := f.mounts(t, nil)
	publisherB, err := client.New(client.Config{BaseURL: f.serverURL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisherB.Login(context.Background(), client.LoginRequest{Identity: client.Identity{Principal: "agent:publisher"}}); err != nil {
		t.Fatal(err)
	}
	sourcesA := directoryDeliverySources("reference/policies", "reference/guides", "data/metrics")
	metrics := "semantic/metrics"
	sourcesB := []dataset.DatasetSource{
		{Repository: deliveryData, Selector: snapshot.DefaultRef, Path: &metrics, SubPath: "semantic/metrics"},
		{Repository: deliveryDocs, Selector: snapshot.DefaultRef, File: "docs/guides/README.md", Target: "guide.md"},
	}
	race := make(chan attempt, 2)
	go publishRevision("directory-mounts", f.operator, sourcesA, race)
	go publishRevision("per-file-entries", publisherB, sourcesB, race)
	winner, loser := attempt{}, attempt{}
	for i := 0; i < 2; i++ {
		got := <-race
		if got.err == nil {
			if winner.label != "" {
				t.Fatal("two concurrent publishes of the same revision must not both be accepted")
			}
			winner = got
			continue
		}
		loser = got
	}
	if winner.label == "" {
		t.Fatalf("one publish must be accepted: winner=%#v loser=%#v", winner, loser)
	}
	if kernel.CodeOf(loser.err) != kernel.ErrNonFastForward {
		t.Fatalf("the rejected publish must be NonFastForward, got %v", loser.err)
	}
	current := f.mounts(t, nil)
	if current.Pin.Revision != 2 {
		t.Fatalf("current revision must be the accepted one: %#v", current.Pin)
	}
	files := 0
	for _, item := range current.Pin.Items {
		if item.Kind == dataset.DatasetItemFile {
			files++
		}
	}
	switch winner.label {
	case "directory-mounts":
		if len(current.Mounts) != 3 || files != 0 {
			t.Fatalf("accepted manifest must be complete, got mounts=%d files=%d", len(current.Mounts), files)
		}
	case "per-file-entries":
		if len(current.Mounts) != 1 || files != 1 {
			t.Fatalf("accepted manifest must be complete, got mounts=%d files=%d", len(current.Mounts), files)
		}
	default:
		t.Fatalf("unknown winner label %q", winner.label)
	}
	f.assertRead(t, first.Pin, "reference/policies", "policy v1\n")
	if err := f.publish(1, directoryDeliverySources("reference/policies", "reference/guides", "data/metrics")); kernel.CodeOf(err) != kernel.ErrNonFastForward {
		t.Fatalf("rollback revision must be refused, got %v", err)
	}
	retrySources := sourcesA
	if loser.label == "directory-mounts" {
		retrySources = sourcesB
	}
	if err := f.publish(3, retrySources); err != nil {
		t.Fatalf("loser must be able to retry with a fresh revision: %v", err)
	}
}

// attempt records one concurrent publish stream's outcome; the revision CAS is
// per-dataset, so the two publisher clients model two independent publish
// streams (identity scoping is covered by U3/U10 evidence).
type attempt struct {
	label string
	err   error
}

func publishRevision(label string, publisher *client.Client, sources []dataset.DatasetSource, out chan<- attempt) {
	err := publisher.SnapshotService().DefineDataset(context.Background(),
		snapshotclient.DatasetRequest{Dataset: "delivery", Revision: 2, Sources: sources}, snapshotclient.RequestOptions{}, nil)
	out <- attempt{label: label, err: err}
}

// U6 gap：已交付文件的内容引用未选中文件时，读取不自动补齐；未选中文件
// 不出现在交付命名空间。
func TestDeliveredReferenceDoesNotPullUnselectedFile(t *testing.T) {
	f := newDirectoryDelivery(t)
	f.write(t, deliveryData, "data-v2", []snapshot.TreeChange{
		{Path: "semantic/metrics/README.md", Content: []byte("metric v2, see details.md\n")},
		{Path: "semantic/metrics/details.md", Content: []byte("internal details\n")},
	})
	sources := []dataset.DatasetSource{
		{Repository: deliveryData, Selector: snapshot.DefaultRef, File: "semantic/metrics/README.md", Target: "picked/README.md"},
	}
	if err := f.publish(2, sources); err != nil {
		t.Fatal(err)
	}
	opened := f.mounts(t, nil)
	assertListing(t, f, opened.Pin, "picked", "README.md")
	f.assertDeliveredRead(t, opened.Pin, "picked/README.md", "metric v2, see details.md\n")
	var out snapshotclient.WorkspaceFileReadResponse
	err := f.reader.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
		WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), Path: "picked/details.md",
	}, snapshotclient.RequestOptions{}, &out)
	if err == nil || len(out.Content) != 0 {
		t.Fatalf("delivered reference must not pull the unselected file: %#v err=%v", out, err)
	}
}

// U7：来源只有普通文本、没有对象声明与查询投影时，发布与读取仍然成立；
// 交付字节逐字节保持原样——文件内的 origin/URL 文本不会被解析、外呼或改写
// （origin 指向不可路由端口，任何取值尝试都会失败）。
func TestDatasetDeliveryWithoutObjectLayerKeepsPlainBytes(t *testing.T) {
	f := newDirectoryDelivery(t)
	payload := "---\norigin: http://127.0.0.1:9/v1/access\n---\nfetch https://scheduler.example/never\n"
	f.write(t, deliveryData, "data-v2", []snapshot.TreeChange{
		{Path: "plain/notes.md", Content: []byte(payload)},
	})
	plain := "plain"
	sources := []dataset.DatasetSource{
		{Repository: deliveryData, Selector: snapshot.DefaultRef, Path: &plain, SubPath: "plain"},
	}
	if err := f.publish(2, sources); err != nil {
		t.Fatal(err)
	}
	opened := f.mounts(t, nil)
	if len(opened.Mounts) != 1 {
		t.Fatalf("plain file delivery must publish a mount: %#v", opened.Mounts)
	}
	assertListing(t, f, opened.Pin, "plain", "notes.md")
	var plainRead snapshotclient.WorkspaceFileReadResponse
	if err := f.reader.WorkspaceFilesService().Read(context.Background(), snapshotclient.WorkspaceFileReadRequest{
		WorkspaceFileCoordinate: f.coordinate(t, &opened.Pin), Path: "plain/notes.md",
	}, snapshotclient.RequestOptions{}, &plainRead); err != nil {
		t.Fatal(err)
	}
	if string(plainRead.Content) != payload || plainRead.TotalBytes != int64(len(payload)) {
		t.Fatalf("plain bytes must be delivered verbatim: %#v", plainRead)
	}
	if plainRead.Mount.Commit == "" || plainRead.Mount.Commit != opened.Pin.Repositories[deliveryData] {
		t.Fatalf("mount read must name the frozen source commit: %#v", plainRead.Mount)
	}
	for _, item := range opened.Pin.Items {
		if item.Kind != dataset.DatasetItemFile && item.Prefix != "plain" {
			continue
		}
		if item.Commit == "" {
			t.Fatalf("delivered items must keep frozen source commits: %#v", opened.Pin.Items)
		}
	}
}

func sourceReaders(t *testing.T, f *directoryDelivery) (docsHead, dataHead kernel.CommitID, docs, data snapshot.TreeReader) {
	t.Helper()
	opened, err := apphome.OpenDeployment(f.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	docsStore, err := opened.Store.Require(kernel.RepositoryID(deliveryDocs), kernel.ErrUsageInvalid)
	if err != nil {
		t.Fatal(err)
	}
	dataStore, err := opened.Store.Require(kernel.RepositoryID(deliveryData), kernel.ErrUsageInvalid)
	if err != nil {
		t.Fatal(err)
	}
	docsHead, err = docsStore.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	dataHead, err = dataStore.Head(snapshot.DefaultRef)
	if err != nil {
		t.Fatal(err)
	}
	docs, ok := docsStore.(snapshot.TreeReader)
	if !ok {
		t.Fatal("source store must expose immutable file reads")
	}
	data, ok = dataStore.(snapshot.TreeReader)
	if !ok {
		t.Fatal("source store must expose immutable file reads")
	}
	return docsHead, dataHead, docs, data
}

func ptr(path string) *string {
	return &path
}
