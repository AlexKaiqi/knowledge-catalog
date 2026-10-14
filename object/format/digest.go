package format

import (
	"kc/kernel"
	objectmodel "kc/object"
)

func DeclarationOf(unit Unit) objectmodel.UnitDeclaration {
	return objectmodel.DeclareUnits(ToObjectUnits([]Unit{unit}))[0]
}

func Declarations(units []Unit) []objectmodel.UnitDeclaration {
	return objectmodel.DeclareUnits(ToObjectUnits(units))
}

func TreeDeclarationDigest(units []Unit) kernel.Digest {
	return objectmodel.UnitSetDeclarationDigest(ToObjectUnits(units))
}
