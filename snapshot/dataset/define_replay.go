package dataset

import (
	"maps"
	"reflect"
)

// Dataset define dispositions mirror Writer receipts: the first acceptance
// applies, a retry of the same accepted definition replays it.
const (
	DefineApplied  = "APPLIED"
	DefineReplayed = "REPLAYED"
)

// ReplayedDefinition returns the accepted revision when a define request
// repeats exactly the definition already accepted under the same revision
// number: the same requested sources (before freezing) and the same metadata.
// A retry after a lost response then gets the original result instead of
// NON_FAST_FORWARD; any difference still meets the monotonic revision check.
// Both the local and the HTTP define paths call it after authorization.
func ReplayedDefinition(svc Service, setID string, revision int, sources []DatasetSource, metadata map[string]string) (Dataset, bool) {
	accepted, err := svc.DatasetVersion(setID, revision)
	if err != nil {
		return Dataset{}, false
	}
	if (len(accepted.Metadata) > 0 || len(metadata) > 0) && !maps.Equal(accepted.Metadata, metadata) {
		return Dataset{}, false
	}
	if len(accepted.Sources) != len(sources) {
		return Dataset{}, false
	}
	for i, source := range sources {
		// Publication fills an omitted commit, so a branch retry keeps its
		// accepted version even after HEAD advances. An explicit commit is a
		// caller selection and must still name that accepted version.
		if source.Commit != "" && source.Commit != accepted.Sources[i].Commit {
			return Dataset{}, false
		}
	}
	if !reflect.DeepEqual(requestedSources(accepted.Sources), requestedSources(sources)) {
		return Dataset{}, false
	}
	return accepted, true
}

// requestedSources drops what publication adds while freezing, leaving what
// the caller asked for.
func requestedSources(sources []DatasetSource) []DatasetSource {
	out := make([]DatasetSource, len(sources))
	for i, source := range sources {
		source.Commit = ""
		if source.Path != nil {
			path := *source.Path
			source.Path = &path
		}
		out[i] = source
	}
	return out
}
