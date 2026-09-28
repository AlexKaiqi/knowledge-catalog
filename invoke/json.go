package invoke

import (
	"encoding/json"
	"fmt"
)

// JSONOut renders one verb result as canonical JSON text. It is the single
// textual form shared by the CLI front and the HTTP facade; encodeCLIProduct
// shapes it, this only serializes it.
func JSONOut(value any) string {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("{\n  \"error\": {\n    \"message\": %q\n  }\n}\n", err.Error())
	}
	return string(b) + "\n"
}
