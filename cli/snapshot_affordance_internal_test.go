package cli

import (
	"context"
	"errors"
	"net/http"
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
	t.Setenv("KC_SERVER_URL", "http://snapshot-help.invalid")
	t.Setenv("KC_AUTH_TOKEN", "help-must-not-use-authentication")
	t.Setenv("KC_AS", "help-reader")
	t.Setenv("KC_HOME", filepath.Join(dir, "home"))
	t.Setenv("KC_CONFIG_DIR", filepath.Join(dir, "config"))
	transport := &snapshotHelpRejectingRoundTripper{}
	originalTransport := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })

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
	if calls := transport.calls.Load(); calls != 0 {
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
	case "dataset clone":
		return []string{"--dir", filepath.Join(dir, "clone")}
	default:
		return nil
	}
}

type snapshotHelpRejectingRoundTripper struct{ calls atomic.Int32 }

func (transport *snapshotHelpRejectingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	transport.calls.Add(1)
	return nil, errors.New("help must not request HTTP")
}
