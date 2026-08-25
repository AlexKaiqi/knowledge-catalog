// Package repofile is the on-disk knowledge unit format used by Snapshot adapters.
// It is not a store.
package repofile

import (
	"kc/kernel"
	"kc/knowledge"
)

// Unit is one aspect/entity file in a snapshot tree.
type Unit struct {
	ObjectID       knowledge.ObjectID            `json:"objectId"`
	Address        knowledge.Address             `json:"address"`
	PathHint       string                        `json:"pathHint"`
	SchemaRef      string                        `json:"schemaRef,omitempty"`
	ValueSource    *knowledge.ValueSource        `json:"valueSource,omitempty"`
	Provenance     *knowledge.ProvenanceEnvelope `json:"provenance,omitempty"`
	Value          any                           `json:"value"`
	Path           string                        `json:"path,omitempty"`
	Digest         kernel.Digest                 `json:"digest,omitempty"`
	declarationErr error
}

// Tree is the assembled snapshot of units at one commit.
type Tree struct {
	Units    map[string]Unit
	ByObject map[knowledge.ObjectID][]Unit
}

func NewTree() *Tree {
	return &Tree{Units: map[string]Unit{}, ByObject: map[knowledge.ObjectID][]Unit{}}
}

func (idx *Tree) Upsert(unit Unit) {
	key := knowledge.AddressKey(unit.Address)
	_, had := idx.Units[key]
	idx.Units[key] = unit
	list := idx.ByObject[unit.Address.ObjectID]
	if had {
		next := list[:0]
		for _, u := range list {
			if knowledge.AddressKey(u.Address) != key {
				next = append(next, u)
			}
		}
		list = append(next, unit)
	} else {
		list = append(list, unit)
	}
	idx.ByObject[unit.Address.ObjectID] = list
}

func (idx *Tree) Remove(address knowledge.Address) *Unit {
	key := knowledge.AddressKey(address)
	prev, ok := idx.Units[key]
	if !ok {
		return nil
	}
	delete(idx.Units, key)
	list := idx.ByObject[address.ObjectID]
	next := list[:0]
	for _, u := range list {
		if knowledge.AddressKey(u.Address) != key {
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

func (idx *Tree) ObjectUnits(objectID knowledge.ObjectID) []Unit {
	return idx.ByObject[objectID]
}
