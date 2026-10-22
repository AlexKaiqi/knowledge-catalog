package config

import "testing"

func TestAdmissionConfigOnlyDeclaresExternalRequestURL(t *testing.T) {
	base := DeploymentConfig{Registry: RegistryBinding{ID: "kr://platform/catalog"}}
	base.Admission = &AdmissionConfig{}
	if err := ValidateAdmissionConfig(base.Admission); err == nil {
		t.Fatal("empty admission block must fail closed")
	}
	base.Admission = &AdmissionConfig{RequestURL: "https://itsm.example/kc-access"}
	if err := ValidateAdmissionConfig(base.Admission); err != nil {
		t.Fatal(err)
	}
}
