package invocation

// Home aliases keep the moved operation bodies readable without requalifying
// every home call. (CAP-ORG-01 B6.)

import (
	kchome "kc/home"
)

type StoresFile = kchome.StoresFile

type HomeFile = kchome.HomeFile

var NormalizeIndexDriver = kchome.NormalizeIndexDriver

var ReadStores = kchome.ReadStores

var LayoutPath = kchome.LayoutPath

var StoresPath = kchome.StoresPath

var Ready = kchome.Ready

var PersistControl = kchome.PersistControl
