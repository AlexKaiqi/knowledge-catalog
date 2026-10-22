package service_test

import (
	"context"
	"errors"
	liveservice "kc/liveaccess/service"
	"reflect"
	"testing"

	"kc/internal/bindingfixture"
	"kc/internal/testkit"
	"kc/kernel"
	serving "kc/liveaccess"
	objectmodel "kc/object"
	"kc/object/reader"
	"kc/observability"
	"kc/snapshot"
)

type stateLookup struct {
	requests []serving.StateLookupRequest
	result   serving.StateObservation
	err      error
}

func (s *stateLookup) LookupState(_ context.Context, request serving.StateLookupRequest) (serving.StateObservation, error) {
	s.requests = append(s.requests, request)
	return s.result, s.err
}

func setupServing(t *testing.T) (*reader.Serving, kernel.RepositoryID, kernel.CommitID, objectmodel.Address) {
	t.Helper()
	s := testkit.NewSetup(t, "")
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "job/orders", AspectName: "runtime"}
	commit, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/job.runtime"}, Value: map[string]any{
				"entity": "Job", "aspect": "runtime",
				"fields": map[string]any{"status": map[string]any{"type": "string"}},
			}},
			bindingfixture.PutDescriptor("resource/scheduler", bindingfixture.Descriptor("https://scheduler.example")),
			bindingfixture.PutDescriptor("resource/scheduler-stream", bindingfixture.StreamDescriptor("https://scheduler.example")),
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: address.ObjectID, AspectName: "definition"}, Value: map[string]any{"owner": "data"}},
			bindingfixture.PutAspect(address.ObjectID, address.AspectName, "schema/job.runtime", "resource/scheduler"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	base := reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		if id != s.RepositoryID {
			return nil, errors.New("unexpected repository")
		}
		return s.Repo.Repository, nil
	}, reader.DatasetPin{SetID: "agent", Repositories: map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit}, Items: reader.WholeRepositoryItems(map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit})})
	return base, s.RepositoryID, commit, address
}

func TestBindingUnitHydratesFromDescriptorOrigin(t *testing.T) {
	s := testkit.NewSetup(t, "")
	address := objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "stats"}
	commit, err := s.Repo.ApplyObjectCommit(objectmodel.CommitChangeSet{
		TargetRepository: s.RepositoryID, TargetRef: snapshot.DefaultRef,
		BaseCommit: s.RootCommitID, ExpectedTargetCommit: s.RootCommitID,
		Operations: []objectmodel.Operation{
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: "schema/table.stats"}, Value: map[string]any{
				"entity": "Table", "aspect": "stats",
				"fields": map[string]any{"rowCount": map[string]any{"type": "number"}},
			}},
			bindingfixture.PutDescriptor("resource/stats", bindingfixture.Descriptor("https://stats.example")),
			{Op: objectmodel.OpPut, Address: objectmodel.Address{Kind: objectmodel.KindAspect, ObjectID: "table/orders", AspectName: "properties"}, Value: map[string]any{"name": "orders"}},
			bindingfixture.PutAspect("table/orders", "stats", "schema/table.stats", "resource/stats"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	base := reader.Open(func(id kernel.RepositoryID) (objectmodel.Repository, error) {
		if id != s.RepositoryID {
			return nil, errors.New("unexpected repository")
		}
		return s.Repo.Repository, nil
	}, reader.DatasetPin{SetID: "agent", Repositories: map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit}, Items: reader.WholeRepositoryItems(map[kernel.RepositoryID]kernel.CommitID{s.RepositoryID: commit})})
	lookup := &stateLookup{result: serving.StateObservation{
		Value: map[string]any{"rowCount": float64(12)},
		Basis: objectmodel.ObservationBasis{
			BindingGeneration: "stats-v1", Consistency: objectmodel.ObservationLatestOnly,
			ObservedAt: "2026-09-18T00:00:00Z",
		},
	}}
	service := liveservice.Open(base, lookup, observability.IdentityContext{Principal: "agent"})
	results, err := service.ReadAddress(context.Background(), address)
	if err != nil || len(results) != 1 {
		t.Fatalf("binding unit access: %#v %v", results, err)
	}
	value, _ := results[0].Value.(map[string]any)
	if value["rowCount"] != float64(12) || lookup.requests[0].Origin != "https://stats.example" {
		t.Fatalf("hydrated aspect %#v requests %#v", results[0], lookup.requests)
	}
	assembled, err := service.Read(context.Background(), address.ObjectID)
	if err != nil || len(assembled) != 1 {
		t.Fatalf("object read: %#v %v", assembled, err)
	}
	body, _ := assembled[0].Value.(map[string]any)
	if body["stats"].(map[string]any)["rowCount"] != float64(12) {
		t.Fatalf("assembled bound aspect: %#v", assembled[0].Value)
	}
}

