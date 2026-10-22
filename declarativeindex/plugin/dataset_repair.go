package plugin

import (
	"bytes"
	"context"
	"errors"

	projection "kc/declarativeindex/projection"
	discoverydatasets "kc/discovery/datasets"
	"kc/kernel"
	"kc/object/reader"
	"kc/snapshot/dataset"
)

// File authority selects fixed accepted definitions. The existing composition
// root supplies its one Dataset-to-reader conversion; no fourth mapping lives
// in this capability's assembly package.
func recoverPublishedDatasets(ctx context.Context, idx *projection.Index, options Options, publication discoverydatasets.PublicationRecovery, lookup reader.MemberLookup, target dataset.PublicationLookup) error {
	var definitions []dataset.Dataset
	if target.SetID != "" {
		definition, err := options.Datasets.DatasetVersion(target.SetID, target.Revision)
		// Preparation also serves not-yet-published candidates. Only existing
		// accepted releases have a preparation receipt to adopt or repair.
		if kernel.CodeOf(err) == kernel.ErrDatasetInvalid || kernel.CodeOf(err) == kernel.ErrVersionUnresolved {
			return nil
		}
		if err != nil {
			return err
		}
		definitions = []dataset.Dataset{definition}
	} else {
		state := options.Datasets.DumpState()
		definitions = append(append([]dataset.Dataset(nil), state.DatasetVersions...), state.Datasets...)
	}
	seen := map[dataset.PublicationLookup]bool{}
	var joined error
	for _, definition := range definitions {
		if err := ctx.Err(); err != nil {
			return errors.Join(joined, err)
		}
		key := dataset.PublicationLookup{SetID: definition.SetID, Revision: definition.Revision}
		if seen[key] {
			continue
		}
		seen[key] = true
		accepted, err := options.Datasets.DatasetVersion(key.SetID, key.Revision)
		if kernel.CodeOf(err) == kernel.ErrDatasetInvalid {
			continue
		} // Entire retired Dataset.
		if err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		resolved, err := options.Datasets.ResolveDefinition(accepted)
		if err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		evidence, err := publication.Evidence(ctx, key.SetID, key.Revision)
		if err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		if len(evidence) == 0 {
			err := publication.Adopt(ctx, key, func(ctx context.Context, definition dataset.Dataset) (discoverydatasets.PublicationProtection, error) {
				fixed, err := options.Datasets.ResolveDefinition(definition)
				if err != nil {
					return nil, err
				}
				return idx.PrepareLegacyDatasetResource(ctx, lookup, options.DatasetPin(fixed))
			})
			joined = errors.Join(joined, err)
			continue
		}
		err = idx.RepairDatasetPublication(ctx, lookup, options.DatasetPin(resolved), func(apply func() error) error {
			current, err := options.Datasets.Set(key.SetID)
			if err != nil {
				return err
			}
			return options.Datasets.WithCurrentRevision(key.SetID, current.Revision, func() error {
				// The current-revision fence serializes all writes of this Dataset,
				// including publication/retirement while a retained target is accepted.
				actual, err := publication.Evidence(ctx, key.SetID, key.Revision)
				if err != nil {
					return err
				}
				if !bytes.Equal(actual, evidence) {
					return kernel.Fail(kernel.ErrPreconditionFailed, "Dataset preparation identity changed during repair")
				}
				return apply()
			})
		})
		joined = errors.Join(joined, err)
	}
	return joined
}
