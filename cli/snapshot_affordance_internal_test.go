package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	snapshotcli "kc/snapshot/cli"
	snapshotclient "kc/snapshot/client"
)

func TestSnapshotHelpPreservesModuleContentThroughPublicEntry(t *testing.T) {
	dir := t.TempDir()
	calls := &countingHandler{}
	server := httptest.NewServer(calls)
	t.Cleanup(server.Close)
	t.Setenv("KC_SERVER_URL", server.URL)
	t.Setenv("KC_AUTH_TOKEN", "help-must-not-use-authentication")
	t.Setenv("KC_AS", "help-reader")
	t.Setenv("KC_HOME", filepath.Join(dir, "home"))
	t.Setenv("KC_CONFIG_DIR", filepath.Join(dir, "config"))

	executor := snapshotcli.New(nil, snapshotcli.Context{})
	for _, command := range snapshotcli.Commands() {
		t.Run(command.Path, func(t *testing.T) {
			if strings.TrimSpace(command.Description) == "" || strings.TrimSpace(command.Usage) == "" {
				t.Fatalf("module has incomplete leaf help: %#v", command)
			}
			value, err := executor.Execute(context.Background(), command.Path, map[string]any{"help": true}, snapshotclient.RequestOptions{})
			want := command.Description + "\n\n" + command.Usage
			if err != nil || value != want {
				t.Fatalf("independent help omitted module content: %#v %v", value, err)
			}
			if actual := formatLeafHelp(command.Path); actual != want {
				t.Fatalf("root leaf formatter lost module help:\n got: %q\nwant: %q", actual, want)
			}
			path := strings.Fields(command.Path)
			variants := map[string][]string{
				"help":       append([]string{"help"}, path...),
				"help flag":  append(append([]string{}, path...), "--help"),
				"with input": append(append(append([]string{}, path...), snapshotHelpInputArgs(command.Path, dir)...), "--help"),
			}
			for name, argv := range variants {
				t.Run(name, func(t *testing.T) {
					result := Run(argv)
					if result.Status != 0 || result.Stdout != want+"\n" {
						t.Fatalf("public help must preserve module content without running the command:\nargv: %q\nresult: %#v\nwant: %q", argv, result, want+"\n")
					}
				})
			}
		})
	}
	if calls := calls.count.Load(); calls != 0 {
		t.Fatalf("public help made %d HTTP requests", calls)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("public help created session, Home, or output files: %v %v", files, err)
	}
}

func snapshotHelpInputArgs(path, dir string) []string {
	switch path {
	case "dataset define":
		return []string{"--file", filepath.Join(dir, "missing-recipe"), "--revision", "invalid"}
	case "dataset overlay":
		return []string{"--file", filepath.Join(dir, "missing-recipe"), "--overlay", filepath.Join(dir, "missing-overlay"), "--out", filepath.Join(dir, "output.yaml")}
	case "grant update":
		return []string{"--file", filepath.Join(dir, "missing-replacement"), "--expected-file", filepath.Join(dir, "missing-observed-rule")}
	case "files read":
		return []string{"--out", filepath.Join(dir, "download.bin")}
	default:
		return nil
	}
}

// countingHandler records every request the help path must never make. Using a
// real server keeps the assertion observable without touching the package-level
// http.DefaultTransport, which other cases in this package also use: replacing
// it made the count depend on package-wide scheduling instead of on this case.
type countingHandler struct {
	count atomic.Int32
}

func (handler *countingHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	handler.count.Add(1)
	http.Error(writer, "help must not request HTTP", http.StatusInternalServerError)
}