func TestStateBindingHydratesConsumerReadAndKeepsBothBases(t *testing.T) {
	base, repositoryID, commit, address := setupServing(t)
	lookup := &stateLookup{result: serving.StateObservation{
		Value: map[string]any{"status": "running", "progress": float64(70)},
		Basis: objectmodel.ObservationBasis{
			BindingGeneration: "scheduler-config-7", Consistency: objectmodel.ObservationRepeatable,
			SourceRevision: "job-rev-42", ObservedAt: "2026-08-27T08:30:00Z",
		},
	}}
	identity := observability.IdentityContext{Principal: "agent", OnBehalfOf: "alice"}
	service := liveservice.Open(base, lookup, identity)

	results, err := service.Read(context.Background(), address.ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("%#v", results)
	}
	value := results[0].Value.(map[string]any)
	runtime := value["runtime"].(map[string]any)
	if runtime["status"] != "running" || runtime["progress"] != float64(70) {
		t.Fatalf("bound value was not hydrated: %#v", value)
	}
	if value["definition"].(map[string]any)["owner"] != "data" {
		t.Fatalf("snapshot aspect was not preserved: %#v", value)
	}
	if results[0].Commit != commit || len(results[0].Observations) != 1 {
		t.Fatalf("result lost declaration/observation versions: %#v", results[0])
	}
	version := results[0].Observations[0]
	if version.Address != address || version.DeclarationCommit != commit || version.DeclarationDigest == "" || version.Basis.SourceRevision != "job-rev-42" {
		t.Fatalf("observation version: %#v", version)
	}
	if len(lookup.requests) != 1 || lookup.requests[0].Binding.Repository != repositoryID || lookup.requests[0].Identity != identity {
		t.Fatalf("lookup request did not preserve binding and identity: %#v", lookup.requests)
	}

	raw, err := base.ReadAddress(address)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 1 || raw[0].Value != nil || len(raw[0].Declarations) != 1 || !raw[0].Declarations[0].ValueSource.IsBinding() {
		t.Fatalf("a binding unit carries its declaration and no Snapshot value: %#v", raw)
	}
}

func TestStateBindingSelectionAvoidsUnrequestedLookup(t *testing.T) {
	base, _, _, address := setupServing(t)
	lookup := &stateLookup{result: serving.StateObservation{
		Value: map[string]any{"status": "running"},
		Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationLatestOnly, ObservedAt: "2026-08-27T08:30:00Z"},
	}}
	service := liveservice.Open(base, lookup, observability.IdentityContext{Principal: "agent"})
	// Selecting the static coordinate instead of the Binding coordinate is what
	// keeps the runtime out of the request; nothing is read and then trimmed.
	results, err := service.ReadAddress(context.Background(), objectmodel.Address{
		Kind: objectmodel.KindAspect, ObjectID: address.ObjectID, AspectName: "definition",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(lookup.requests) != 0 {
		t.Fatalf("unselected Binding caused a runtime call: %#v", lookup.requests)
	}
	if len(results) != 1 || !reflect.DeepEqual(results[0].Value, map[string]any{"owner": "data"}) {
		t.Fatalf("coordinate result: %#v", results)
	}
}

func TestBoundReadFailsClosedWithoutStateRuntime(t *testing.T) {
	base, _, _, address := setupServing(t)
	values, err := liveservice.Open(base, nil, observability.IdentityContext{Principal: "agent"}).ReadAddress(context.Background(), address)
	assertFailedUnitRead(t, values, err, address, kernel.ErrCapabilityUnsatisfied)
}

func TestOrdinaryReadRejectsStreamBinding(t *testing.T) {
	base, repositoryID, commit, address := setupServing(t)
	lookup := &stateLookup{}
	service := liveservice.Open(base, lookup, observability.IdentityContext{Principal: "agent"})
	raw := objectmodel.FederatedValue{
		ObjectRef:  objectmodel.ObjectRef{Repository: repositoryID, Object: address.ObjectID},
		Repository: repositoryID, Commit: commit, ObjectID: address.ObjectID, Address: address,
		Declarations: []objectmodel.UnitDeclaration{{
			Address:     address,
			ValueSource: bindingfixture.Source("resource/scheduler-stream"),
		}},
	}
	value, err := service.Hydrate(context.Background(), raw)
	assertFailedUnitRead(t, []objectmodel.ObservedValue{value}, err, address, kernel.ErrCapabilityUnsatisfied)
	if len(lookup.requests) != 0 {
		t.Fatal("ordinary READ must not call a State runtime for a Stream Binding")
	}
}

func TestStateRuntimeFailuresAndInvalidBasisFailHonestly(t *testing.T) {
	base, _, _, address := setupServing(t)
	lookup := &stateLookup{err: errors.New("connection reset")}
	values, err := liveservice.Open(base, lookup, observability.IdentityContext{Principal: "agent"}).ReadAddress(context.Background(), address)
	assertFailedUnitRead(t, values, err, address, kernel.ErrTemporaryUnavailable)

	lookup.err = nil
	lookup.result = serving.StateObservation{Value: true, Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationLatestOnly}}
	values, err = liveservice.Open(base, lookup, observability.IdentityContext{Principal: "agent"}).ReadAddress(context.Background(), address)
	assertFailedUnitRead(t, values, err, address, kernel.ErrCapabilityUnsatisfied)
}

