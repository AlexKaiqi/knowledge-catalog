// Package space fixes the files and declaration namespace of one Dataset
// revision before the index plugin builds or queries any of its sources.
package space

import (
	"sort"
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
	"kc/object/maintenance"
	"kc/object/reader"
)

// Member keeps the source coordinates used for canonical reads while giving
// its index compiler the Schemas and index members selected by this Dataset.
// A declaration held in another source is visible only through this frozen
// namespace. The derived engine identity includes the complete file scope.
func Member(lookup reader.MemberLookup, pin reader.DatasetPin, id kernel.RepositoryID) (objectmodel.Repository, error) {
	serving := reader.Open(lookup, pin)
	pin = serving.Pin()
	selected, err := serving.Member(id)
	if err != nil {
		return nil, err
	}
	raw, err := lookup(id)
	if err != nil {
		return nil, err
	}
	return &repository{Repository: selected, raw: raw, lookup: lookup, serving: serving, pin: pin, commit: pin.Repositories[id]}, nil
}

func (r *repository) ScanSnapshotPage(at kernel.CommitID, request maintenance.ScanRequest) (maintenance.ScanPage, error) {
	if at != r.commit {
		return maintenance.ScanPage{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "commit is outside Dataset revision")
	}
	scanner, err := maintenance.RequireScanner(r.raw)
	if err != nil {
		return maintenance.ScanPage{}, err
	}
	page, err := scanner.ScanSnapshotPage(at, request)
	if err != nil {
		return page, err
	}
	selected := make([]objectmodel.ObjectValue, 0, len(page.Values))
	for _, value := range page.Values {
		value, err := r.Read(value.Address.ObjectID, at)
		if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
			continue
		} else if err != nil {
			return maintenance.ScanPage{}, err
		}
		selected = append(selected, value)
	}
	page.Values = selected
	return page, nil
}

// DatasetScope is the already fixed input for the plugin's Dataset ports.
func (r *repository) DatasetScope() (reader.MemberLookup, reader.DatasetPin) {
	return r.lookup, r.serving.Pin()
}

func (r *repository) BindingSchemaObjectIDs(at kernel.CommitID) ([]objectmodel.ObjectID, error) {
	locator, ok := r.Repository.(objectmodel.BindingLocator)
	if !ok {
		return nil, nil
	}
	ids, err := locator.BindingSchemaObjectIDs(at)
	if kernel.CodeOf(err) == kernel.ErrCapabilityUnsatisfied {
		// A source's complete empty binding namespace proves there is no
		// external field even when a test/native wrapper cannot forward the
		// Reader-private scoped coordinates. Nonempty remains fail closed.
		if raw, ok := r.raw.(objectmodel.BindingLocator); ok {
			all, rawErr := raw.BindingSchemaObjectIDs(at)
			if rawErr == nil && len(all) == 0 {
				return nil, nil
			}
		}
	}
	return ids, err
}

type repository struct {
	objectmodel.Repository
	raw     objectmodel.Repository
	lookup  reader.MemberLookup
	serving *reader.Serving
	pin     reader.DatasetPin
	commit  kernel.CommitID
}

func (r *repository) StoreDigest() kernel.Digest {
	var source kernel.Digest
	if identified, ok := r.raw.(objectmodel.StoreIdentified); ok {
		source = identified.StoreDigest()
	}
	return kernel.CanonicalDigest([]any{source, r.pin})
}

