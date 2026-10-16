package object

import "kc/kernel"

// MaintenanceDigest is the conditional-write basis for one complete unit.
// Snapshot units retain their content digest. A binding has no stored value,
// so its basis is its local Schema/valueSource declaration, without the
// referenced descriptor's version or any live observation.
func MaintenanceDigest(content kernel.Digest, schemaRef string, source *ValueSource) kernel.Digest {
	if source.IsBinding() {
		return DeclarationDigest(schemaRef, source)
	}
	return content
}

// UnitSetMaintenanceDigest is the conditional-remove basis for a whole object.
// All-Snapshot objects retain the assembled content digest. A mixed object
// includes every complete unit's maintenance basis and Address in stable order.
func UnitSetMaintenanceDigest(units []Unit) (kernel.Digest, error) {
	assembled, err := AssembleUnits(units)
	if err != nil {
		return "", err
	}
	bound := false
	for _, unit := range units {
		bound = bound || unit.ValueSource.IsBinding()
	}
	if !bound {
		return kernel.CanonicalDigest(assembled), nil
	}
	rows := make([]any, 0, len(units))
	for _, declaration := range DeclareUnits(units) {
		rows = append(rows, map[string]any{
			"address": AddressKey(declaration.Address),
			"digest":  MaintenanceDigest(declaration.Digest, declaration.SchemaRef, declaration.ValueSource),
		})
	}
	return kernel.CanonicalDigest(map[string]any{"maintenanceUnits": rows}), nil
}
