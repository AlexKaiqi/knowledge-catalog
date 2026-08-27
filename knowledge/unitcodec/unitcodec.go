// Package unitcodec owns provider-neutral unit mutation and assembly. File
// repositories add frontmatter/path serialization around it; native providers
// persist the same units as typed rows.
package unitcodec

import (
	"sort"

	"kc/internal/repofile"
	"kc/kernel"
	"kc/knowledge"
)

type Unit = repofile.Unit
type Tree = repofile.Tree

func NewTree() *Tree { return repofile.NewTree() }

func Assemble(units []Unit) (any, error) { return repofile.Assemble(units) }

func Declarations(units []Unit) []knowledge.UnitDeclaration { return repofile.Declarations(units) }

func DeclarationDigest(units []Unit) kernel.Digest { return repofile.TreeDeclarationDigest(units) }

// Apply mutates only the supplied object working set. Callers load the units
// for object IDs touched by operations; no repository-wide tree is required.
func Apply(existing []Unit, operations []knowledge.Operation, provenance *knowledge.ProvenanceEnvelope) (final []Unit, deleted []knowledge.Address, err error) {
	tree := repofile.NewTree()
	initial := map[string]knowledge.Address{}
	for _, unit := range existing {
		tree.Upsert(unit)
		initial[knowledge.AddressKey(unit.Address)] = unit.Address
	}
	writes := map[string]string{}
	removes := map[string]struct{}{}
	for _, operation := range operations {
		if err := repofile.Apply(tree, operation, provenance, writes, removes); err != nil {
			return nil, nil, err
		}
	}
	for key, address := range initial {
		if _, ok := tree.Units[key]; !ok {
			deleted = append(deleted, address)
		}
	}
	for _, unit := range tree.Units {
		final = append(final, unit)
	}
	sort.Slice(final, func(i, j int) bool {
		return knowledge.AddressKey(final[i].Address) < knowledge.AddressKey(final[j].Address)
	})
	sort.Slice(deleted, func(i, j int) bool {
		return knowledge.AddressKey(deleted[i]) < knowledge.AddressKey(deleted[j])
	})
	return final, deleted, nil
}

func AssembleKnowledgeValue(repository kernel.RepositoryID, objectID knowledge.ObjectID, commit kernel.CommitID, units []Unit) (knowledge.KnowledgeValue, error) {
	assembled, err := Assemble(units)
	if err != nil {
		return knowledge.KnowledgeValue{}, err
	}
	value := knowledge.KnowledgeValue{
		KnowledgeRef: knowledge.KnowledgeRef{Repository: repository, Object: objectID},
		Repository:   repository, Commit: commit,
		Address: knowledge.Address{Kind: knowledge.KindEntity, ObjectID: objectID},
		Value:   assembled, Declarations: Declarations(units),
	}
	if len(units) == 1 {
		value.Provenance = units[0].Provenance
	}
	for _, unit := range units {
		if unit.Address.AspectName == "" {
			continue
		}
		for _, member := range units {
			value.Units = append(value.Units, member.Address)
		}
		break
	}
	return value, nil
}
