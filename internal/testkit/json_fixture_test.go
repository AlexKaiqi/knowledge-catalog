package testkit_test

import (
	"testing"

	"kc/internal/testkit"
	objectmodel "kc/object"
	objectclient "kc/object/client"
)

func TestJSONFixturesKeepPublicObjectInputContracts(t *testing.T) {
	const reference = "urn:kc:object:kr%3A%2F%2Fscene%2Fobject:metric%2Fgmv"
	for _, tc := range []struct {
		name, task string
		payload    map[string]any
		reject     bool
	}{
		{"read root", "read", map[string]any{"dataset": "scene-set", "reference": reference}, false},
		{"read selected aspect", "read", map[string]any{"reference": reference + "#/definition", "commit": "fixed"}, false},
		{"read legacy identity", "read", map[string]any{"dataset": "scene-set", "object": "metric/gmv"}, true},
		{"read legacy selector", "read", map[string]any{"reference": reference, "aspect": "definition"}, true},
		{"read incomplete identity", "read", map[string]any{"reference": "metric/gmv"}, true},
		{"put reference", "put", map[string]any{"reference": reference + "#/definition", "commandId": "update", "value": "replacement"}, false},
		{"put legacy identity", "put", map[string]any{"object": "metric/gmv", "commandId": "update", "value": "replacement"}, true},
		{"remove reference", "remove", map[string]any{"reference": reference, "commandId": "remove"}, false},
		{"remove legacy identity", "remove", map[string]any{"repository": "kr://scene/object", "object": "metric/gmv", "commandId": "remove"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var request any
			var target *string
			if tc.task == "read" {
				read := &objectclient.ReadRequest{}
				request, target = read, &read.Reference
			} else {
				maintenance := &objectclient.MaintenanceRequest{}
				request, target = maintenance, &maintenance.Reference
			}
			validated := false
			err := testkit.ValidateJSONFixture(tc.payload, request, func() error {
				validated = true
				_, err := objectmodel.ParseReference(*target)
				return err
			})
			if (err != nil) != tc.reject {
				t.Fatalf("reject=%v, want %v: %v", err != nil, tc.reject, err)
			}
			if !tc.reject && !validated {
				t.Fatal("wire decoding must run the supplied semantic validation")
			}
		})
	}
}
