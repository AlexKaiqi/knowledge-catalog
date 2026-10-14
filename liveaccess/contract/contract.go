package contract

import objectmodel "kc/object"

// LiveAccess is the first independently processed object Interface. Its
// knowledge contract and implementation Schema are published in system repo.
const (
	LiveAccessInterfaceV1            = objectmodel.ObjectID("interface/live-access/v1")
	LiveAccessImplementationSchemaV1 = objectmodel.ObjectID("schema/implements.live-access/v1")
)
