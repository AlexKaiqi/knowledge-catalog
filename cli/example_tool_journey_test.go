package cli_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestQingheDeliveredToolRunsFromPublishedInstructions(t *testing.T) {
	world := readExampleWorld(t)
	d := openExampleDeployment(t, world)
	d.publish(t, world, true)
	const consumer, dataset, revision = "agent:tool-consumer", "store-oncall", "2"
	read := asMap(t, body(t, d.as(t, consumer, "read", "urn:kc:object:tool/qinghe.order-cli#/definition",
		"--dataset", dataset, "--revision", revision)))
	definition := asMap(t, read["value"])
	install, _ := definition["install"].(string)
	version, _ := definition["version"].(string)
	path := regexp.MustCompile(`tools/[A-Za-z0-9/_.-]+\.sh`).FindString(install)
	if definition["kind"] != "cli" || definition["name"] != "order-cli" || version == "" ||
		path == "" || !strings.Contains(install, "bash") || !strings.Contains(install, "curl") || !strings.Contains(install, "--help") {
		t.Fatalf("published tool must explain its version, script and runtime: %#v", definition)
	}
	start, _ := definition["start"].(string)
	startArgs := strings.Fields(start)
	if len(startArgs) < 3 || startArgs[1] != "./order-cli" {
		t.Fatalf("published tool must explain configuration and startup: %q", start)
	}
	configuration, placeholder, ok := strings.Cut(startArgs[0], "=")
	if !ok || configuration == "" || placeholder != "<endpoint>" {
		t.Fatalf("published startup must identify its service configuration: %q", start)
	}
	listing := asMap(t, body(t, d.as(t, consumer, "files", "list", "--dataset", dataset,
		"--revision", revision, "--path", filepath.ToSlash(filepath.Dir(path)))))
	found := false
	for _, entry := range listing["entries"].([]any) {
		found = found || asMap(t, entry)["path"] == path
	}
	if !found {
		t.Fatalf("tool instructions name a file absent from the selected revision: %#v", listing)
	}
	installed := filepath.Join(t.TempDir(), definition["name"].(string))
	receipt := asMap(t, body(t, d.as(t, consumer, "files", "read", "--dataset", dataset,
		"--revision", revision, "--path", path, "--out", installed)))
	if receipt["dataset"] != dataset || receipt["revision"] != float64(2) || receipt["path"] != path ||
		receipt["pinId"] != listing["pinId"] || !reflect.DeepEqual(receipt["repositories"], listing["repositories"]) {
		t.Fatalf("tool download changed the selected publication: listing=%#v receipt=%#v", listing, receipt)
	}
	if err := os.Chmod(installed, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("offline-help", func(t *testing.T) {
		output, err := runDeliveredOrderTool(t, installed, configuration, "", "--help")
		if err != nil || !strings.Contains(output, "order-cli "+version) || !strings.Contains(output, "health") ||
			!strings.Contains(output, "get <orderId>") || !strings.Contains(output, "ORDER_SERVICE_URL") {
			t.Fatalf("downloaded tool must explain itself without a service: %v\n%s", err, output)
		}
	})
	t.Run("missing-service", func(t *testing.T) {
		output, err := runDeliveredOrderTool(t, installed, configuration, "", "health")
		exit, ok := err.(*exec.ExitError)
		var failure map[string]string
		if !ok || exit.ExitCode() != 3 || json.Unmarshal([]byte(output), &failure) != nil ||
			failure["error"] != "ORDER_SERVICE_URL is not set; this public example has no live order service" {
			t.Fatalf("unconfigured example must not invent live health: %v\n%s", err, output)
		}
	})
	t.Run("documented-queries", func(t *testing.T) {
		requests := make(chan string, 4)
		want := map[string]map[string]any{
			"GET /health":        {"service": "qinghe.order-service", "status": "UP"},
			"GET /orders/O-1042": {"orderId": "O-1042", "status": "PAID", "storeId": "QH-001"},
		}
		expectedRequests := map[string]string{
			"./order-cli health":     "GET /health",
			"./order-cli get O-1042": "GET /orders/O-1042",
		}
		service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			request := r.Method + " " + r.URL.RequestURI()
			requests <- request
			payload, ok := want[request]
			if !ok {
				http.Error(w, "unexpected example request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(payload)
		}))
		defer service.Close()
		examples, ok := definition["examples"].([]any)
		if !ok || len(examples) != len(want) {
			t.Fatalf("published tool must contain its health and order query examples: %#v", definition)
		}
		// Execute the public startup as well as each example, using the variable
		// named in that startup rather than supplying a private configuration.
		calls := append([]any{strings.Join(startArgs[1:], " ")}, examples...)
		seen := make(map[string]int)
		for _, example := range calls {
			line, ok := example.(string)
			args := strings.Fields(line)
			if !ok || len(args) < 2 || args[0] != "./order-cli" {
				t.Fatalf("example does not invoke the installed tool: %#v", example)
			}
			expectedRequest, known := expectedRequests[line]
			if !known {
				t.Fatalf("published query has no expected service operation: %q", line)
			}
			output, err := runDeliveredOrderTool(t, installed, configuration, service.URL, args[1:]...)
			if err != nil {
				t.Fatalf("published example %q failed: %v\n%s", line, err, output)
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(output), &got); err != nil {
				t.Fatalf("published example %q returned invalid JSON: %s", line, output)
			}
			select {
			case request := <-requests:
				if request != expectedRequest || !reflect.DeepEqual(got, want[expectedRequest]) {
					t.Fatalf("published example %q changed its service operation or result: request=%s result=%#v", line, request, got)
				}
				seen[request]++
			default:
				t.Fatalf("published example %q never queried the supplied service", line)
			}
		}
		if seen["GET /health"] != 2 || seen["GET /orders/O-1042"] != 1 || len(requests) != 0 {
			t.Fatalf("published examples did not make exactly the expected requests: %#v extra=%d", seen, len(requests))
		}
	})
}

func runDeliveredOrderTool(t *testing.T, installed, configuration, endpoint string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, installed, args...)
	command.Env = append(os.Environ(), "ORDER_SERVICE_URL=", configuration+"="+endpoint)
	output, err := command.CombinedOutput()
	return string(output), err
}
