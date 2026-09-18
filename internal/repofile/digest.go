package repofile

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/unitcodec"
)

func TreeDigest(units []Unit) kernel.Digest {
	rows := make([]any, 0, len(units))
	for _, u := range units {
		rows = append(rows, map[string]any{"k": knowledge.AddressKey(u.Address), "d": string(u.Digest)})
	}
	return kernel.CanonicalDigest(rows)
}

func DeclarationOf(unit Unit) knowledge.UnitDeclaration {
	return unitcodec.Declarations(coreUnits([]Unit{unit}))[0]
}

func Declarations(units []Unit) []knowledge.UnitDeclaration {
	return unitcodec.Declarations(coreUnits(units))
}

func TreeDeclarationDigest(units []Unit) kernel.Digest {
	return unitcodec.DeclarationDigest(coreUnits(units))
}
