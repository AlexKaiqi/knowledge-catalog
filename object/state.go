package object

import (
	"sort"

	"kc/kernel"
)

// LocalStateAlgorithm identifies the equality relation, independently of
// maintenance preconditions, source coordinates and interpretation versions.
const LocalStateAlgorithm = "kc-object-local-state/v2"

// ReadState summarizes local persistence inside one logical selection. Digest
// is absent when the scope could not be proved complete.
type ReadState struct {
	Algorithm string        `json:"algorithm"`
	Scope     kernel.Digest `json:"scope"`
	Digest    kernel.Digest `json:"digest,omitempty"`
	Complete  bool          `json:"complete"`
}

// LocalStateDigest compares complete selected units before field projection
// or external observations. The caller owns scope and completeness checks.
// An empty, proven scope has a digest too; unknown input must not call this.
func LocalStateDigest(declarations []UnitDeclaration) kernel.Digest {
	rows := make([]localStateUnit, 0, len(declarations))
	for _, declaration := range declarations {
		rows = append(rows, localStateUnit{
			Address: AddressKey(declaration.Address), Kind: declaration.Address.Kind, Value: declaration.Digest,
			Declaration: DeclarationDigest(declaration.SchemaRef, declaration.ValueSource),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Address < rows[j].Address })
	return kernel.CanonicalDigest(struct {
		Algorithm string           `json:"algorithm"`
		Units     []localStateUnit `json:"units"`
	}{LocalStateAlgorithm, rows})
}

type localStateUnit struct {
	Kind        AddressKind   `json:"kind"`
	Address     string        `json:"address"`
	Value       kernel.Digest `json:"value"`
	Declaration kernel.Digest `json:"declaration"`
}
