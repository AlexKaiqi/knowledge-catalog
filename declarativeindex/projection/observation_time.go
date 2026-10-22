package projection

import (
	objectmodel "kc/object"
	"time"
)

// observationTime preserves the source's timestamp spelling while selecting
// the earliest instant. Invalid dates cannot establish an observed age.
type observationTime struct {
	at  time.Time
	raw string
}

func (oldest *observationTime) include(observations []objectmodel.UnitObservation) {
	for _, observation := range observations {
		raw := observation.Basis.ObservedAt
		at, err := time.Parse(time.RFC3339Nano, raw)
		if err == nil && (oldest.raw == "" || at.Before(oldest.at)) {
			oldest.at, oldest.raw = at, raw
		}
	}
}