func (r *repository) namespace(id objectmodel.ObjectID, address objectmodel.Address, at kernel.CommitID) (objectmodel.Repository, kernel.CommitID, error) {
	if at != r.commit {
		return nil, "", kernel.Fail(kernel.ErrObjectRefUnresolved, "commit is outside Dataset revision")
	}
	ids := make([]kernel.RepositoryID, 0, len(r.pin.Repositories))
	for id := range r.pin.Repositories {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, source := range ids {
		member, err := r.serving.Member(source)
		if err != nil {
			return nil, "", err
		}
		resolved, err := member.ResolveAddress(address, r.pin.Repositories[source])
		if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		if resolved.Status == objectmodel.StatusUnresolved || resolved.Status == objectmodel.StatusRemoved {
			continue
		}
		return member, r.pin.Repositories[source], nil
	}
	return nil, "", kernel.Fail(kernel.ErrObjectRefUnresolved, "object %s is outside Dataset declaration namespace", id)
}

func isNamespace(id objectmodel.ObjectID, address objectmodel.Address) bool {
	return objectmodel.IsSchemaObject(id) || address.AspectName == "index"
}

func (r *repository) Read(id objectmodel.ObjectID, at kernel.CommitID) (objectmodel.ObjectValue, error) {
	if !objectmodel.IsSchemaObject(id) {
		value, err := r.Repository.Read(id, at)
		if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
			return value, err
		}
		paths, err := r.selectedPaths(id, at)
		if err != nil {
			return objectmodel.ObjectValue{}, err
		}
		hydrator, ok := r.raw.(objectmodel.UnitPathsHydrator)
		if !ok {
			return objectmodel.ObjectValue{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset source cannot read selected object units")
		}
		values, err := hydrator.ReadManyAtPaths([]objectmodel.ObjectID{id}, at, map[objectmodel.ObjectID][]string{id: paths})
		if err != nil {
			return objectmodel.ObjectValue{}, err
		}
		value, ok = values[id]
		if !ok {
			return objectmodel.ObjectValue{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "object has no selected Dataset units")
		}
		return value, nil
	}
	member, commit, err := r.namespace(id, objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: id}, at)
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	return member.Read(id, commit)
}

func (r *repository) Resolve(id objectmodel.ObjectID, at kernel.CommitID) (objectmodel.Resolution, error) {
	value, err := r.Repository.Resolve(id, at)
	if kernel.CodeOf(err) != kernel.ErrObjectRefUnresolved {
		return value, err
	}
	if _, err := r.selectedPaths(id, at); err != nil {
		return objectmodel.Resolution{}, err
	}
	return r.raw.Resolve(id, at)
}

func (r *repository) selectedPaths(id objectmodel.ObjectID, at kernel.CommitID) ([]string, error) {
	if at != r.commit {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "commit is outside Dataset revision")
	}
	locator, ok := r.raw.(objectmodel.UnitLocator)
	if !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset source cannot locate selected units")
	}
	paths, err := locator.ObjectUnitPaths(id, at)
	if err != nil {
		return nil, err
	}
	selected := []string{}
	for _, path := range paths {
		for _, item := range r.pin.Items {
			if item.Repository != r.ID() {
				continue
			}
			prefix := strings.Trim(item.Prefix, "/")
			if item.Kind == "file" && strings.Trim(item.File, "/") == path || item.Kind == "prefix" && (prefix == "" || path == prefix || strings.HasPrefix(path, prefix+"/")) {
				selected = append(selected, path)
				break
			}
		}
	}
	if len(selected) == 0 {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "object has no selected Dataset units")
	}
	return selected, nil
}

func (r *repository) ReadAddress(address objectmodel.Address, at kernel.CommitID) (objectmodel.ObjectValue, error) {
	if !isNamespace(address.ObjectID, address) {
		return r.Repository.ReadAddress(address, at)
	}
	member, commit, err := r.namespace(address.ObjectID, address, at)
	if err != nil {
		return objectmodel.ObjectValue{}, err
	}
	return member.ReadAddress(address, commit)
}

func (r *repository) ResolveAddress(address objectmodel.Address, at kernel.CommitID) (objectmodel.Resolution, error) {
	if !isNamespace(address.ObjectID, address) {
		return r.Repository.ResolveAddress(address, at)
	}
	member, commit, err := r.namespace(address.ObjectID, address, at)
	if err != nil {
		return objectmodel.Resolution{}, err
	}
	return member.ResolveAddress(address, commit)
}

