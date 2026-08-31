package retrieval

import (
	"testing"
	"time"
)

func TestSearchExecutionStatsAddPreservesFactsAndStrongestPartialReason(t *testing.T) {
	stats := SearchExecutionStats{Candidates: 2, Hydrated: 1, Dropped: 1, PlanDuration: time.Millisecond, PartialReason: "unsupported"}
	stats.Add(SearchExecutionStats{
		Candidates: 3, Hydrated: 2, Dropped: 1, DroppedAuthorization: 1,
		ProbeDuration: 2 * time.Millisecond, HydrateDuration: 3 * time.Millisecond,
		PartialReason: "authorization",
	})
	if stats.Candidates != 5 || stats.Hydrated != 3 || stats.Dropped != 2 || stats.DroppedAuthorization != 1 {
		t.Fatalf("counts were not aggregated: %#v", stats)
	}
	if stats.PlanDuration != time.Millisecond || stats.ProbeDuration != 2*time.Millisecond || stats.HydrateDuration != 3*time.Millisecond {
		t.Fatalf("phase durations were not aggregated: %#v", stats)
	}
	if stats.PartialReason != "authorization" {
		t.Fatalf("partial reason priority was not preserved: %#v", stats)
	}
}
