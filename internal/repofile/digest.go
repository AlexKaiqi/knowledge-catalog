package repofile

import (
	"kc/kernel"
	"kc/knowledge"
)

func TreeDigest(units []Unit) kernel.Digest {
	rows := make([]any, 0, len(units))
	for _, u := range units {
		rows = append(rows, map[string]any{"k": knowledge.AddressKey(u.Address), "d": string(u.Digest)})
	}
	return kernel.CanonicalDigest(rows)
}

func DeclarationOf(unit Unit) knowledge.UnitDeclaration {
	return knowledge.UnitDeclaration{
		Address: unit.Address, Digest: unit.Digest,
		DeclarationDigest: knowledge.DeclarationDigest(unit.SchemaRef, unit.ValueSource),
		SchemaRef:         unit.SchemaRef, ValueSource: unit.ValueSource,
	}
}

func Declarations(units []Unit) []knowledge.UnitDeclaration {
	out := make([]knowledge.UnitDeclaration, 0, len(units))
	for _, unit := range units {
		out = append(out, DeclarationOf(unit))
	}
	return out
}

func TreeDeclarationDigest(units []Unit) kernel.Digest {
	rows := make([]any, 0, len(units))
	for _, unit := range units {
		rows = append(rows, map[string]any{
			"address": knowledge.AddressKey(unit.Address),
			"digest":  knowledge.DeclarationDigest(unit.SchemaRef, unit.ValueSource),
		})
	}
	return kernel.CanonicalDigest(rows)
}
