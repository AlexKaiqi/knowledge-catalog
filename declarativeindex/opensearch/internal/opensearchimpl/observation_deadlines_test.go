package opensearchimpl

import (
	"encoding/json"
	"testing"
	"time"
)

func TestObservationReplacementDeadlinesPersistWithoutRenewal(t *testing.T) {
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	original := controlDoc{Generation: "first", Cause: "observation"}
	candidate := withObservationDeadlines(original, controlDoc{Generation: "second", Cause: "observation"}, now)
	until := now.Add(continuationGrace).UnixMilli()
	if candidate.ObservationDeadlines["first"] != until || len(original.ObservationDeadlines) != 0 {
		t.Fatal("candidate mutated predecessor or omitted original deadline")
	}
	raw, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	var reopened controlDoc
	if err := json.Unmarshal(raw, &reopened); err != nil {
		t.Fatal(err)
	}
	third := withObservationDeadlines(reopened, controlDoc{Generation: "third", Cause: "observation"}, now.Add(time.Minute))
	if third.ObservationDeadlines["first"] != until || third.ObservationDeadlines["second"] != now.Add(time.Minute+continuationGrace).UnixMilli() {
		t.Fatalf("replacement renewed or lost deadline: %v", third.ObservationDeadlines)
	}
	third.ObservationDeadlines["first"]++
	if reopened.ObservationDeadlines["first"] != until {
		t.Fatal("next generation aliases durable predecessor")
	}
	expired := withObservationDeadlines(reopened, controlDoc{Generation: "second", Cause: "observation"}, now.Add(continuationGrace))
	if len(expired.ObservationDeadlines) != 0 {
		t.Fatalf("expired deadline retained or recreated: %v", expired.ObservationDeadlines)
	}
}

func TestObservationDeadlineDoesNotExpireUnreplacedOrStaticGeneration(t *testing.T) {
	now := time.Now()
	for _, old := range []controlDoc{{Generation: "same", Cause: "observation"}, {Generation: "other", Cause: "snapshot"}, {}} {
		next := withObservationDeadlines(old, controlDoc{Generation: "same"}, now)
		if len(next.ObservationDeadlines) != 0 {
			t.Fatalf("unreplaced/static generation got deadline: %v", next)
		}
	}
}
