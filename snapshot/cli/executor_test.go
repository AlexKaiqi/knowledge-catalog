package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kc/kernel"
	snapshotcli "kc/snapshot/cli"
	"kc/snapshot/client"
)

type testTransport struct {
	client *http.Client
	calls  int
}

func (transport *testTransport) Do(ctx context.Context, audience string, request *http.Request) (*http.Response, error) {
	transport.calls++
	request = request.Clone(ctx)
	request.Header.Set("Authorization", "Bearer caller-provided")
	return transport.client.Do(request)
}

func TestIndependentSnapshotCommandsUseInjectedTransportAndTypedInputs(t *testing.T) {
	var defined client.DatasetRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer caller-provided" || request.Header.Get("X-Kc-As") != "" {
			t.Error("module replaced its injected authentication")
		}
		switch request.URL.Path {
		case "/snapshot/v1/inventory":
			_ = json.NewEncoder(w).Encode(map[string]any{"repositories": []string{"source"}})
		case "/snapshot/v1/datasets":
			if err := json.NewDecoder(request.Body).Decode(&defined); err != nil {
				t.Error(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"setId": defined.Dataset, "revision": defined.Revision, "metadata": defined.Metadata})
		default:
			t.Errorf("unexpected HTTP operation %s %s", request.Method, request.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	transport := &testTransport{client: server.Client()}
	sdk, err := client.New(client.Config{BaseURL: server.URL, Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	commands := snapshotcli.New(sdk, snapshotcli.Context{Principal: "opaque caller"})
	if _, err := commands.Execute(context.Background(), "show", nil, client.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	flags := map[string]any{"dataset": "delivery", "revision": "2", "source": "source@reference@handbook", "metadata": `{"description":"delivery","custom":"value"}`}
	if _, err := commands.Execute(context.Background(), "dataset define", flags, client.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if transport.calls != 2 || defined.Dataset != "delivery" || defined.Revision != 2 || !reflect.DeepEqual(defined.Metadata, map[string]string{"description": "delivery", "custom": "value"}) || len(defined.Sources) != 1 || *defined.Sources[0].Path != "reference" || defined.Sources[0].SubPath != "handbook" {
		t.Fatalf("Snapshot input or invocation changed: calls=%d definition=%#v", transport.calls, defined)
	}
	flags["metadata"] = `{"custom":null}`
	if _, err := commands.Execute(context.Background(), "dataset define", flags, client.RequestOptions{}); kernel.CodeOf(err) != kernel.ErrUsageInvalid || transport.calls != 2 {
		t.Fatalf("invalid metadata reached transport: %d %v", transport.calls, err)
	}
}

func TestSnapshotHelpHasNoRemoteOrLocalSideEffects(t *testing.T) {
	transport := &helpRejectingTransport{}
	sdk, err := client.New(client.Config{BaseURL: "http://unused.invalid", Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	entries := snapshotcli.Commands()
	if len(entries) != 14 {
		t.Fatalf("help must cover all 14 module leaves, got %d", len(entries))
	}
	for _, command := range entries {
		t.Run(command.Path, func(t *testing.T) {
			if strings.TrimSpace(command.Description) == "" || strings.TrimSpace(command.Usage) == "" {
				t.Fatalf("help declaration is incomplete: %#v", command)
			}
			flags := map[string]any{"help": true}
			switch command.Path {
			case "dataset define":
				flags["file"] = filepath.Join(dir, "missing-recipe")
			case "dataset overlay":
				flags["file"], flags["overlay"], flags["out"] = filepath.Join(dir, "missing-recipe"), filepath.Join(dir, "missing-overlay"), filepath.Join(dir, "output.yaml")
			case "grant update":
				flags["file"], flags["expected-file"] = filepath.Join(dir, "missing-replacement"), filepath.Join(dir, "missing-observed-rule")
			case "files read":
				flags["out"] = filepath.Join(dir, "download.bin")
			}
			value, err := snapshotcli.New(sdk, snapshotcli.Context{}).Execute(context.Background(), command.Path, flags, client.RequestOptions{})
			want := command.Description + "\n\n" + command.Usage
			if err != nil || value != want {
				t.Fatalf("help must preserve the complete description and usage without opening inputs:\n got: %#v (%v)\nwant: %q", value, err, want)
			}
		})
	}
	if transport.calls != 0 {
		t.Fatalf("help performed %d HTTP calls", transport.calls)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("help created local inputs or outputs: %v %v", files, err)
	}
}

type helpRejectingTransport struct{ calls int }

func (transport *helpRejectingTransport) Do(context.Context, string, *http.Request) (*http.Response, error) {
	transport.calls++
	return nil, errors.New("help must not request HTTP")
}

func TestSnapshotDeclarationOwnsHelpAndRestrictedFlags(t *testing.T) {
	entries := snapshotcli.Commands()
	seen := map[string]bool{}
	for _, command := range entries {
		if seen[command.Path] || command.Handler == "" || command.Action == "" || command.Usage == "" {
			t.Fatalf("invalid module declaration: %#v", command)
		}
		seen[command.Path] = true
		if command.Path == "files read" {
			command.Flags[0] = "mutated"
		}
		if command.Path == "dataset show" {
			if !reflect.DeepEqual(command.Operands, []string{"dataset"}) || !reflect.DeepEqual(command.Groups, []string{"composition", "dataset", "files"}) {
				t.Fatalf("dataset show lost its declared operands or task groups: %#v", command)
			}
			command.Operands[0] = "mutated"
			command.Groups[0], command.Groups[1], command.Groups[2] = "mutated", "also-mutated", "third-mutated"
		}
	}
	if seen["create"] || len(seen) != 14 {
		t.Fatalf("module absorbed host provisioning or lost leaves: %#v", seen)
	}
	if err := snapshotcli.New(nil, snapshotcli.Context{}).Validate("files read", map[string]any{"dataset": "d", "path": "file", "server": "url"}); err != nil {
		t.Fatalf("declaration returned a mutable flag list: %v", err)
	}
	if err := snapshotcli.New(nil, snapshotcli.Context{}).Validate("files read", map[string]any{"dataset": "d", "path": "file", "repo": "other"}); kernel.CodeOf(err) != kernel.ErrUsageInvalid {
		t.Fatalf("module ignored its declared flag boundary: %v", err)
	}
	for _, command := range snapshotcli.Commands() {
		if command.Path == "dataset show" && (!reflect.DeepEqual(command.Operands, []string{"dataset"}) || !reflect.DeepEqual(command.Groups, []string{"composition", "dataset", "files"})) {
			t.Fatalf("returned slices mutated the formal operand/group declarations: %#v", command)
		}
	}
}
