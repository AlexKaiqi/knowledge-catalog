package format

import objectmodel "kc/object"

// ToObjectUnits converts parsed file units to the logical maintenance units
// consumed by Reader, Writer and the Object algebra. Physical Path stays in the
// format layer; this conversion neither validates nor assembles content.
func ToObjectUnits(units []Unit) []objectmodel.Unit {
	out := make([]objectmodel.Unit, 0, len(units))
	for _, unit := range units {
		out = append(out, objectmodel.Unit{
			ObjectID: unit.ObjectID, Address: unit.Address, PathHint: unit.PathHint,
			SchemaRef: unit.SchemaRef, ValueSource: unit.ValueSource,
			Provenance: unit.Provenance, Value: unit.Value, Digest: unit.Digest,
		})
	}
	return out
}
