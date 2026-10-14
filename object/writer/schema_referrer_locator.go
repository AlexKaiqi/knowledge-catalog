package writer

import (
	"kc/kernel"
	objectmodel "kc/object"
	repofile "kc/object/format"
	"kc/snapshot"
)

func updateSchemaReferrerLocators(tree snapshot.TreeReader, commit kernel.CommitID, before map[objectmodel.ObjectID][]objectmodel.Address, idx *repofile.Tree, operations []objectmodel.Operation, toWrite map[string]string, toDelete map[string]struct{}) error {
	after := repofile.BuildLocatorManifest(idx).Referrers
	affected := map[objectmodel.ObjectID]struct{}{}
	for id := range before {
		affected[id] = struct{}{}
	}
	for id := range after {
		affected[id] = struct{}{}
	}
	changed := map[objectmodel.ObjectID]bool{}
	for _, id := range changedObjectIDs(operations) {
		changed[id] = true
	}
	_, markerErr := tree.ReadFile(repofile.LocatorReferrersCompletePath, commit)
	if markerErr != nil && kernel.CodeOf(markerErr) != kernel.ErrObjectRefUnresolved {
		return markerErr
	}
	for id := range affected {
		current := []objectmodel.Address{}
		if markerErr == nil {
			var err error
			current, err = repofile.ReadSchemaReferrers(tree, id, commit)
			if err != nil {
				return err
			}
		}
		final := map[string]objectmodel.Address{}
		for _, address := range current {
			if !changed[address.ObjectID] {
				final[objectmodel.AddressKey(address)] = address
			}
		}
		for _, address := range after[id] {
			final[objectmodel.AddressKey(address)] = address
		}
		path := repofile.SchemaReferrersPath(id)
		if len(final) == 0 {
			toDelete[path] = struct{}{}
			continue
		}
		addresses := make([]objectmodel.Address, 0, len(final))
		for _, address := range final {
			addresses = append(addresses, address)
		}
		raw, err := repofile.EncodeSchemaReferrers(id, addresses)
		if err != nil {
			return err
		}
		toWrite[path] = string(raw)
	}
	return nil
}
