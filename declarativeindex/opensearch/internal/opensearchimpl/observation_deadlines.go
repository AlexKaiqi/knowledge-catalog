package opensearchimpl

import "time"

// Carry the original replacement deadlines in the very same CAS as the new
// READY pointer. A failed candidate has no authority to expire its predecessor.
// These are admission deadlines, not permission to delete physical resources:
// already admitted requests retain their separate bounded resource claims.
func withObservationDeadlines(previous, next controlDoc, now time.Time) controlDoc {
	var deadlines map[string]int64
	for generation, until := range previous.ObservationDeadlines {
		if until > now.UnixMilli() {
			if deadlines == nil {
				deadlines = map[string]int64{}
			}
			deadlines[generation] = until
		}
	}
	if previous.Cause == "observation" && previous.Generation != "" && previous.Generation != next.Generation {
		if deadlines == nil {
			deadlines = map[string]int64{}
		}
		// An existing deadline is immutable even across retries or later refreshes.
		if original, exists := previous.ObservationDeadlines[previous.Generation]; exists {
			if original > now.UnixMilli() {
				deadlines[previous.Generation] = original
			}
		} else {
			deadlines[previous.Generation] = now.Add(continuationGrace).UnixMilli()
		}
	}
	next.ObservationDeadlines = deadlines
	return next
}
