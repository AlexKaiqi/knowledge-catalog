package cli

import (
	"strings"
	"testing"
)

// Plugin-extension's description path starts in ordinary offline task help;
// reading a Schema must not inject deployment hints into its JSON result.
func TestPluginHelpNavigationIsAvailableWithoutServer(t *testing.T) {
	t.Setenv("KC_SERVER_URL", "http://127.0.0.1:1")
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"root", []string{"help"}, "kc plugin"},
		{"schema", []string{"help", "schema", "describe"}, "kc plugin declarative-index help"},
		{"overview", []string{"help", "operations", "access-spec", "describe"}, "kc plugin"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := Run(test.args)
			if result.Status != 0 || !strings.Contains(result.Stdout, test.want) {
				t.Fatalf("offline help lacks plugin navigation %q: status=%d\n%s", test.want, result.Status, result.Stdout)
			}
		})
	}
}
