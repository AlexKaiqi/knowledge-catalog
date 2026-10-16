package service

import (
	retrieval "kc/discovery"
	"kc/object/reader"
)

// datasetBasis reports a named Dataset's revision; temporary definitions have
// no accepted revision to pass on.
func datasetBasis(pin reader.DatasetPin) *retrieval.DatasetBasis {
	if pin.SetID == "" || pin.Revision <= 0 {
		return nil
	}
	return &retrieval.DatasetBasis{ID: pin.SetID, Revision: pin.Revision}
}
