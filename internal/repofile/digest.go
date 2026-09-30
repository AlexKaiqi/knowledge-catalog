package repofile

import (
	"kc/kernel"
	"kc/knowledge"
	"kc/knowledge/unitcodec"
)

func DeclarationOf(unit Unit) knowledge.UnitDeclaration {
	return unitcodec.Declarations(coreUnits([]Unit{unit}))[0]
}

func Declarations(units []Unit) []knowledge.UnitDeclaration {
	return unitcodec.Declarations(coreUnits(units))
}

func TreeDeclarationDigest(units []Unit) kernel.Digest {
	return unitcodec.DeclarationDigest(coreUnits(units))
}
