package evidence

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

func TestCollectorOnlyCapturesAdoptedRequestResults(t *testing.T) {
	if err := Capture(context.Background(), Sample{}); err != nil {
		t.Fatalf("operation without consumer collector tried to record a result: %v", err)
	}
	if values, err := CapturedValues(context.Background()); err != nil || len(values) != 0 {
		t.Fatalf("non-consumer operation acquired adopted results: %#v %v", values, err)
	}
	collector := NewCollector()
	ctx := WithCollector(context.Background(), collector)
	sample := retainedSample("health", map[string]any{"status": "adopted"})
	if err := Capture(ctx, sample); err != nil {
		t.Fatal(err)
	}
	sample.Value.(map[string]any)["status"] = "source-reused-buffer"
	sample.Observation.Basis.SourceRevision = "new-revision"
	values, err := CapturedValues(ctx)
	if err != nil || len(values) != 1 || values[0].Value.(map[string]any)["status"] != "adopted" || values[0].Observation.Basis.SourceRevision != "r1" {
		t.Fatalf("captured result changed after adoption: %#v %v", values, err)
	}
	values[0].Value.(map[string]any)["status"] = "caller-mutated"
	again, err := collector.Values()
	if err != nil || again[0].Value.(map[string]any)["status"] != "adopted" {
		t.Fatalf("collector returned a mutable internal result: %#v %v", again, err)
	}
}

func TestConcurrentCollectorsDoNotMixConsumerRequests(t *testing.T) {
	alice, bob := NewCollector(), NewCollector()
	contexts := []context.Context{WithCollector(context.Background(), alice), WithCollector(context.Background(), bob)}
	var wg sync.WaitGroup
	for i, ctx := range contexts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 8; n++ {
				if err := Capture(ctx, retainedSample("health", float64(i))); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	for i, collector := range []*Collector{alice, bob} {
		values, err := collector.Values()
		if err != nil || len(values) != 8 {
			t.Fatalf("consumer %d lost its own samples: %#v %v", i, values, err)
		}
		for _, sample := range values {
			if sample.Value != float64(i) {
				t.Fatalf("consumer %d acquired another consumer's result: %#v", i, sample)
			}
		}
	}
}

func TestSampleFormatDistinguishesExplicitNullAndMissingValue(t *testing.T) {
	null := retainedSample("optional", nil)
	raw, err := json.Marshal(null)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Sample
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.Outcome != ValueOutcome || decoded.Value != nil || decoded.Observation == nil {
		t.Fatalf("explicit observed null did not round trip: %#v %v", decoded, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "value")
	missing, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(missing, &decoded); err == nil {
		t.Fatal("missing retained value was accepted as an observed null")
	}
}

func TestRecordRejectsContradictoryOrUnmatchedResults(t *testing.T) {
	mutations := map[string]func(*Record){
		"missing request":              func(r *Record) { r.RequestID = "" },
		"missing adopted units":        func(r *Record) { r.Units = nil },
		"missing observation":          func(r *Record) { r.Units[0].Observation = nil },
		"invalid basis":                func(r *Record) { r.Units[0].Observation.Basis.ObservedAt = "" },
		"wrong declaration":            func(r *Record) { r.Units[0].Observation.DeclarationDigest = "other-declaration" },
		"wrong fixed commit":           func(r *Record) { r.Units[0].Observation.DeclarationCommit = "new-commit" },
		"wrong address":                func(r *Record) { r.Units[0].Observation.Address.AspectName = "other-unit" },
		"failure without reason":       func(r *Record) { r.Units[2].Failure = nil },
		"failure invents observation":  func(r *Record) { r.Units[2].Observation = r.Units[0].Observation },
		"failure invents value":        func(r *Record) { r.Units[2].Value = "old-success" },
		"value accompanied by failure": func(r *Record) { r.Units[0].Failure = r.Units[2].Failure },
		"duplicate adopted address":    func(r *Record) { r.Units = append(r.Units, r.Units[0]) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			record := retainedRecord()
			mutate(&record)
			ref, err := retainedStore(t, t.TempDir()).Record(context.Background(), record)
			if err == nil || ref != (Reference{}) {
				t.Fatalf("invalid adopted result produced a durable receipt: %+v %v", ref, err)
			}
		})
	}
}
