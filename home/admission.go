package home

import (
	"kc/kernel"
)

// AdmissionConfig points users at the deployment's external access workflow.
// KC reports current grants but does not host an application or approval queue.
type AdmissionConfig struct {
	RequestURL string `json:"requestURL,omitempty" yaml:"requestURL,omitempty"`
}

func validateAdmissionConfig(c DeploymentConfig) error {
	p := c.Admission
	if p == nil {
		return nil
	}
	if p.RequestURL == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "admission: requestURL is required")
	}
	return nil
}
