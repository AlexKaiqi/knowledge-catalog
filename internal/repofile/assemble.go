package repofile

import (
	"kc/knowledge/unitcodec"
)

func Assemble(units []Unit) (any, error) {
	return unitcodec.Assemble(coreUnits(units))
}

func coreUnits(units []Unit) []unitcodec.Unit {
	out := make([]unitcodec.Unit, 0, len(units))
	for _, unit := range units {
		out = append(out, unitcodec.Unit{
			ObjectID: unit.ObjectID, Address: unit.Address, PathHint: unit.PathHint,
			SchemaRef: unit.SchemaRef, ValueSource: unit.ValueSource,
			Provenance: unit.Provenance, Value: unit.Value, Digest: unit.Digest,
		})
	}
	return out
}
