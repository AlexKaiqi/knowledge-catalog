package home

import "testing"

func TestAdmissionConfigOnlyDeclaresExternalRequestURL(t *testing.T) {
	base := DeploymentConfig{Catalogs: []CatalogBinding{{ID: "kr://platform/catalog"}}}
	base.Admission = &AdmissionConfig{}
	if err := validateAdmissionConfig(base); err == nil {
		t.Fatal("empty admission block must fail closed")
	}
	base.Admission = &AdmissionConfig{RequestURL: "https://itsm.example/kc-access"}
	if err := validateAdmissionConfig(base); err != nil {
		t.Fatal(err)
	}
}
