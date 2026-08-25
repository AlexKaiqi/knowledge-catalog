package catalog_test

import (
	"fmt"
	"os"
	"testing"

	"kc/internal/testkit"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if err := testkit.StopGitea(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}
