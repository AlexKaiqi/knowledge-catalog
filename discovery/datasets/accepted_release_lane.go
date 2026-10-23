package datasets

import (
	"context"
	"errors"
	"sort"

	"kc/kernel"
	"kc/snapshot/dataset"
)

// AcceptedReleaseDemand is the accepted-release view one build pass needs: which
// releases are current, one frozen version by coordinate, and the current
// release of one Dataset. It reads only immutable accepted releases, because an
// accepted release pins its own sources.
type AcceptedReleaseDemand struct {
	Releases interface {
		DumpState() dataset.State
		DatasetVersion(string, int) (dataset.Dataset, error)
		Set(string) (dataset.Dataset, error)
	}
	// Ready reports whether this exact revision already has a prepared corpus.
	// A build that finished is not repeated; unknown or failed stays buildable,
	// so the next pass is the retry path and needs no separate queue.
	Ready func(ctx context.Context, setID string, revision int) (bool, error)
	// Owns filters releases to the ones this lane can build. A deployment may
	// run several instances, and each builds only what its own scope admits;
	// without it every instance would attempt every Dataset.
	Owns func(dataset.Dataset) bool
	// Build builds one accepted release's corpus.
	Build func(context.Context, dataset.Dataset) error
}

// Builds lists accepted releases whose corpus is not ready yet, newest revision
// per Dataset first. A newer accepted release makes an older corpus worthless,
// so only the current release of each Dataset is a target
// (doc/design/dataset.md#发布).
func (d AcceptedReleaseDemand) Builds(ctx context.Context) ([]dataset.PublicationLookup, error) {
	if d.Releases == nil {
		return nil, nil
	}
	current := d.Releases.DumpState().Datasets
	targets := make([]dataset.PublicationLookup, 0, len(current))
	for _, def := range current {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if def.Retired || def.SetID == "" || def.Revision <= 0 {
			continue
		}
		if d.Owns != nil && !d.Owns(def) {
			continue
		}
		if d.Ready != nil {
			ready, err := d.Ready(ctx, def.SetID, def.Revision)
			if err != nil {
				return nil, err
			}
			if ready {
				continue
			}
		}
		targets = append(targets, dataset.PublicationLookup{SetID: def.SetID, Revision: def.Revision})
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].SetID != targets[j].SetID {
			return targets[i].SetID < targets[j].SetID
		}
		return targets[i].Revision < targets[j].Revision
	})
	return targets, nil
}

// CatchUp builds the corpus of every accepted release that is not ready yet.
// One failed build does not stop the others, and it never reverts the accepted
// release: the release simply stays not-ready and is retried by the next pass
// or by explicit preparation.
func (d AcceptedReleaseDemand) CatchUp(ctx context.Context) error {
	if d.Build == nil {
		return nil
	}
	targets, err := d.Builds(ctx)
	if err != nil {
		return err
	}
	var joined error
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return errors.Join(joined, err)
		}
		if err := d.build(ctx, target); err != nil {
			joined = errors.Join(joined, err)
		}
	}
	return joined
}

func (d AcceptedReleaseDemand) build(ctx context.Context, target dataset.PublicationLookup) error {
	def, err := d.Releases.DatasetVersion(target.SetID, target.Revision)
	if kernel.CodeOf(err) == kernel.ErrDatasetInvalid || kernel.CodeOf(err) == kernel.ErrVersionUnresolved {
		// Retired or superseded between listing and building: not a failure.
		return nil
	}
	if err != nil {
		return err
	}
	if d.Owns != nil && !d.Owns(def) {
		return nil
	}
	// A newer release may have been accepted meanwhile; do not build a version
	// the Dataset already moved past.
	current, err := d.Releases.Set(target.SetID)
	if err != nil {
		return err
	}
	if current.Revision != target.Revision {
		return nil
	}
	if d.Ready != nil {
		ready, err := d.Ready(ctx, target.SetID, target.Revision)
		if err != nil || ready {
			return err
		}
	}
	return d.Build(ctx, def)
}
