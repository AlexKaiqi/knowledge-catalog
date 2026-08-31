package retrieval

import "time"

// SearchExecutionStats contains provider/executor facts used by runtime
// telemetry. It is deliberately excluded from the public SEARCH protocol:
// metrics may be sampled or dropped and are never part of canonical results.
type SearchExecutionStats struct {
	Candidates           int           `json:"-"`
	Hydrated             int           `json:"-"`
	Dropped              int           `json:"-"`
	DroppedAuthorization int           `json:"-"`
	PlanDuration         time.Duration `json:"-"`
	ProbeDuration        time.Duration `json:"-"`
	HydrateDuration      time.Duration `json:"-"`
	PartialReason        string        `json:"-"`
}

func (s *SearchExecutionStats) Add(other SearchExecutionStats) {
	s.Candidates += other.Candidates
	s.Hydrated += other.Hydrated
	s.Dropped += other.Dropped
	s.DroppedAuthorization += other.DroppedAuthorization
	s.PlanDuration += other.PlanDuration
	s.ProbeDuration += other.ProbeDuration
	s.HydrateDuration += other.HydrateDuration
	s.MarkPartial(other.PartialReason)
}

func (s *SearchExecutionStats) MarkPartial(reason string) {
	if partialReasonPriority(reason) > partialReasonPriority(s.PartialReason) {
		s.PartialReason = reason
	}
}

func partialReasonPriority(reason string) int {
	switch reason {
	case "authorization":
		return 6
	case "projection":
		return 5
	case "hydrate":
		return 4
	case "binding":
		return 3
	case "unsupported":
		return 2
	case "other":
		return 1
	default:
		return 0
	}
}
