package datasets

import (
	"context"
	"sort"

	"kc/kernel"
	"kc/snapshot/dataset"
)

// fakeAcceptedReleases is the accepted-release view the build pass consumes:
// which releases are current, one frozen version by coordinate, and the current
// release of one Dataset. Everything else on dataset.Service stays unused, so
// the pass's decisions can be observed without publishing files.
type fakeAcceptedReleases struct {
	current    map[string]dataset.Dataset
	versions   map[string]map[int]dataset.Dataset
	probes     []dataset.PublicationLookup
	versionErr error
	setErr     error
}

func newFakeAcceptedReleases(accepted ...dataset.Dataset) *fakeAcceptedReleases {
	f := &fakeAcceptedReleases{current: map[string]dataset.Dataset{}, versions: map[string]map[int]dataset.Dataset{}}
	for _, def := range accepted {
		f.accept(def)
	}
	return f
}

// accept records an immutable version and advances latest, which is exactly
// what publication does to the accepted view.
func (f *fakeAcceptedReleases) accept(def dataset.Dataset) {
	if f.versions[def.SetID] == nil {
		f.versions[def.SetID] = map[int]dataset.Dataset{}
	}
	f.versions[def.SetID][def.Revision] = def
	if current, ok := f.current[def.SetID]; !ok || def.Revision > current.Revision {
		f.current[def.SetID] = def
	}
}

func (f *fakeAcceptedReleases) DumpState() dataset.State {
	state := dataset.State{Datasets: make([]dataset.Dataset, 0, len(f.current))}
	for _, def := range f.current {
		state.Datasets = append(state.Datasets, def)
	}
	return state
}

func (f *fakeAcceptedReleases) DatasetVersion(setID string, revision int) (dataset.Dataset, error) {
	if f.versionErr != nil {
		return dataset.Dataset{}, f.versionErr
	}
	// Retirement is current policy rather than versioned content, so a retired
	// Dataset resolves no version at all, exactly as the accepted service does.
	if current, ok := f.current[setID]; !ok || current.Retired {
		return dataset.Dataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "dataset %s is absent or retired", setID)
	}
	versions, ok := f.versions[setID]
	if !ok {
		return dataset.Dataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "unknown dataset %s", setID)
	}
	def, ok := versions[revision]
	if !ok {
		return dataset.Dataset{}, kernel.Fail(kernel.ErrVersionUnresolved, "dataset %s has no v%d", setID, revision)
	}
	return def, nil
}

func (f *fakeAcceptedReleases) Set(setID string) (dataset.Dataset, error) {
	if f.setErr != nil {
		return dataset.Dataset{}, f.setErr
	}
	def, ok := f.current[setID]
	if !ok {
		return dataset.Dataset{}, kernel.Fail(kernel.ErrDatasetInvalid, "unknown dataset %s", setID)
	}
	return def, nil
}

// recordingBuild records the releases the pass actually built.
type recordingBuild struct {
	built []dataset.PublicationLookup
	fail  map[dataset.PublicationLookup]error
}

func (b *recordingBuild) build(_ context.Context, def dataset.Dataset) error {
	key := dataset.PublicationLookup{SetID: def.SetID, Revision: def.Revision}
	b.built = append(b.built, key)
	return b.fail[key]
}

func acceptedRelease(setID string, revision int, repositories ...kernel.RepositoryID) dataset.Dataset {
	def := dataset.Dataset{SetID: setID, Revision: revision}
	for _, repository := range repositories {
		def.Sources = append(def.Sources, dataset.DatasetSource{Repository: repository, Selector: "refs/heads/main"})
	}
	return def
}

func sameReleases(got []dataset.PublicationLookup, want ...dataset.PublicationLookup) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// sameReleaseSet compares probes without order: the accepted view is a map, so
// the listing pass visits releases in no guaranteed order.
func sameReleaseSet(got []dataset.PublicationLookup, want ...dataset.PublicationLookup) bool {
	if len(got) != len(want) {
		return false
	}
	normalize := func(in []dataset.PublicationLookup) []dataset.PublicationLookup {
		out := append([]dataset.PublicationLookup(nil), in...)
		sort.Slice(out, func(i, j int) bool {
			if out[i].SetID != out[j].SetID {
				return out[i].SetID < out[j].SetID
			}
			return out[i].Revision < out[j].Revision
		})
		return out
	}
	return sameReleases(normalize(got), normalize(want)...)
}
