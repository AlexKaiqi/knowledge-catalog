package format

import (
	objectmodel "kc/object"
)

func Assemble(units []Unit) (any, error) {
	return objectmodel.AssembleUnits(ToObjectUnits(units))
}