func (r *repository) SchemaObjectIDs(at kernel.CommitID) ([]objectmodel.ObjectID, error) {
	if at != r.commit {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "commit is outside Dataset revision")
	}
	set := map[objectmodel.ObjectID]bool{}
	for id, commit := range r.pin.Repositories {
		member, err := r.serving.Member(id)
		if err != nil {
			return nil, err
		}
		locator, ok := member.(objectmodel.SchemaStore)
		if !ok {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset source cannot locate Schemas")
		}
		ids, err := locator.SchemaObjectIDs(commit)
		if err != nil {
			return nil, err
		}
		for _, schema := range ids {
			set[schema] = true
		}
	}
	out := make([]objectmodel.ObjectID, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// Declaration enumeration is used only by explicit maintenance. Query-side
// compilation instead verifies the exact addresses retained in projection Meta.
func (r *repository) SchemaReferrerAddresses(schema objectmodel.ObjectID, at kernel.CommitID) ([]objectmodel.Address, error) {
	if at != r.commit {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "commit is outside Dataset revision")
	}
	out := []objectmodel.Address{}
	for id, commit := range r.pin.Repositories {
		raw, err := r.lookup(id)
		if err != nil {
			return nil, err
		}
		locator, ok := raw.(objectmodel.SchemaReferrerLocator)
		if !ok {
			return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset source cannot locate declarations")
		}
		addresses, err := locator.SchemaReferrerAddresses(schema, commit)
		if err != nil {
			return nil, err
		}
		member, err := r.serving.Member(id)
		if err != nil {
			return nil, err
		}
		for _, address := range addresses {
			if schema != objectmodel.IndexDeclarationSchemaV1 && id != r.ID() {
				continue
			}
			if _, err := member.ReadAddress(address, commit); kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
				continue
			} else if err != nil {
				return nil, err
			}
			out = append(out, address)
		}
	}
	sort.Slice(out, func(i, j int) bool { return objectmodel.AddressKey(out[i]) < objectmodel.AddressKey(out[j]) })
	return out, nil
}

func (r *repository) ReadMany(ids []objectmodel.ObjectID, at kernel.CommitID) (map[objectmodel.ObjectID]objectmodel.ObjectValue, error) {
	if at != r.commit {
		return nil, kernel.Fail(kernel.ErrObjectRefUnresolved, "commit is outside Dataset revision")
	}
	type batchGroup struct {
		member objectmodel.Repository
		commit kernel.CommitID
		ids    []objectmodel.ObjectID
	}
	groups := map[kernel.RepositoryID]*batchGroup{}
	for _, id := range ids {
		member, commit := r.Repository, at
		if objectmodel.IsSchemaObject(id) {
			var err error
			member, commit, err = r.namespace(id, objectmodel.Address{Kind: objectmodel.KindEntity, ObjectID: id}, at)
			if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
				continue
			}
			if err != nil {
				return nil, err
			}
		}
		group := groups[member.ID()]
		if group == nil {
			group = &batchGroup{member: member, commit: commit}
			groups[member.ID()] = group
		}
		group.ids = append(group.ids, id)
	}
	out := map[objectmodel.ObjectID]objectmodel.ObjectValue{}
	for _, group := range groups {
		values := map[objectmodel.ObjectID]objectmodel.ObjectValue{}
		if batch, ok := group.member.(objectmodel.BatchReadStore); ok {
			var err error
			values, err = batch.ReadMany(group.ids, group.commit)
			if err != nil {
				return nil, err
			}
		}
		for _, id := range group.ids {
			value, found := values[id]
			if !found {
				var err error
				if objectmodel.IsSchemaObject(id) {
					value, err = group.member.Read(id, group.commit)
				} else {
					value, err = r.Read(id, at)
				}
				if kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
					continue
				}
				if err != nil {
					return nil, err
				}
			}
			out[id] = value
		}
	}
	return out, nil
}

func (r *repository) ObjectIDsPage(at kernel.CommitID, limit int, continuation string) (objectmodel.ObjectIDPage, error) {
	if at != r.commit {
		return objectmodel.ObjectIDPage{}, kernel.Fail(kernel.ErrObjectRefUnresolved, "commit is outside Dataset revision")
	}
	pager, ok := r.raw.(objectmodel.SnapshotObjectPager)
	if !ok {
		return objectmodel.ObjectIDPage{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "Dataset source cannot enumerate maintenance identities")
	}
	page, err := pager.ObjectIDsPage(at, limit, continuation)
	if err != nil {
		return page, err
	}
	allowed := page.ObjectIDs[:0]
	for _, id := range page.ObjectIDs {
		if _, err := r.Repository.Resolve(id, at); kernel.CodeOf(err) == kernel.ErrObjectRefUnresolved {
			continue
		} else if err != nil {
			return objectmodel.ObjectIDPage{}, err
		}
		allowed = append(allowed, id)
	}
	page.ObjectIDs = allowed
	return page, nil
}