// platformRecordLookup models the platform-side retained observation record of
// a latest-only source (resource-access.md U4 / 恢复与观察保留): the record is
// the ONLY re-read basis, so losing it is a recovery fault while normal expiry
// is lifecycle end. Both must fail explicitly and distinguishably — never with
// the current value or an empty history.
type platformRecordLookup struct {
	requests []serving.StateLookupRequest
	record   *serving.StateObservation
	state    string // "retained" | "lost" | "expired"
}

func (s *platformRecordLookup) LookupState(_ context.Context, request serving.StateLookupRequest) (serving.StateObservation, error) {
	s.requests = append(s.requests, request)
	switch s.state {
	case "retained":
		if s.record != nil {
			return *s.record, nil
		}
		return serving.StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "observation record store has no retained record")
	case "lost":
		return serving.StateObservation{}, kernel.Fail(kernel.ErrTemporaryUnavailable,
			"observation record lost within the promised retention window (recovery fault); source declares latest-only")
	case "expired":
		return serving.StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied,
			"observation retention expired (lifecycle end); source declares latest-only")
	default:
		return serving.StateObservation{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "unknown record state")
	}
}

func TestLatestOnlyRecordLossAndExpiryFailDistinguishably(t *testing.T) {
	base, _, _, address := setupServing(t)
	original := serving.StateObservation{
		Value: map[string]any{"status": "running"},
		Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationLatestOnly, SourceRevision: "r-1", ObservedAt: "2026-08-27T00:00:00Z"},
	}
	lookup := &platformRecordLookup{record: &original, state: "retained"}
	service := liveservice.Open(base, lookup, observability.IdentityContext{Principal: "agent"})

	// 保留期内：重读的是平台保存的原观察（原 basis），不是最新值。
	results, err := service.ReadAddress(context.Background(), address)
	if err != nil {
		t.Fatalf("retained record must re-read: %v", err)
	}
	if len(results) != 1 || len(results[0].Observations) != 1 {
		t.Fatalf("re-read must carry the retained observation: %#v", results)
	}
	if !reflect.DeepEqual(results[0].Value, original.Value) {
		t.Fatalf("re-read must return the retained value: got %#v want %#v", results[0].Value, original.Value)
	}
	if got := results[0].Observations[0].Basis; got != original.Basis {
		t.Fatalf("re-read must return the retained basis: got %#v want %#v", got, original.Basis)
	}

	// 记录丢失=恢复故障：明确失败（TEMPORARY_UNAVAILABLE 类），不返回当前值或空历史。
	lookup.state = "lost"
	results, err = service.ReadAddress(context.Background(), address)
	assertFailedUnitRead(t, results, err, address, kernel.ErrTemporaryUnavailable)
	// 到期=生命周期结束：同样明确失败，但与丢失可区分（CAPABILITY_UNSATISFIED 类）。
	lookup.state = "expired"
	results, err = service.ReadAddress(context.Background(), address)
	assertFailedUnitRead(t, results, err, address, kernel.ErrCapabilityUnsatisfied)
}

// TestBusinessNullContrastsPlaceholderAndNeverFallsBackToScan pins U2: a
// business null carries a successful observation; a value-shaped placeholder
// without a legal basis must fail; and a State lookup failure must never
// degrade into serving the declaration's snapshot bytes as the dynamic value.
func TestBusinessNullContrastsPlaceholderAndNeverFallsBackToScan(t *testing.T) {
	base, _, _, address := setupServing(t)
	lookup := &stateLookup{result: serving.StateObservation{
		Value: nil,
		Basis: objectmodel.ObservationBasis{BindingGeneration: "g1", Consistency: objectmodel.ObservationBounded, SourceRevision: "r-null", ObservedAt: "2026-08-27T00:00:00Z"},
	}}
	service := liveservice.Open(base, lookup, observability.IdentityContext{Principal: "agent"})

	// 业务空值：有成功观察（合法 basis 在场），不是失败也不是占位。
	results, err := service.ReadAddress(context.Background(), address)
	if err != nil {
		t.Fatalf("business null with a valid basis must read successfully: %v", err)
	}
	if len(results) != 1 || results[0].Observations[0].Basis.SourceRevision != "r-null" {
		t.Fatalf("business null must carry its observation basis: %#v", results)
	}

	// 占位空：同样的空值但 basis 非法（缺 observedAt）必须失败关闭。
	lookup.result.Basis.ObservedAt = ""
	results, err = service.ReadAddress(context.Background(), address)
	assertFailedUnitRead(t, results, err, address, kernel.ErrCapabilityUnsatisfied)

	// 不降级为扫描：取值失败时必须返回错误，不得把声明的快照字节当动态值交付。
	lookup.result = serving.StateObservation{}
	lookup.err = kernel.Fail(kernel.ErrTemporaryUnavailable, "runtime unreachable")
	results, err = service.ReadAddress(context.Background(), address)
	assertFailedUnitRead(t, results, err, address, kernel.ErrTemporaryUnavailable)
}
