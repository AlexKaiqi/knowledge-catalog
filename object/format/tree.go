// Package repofile is the on-disk knowledge unit format used by Snapshot adapters.
// It is not a store.
package format

import (
	"kc/kernel"
	objectmodel "kc/object"
)

// Unit is one aspect/entity file in a snapshot tree.
type Unit struct {
	ObjectID       objectmodel.ObjectID            `json:"objectId"`
	Address        objectmodel.Address             `json:"address"`
	PathHint       string                          `json:"pathHint"`
	SchemaRef      string                          `json:"schemaRef,omitempty"`
	ValueSource    *objectmodel.ValueSource        `json:"valueSource,omitempty"`
	Provenance     *objectmodel.ProvenanceEnvelope `json:"provenance,omitempty"`
	Value          any                             `json:"value"`
	Path           string                          `json:"path,omitempty"`
	Digest         kernel.Digest                   `json:"digest,omitempty"`
	declarationErr error
}

func (u *Unit) DeclarationError() error {
	if u == nil {
		return nil
	}
	return u.declarationErr
}

// Tree is the assembled snapshot of units at one commit.
type Tree struct {
	Units    map[string]Unit
	ByObject map[objectmodel.ObjectID][]Unit
}

func NewTree() *Tree {
	return &Tree{Units: map[string]Unit{}, ByObject: map[objectmodel.ObjectID][]Unit{}}
}

func (idx *Tree) Upsert(unit Unit) {
	key := objectmodel.AddressKey(unit.Address)
	_, had := idx.Units[key]
	idx.Units[key] = unit
	list := idx.ByObject[unit.Address.ObjectID]
	if had {
		next := list[:0]
		for _, u := range list {
			if objectmodel.AddressKey(u.Address) != key {
				next = append(next, u)
			}
		}
		list = append(next, unit)
	} else {
		list = append(list, unit)
	}
	idx.ByObject[unit.Address.ObjectID] = list
}

func (idx *Tree) Remove(address objectmodel.Address) *Unit {
	key := objectmodel.AddressKey(address)
	prev, ok := idx.Units[key]
	if !ok {
		return nil
	}
	delete(idx.Units, key)
	list := idx.ByObject[address.ObjectID]
	next := list[:0]
	for _, u := range list {
		if objectmodel.AddressKey(u.Address) != key {
			next = append(next, u)
		}
	}
	if len(next) == 0 {
		delete(idx.ByObject, address.ObjectID)
	} else {
		idx.ByObject[address.ObjectID] = next
	}
	return &prev
}

func (idx *Tree) ObjectUnits(objectID objectmodel.ObjectID) []Unit {
	return idx.ByObject[objectID]
}
