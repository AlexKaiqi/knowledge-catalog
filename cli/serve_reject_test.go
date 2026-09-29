package cli

import (
	"strings"
	"testing"
)

func TestServeRequiresExplicitDeploymentConfiguration(t *testing.T) {
	for _, args := range [][]string{{"serve"}, {"serve", "--auth", "local"}} {
		result := Run(args)
		if result.Status == 0 || !strings.Contains(result.Stdout, "--config") {
			t.Fatalf("kc serve without declared configuration: %#v", result)
		}
	}
}
