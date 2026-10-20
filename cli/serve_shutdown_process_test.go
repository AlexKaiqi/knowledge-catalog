package cli_test

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"kc/declarativeindex/opensearch"
	"kc/kernel"
)

type serveShutdownProcess struct {
	command *exec.Cmd
	done    chan error
	logPath string
	stopped bool
}

func serveShutdownAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func startServeShutdownProcess(t *testing.T, config, address string) *serveShutdownProcess {
	t.Helper()
	process := &serveShutdownProcess{done: make(chan error, 1), logPath: filepath.Join(t.TempDir(), "serve.log")}
	log, err := os.Create(process.logPath)
	if err != nil {
		t.Fatal(err)
	}
	process.command = exec.Command(os.Args[0], "-test.run=^TestServeSIGTERMCancelsMaintenanceAndRecoversOnRestart$", "-test.count=1")
	process.command.Env = append(os.Environ(), "KC_TEST_SERVE_SHUTDOWN_CONFIG="+config,
		"KC_TEST_SERVE_SHUTDOWN_LISTEN="+address, "KC_ASSERT_E2E_COVERAGE=", "KC_COMMAND_COVERAGE_REPORT=")
	process.command.Stdout, process.command.Stderr = log, log
	if err := process.command.Start(); err != nil {
		_ = log.Close()
		t.Fatal(err)
	}
	go func() {
		err := process.command.Wait()
		_ = log.Close()
		process.done <- err
	}()
	t.Cleanup(func() {
		if process.stopped {
			return
		}
		_ = process.command.Process.Kill()
		select {
		case <-process.done:
		case <-time.After(5 * time.Second):
			t.Error("failed subprocess could not be reaped")
		}
	})
	return process
}

func (p *serveShutdownProcess) output(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(p.logPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func (p *serveShutdownProcess) waitHTTP(t *testing.T, address string) {
	t.Helper()
	client := &http.Client{Timeout: 300 * time.Millisecond}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-p.done:
			p.stopped = true
			t.Fatalf("serve exited before HTTP listening: %v\n%s", err, p.output(t))
		default:
		}
		response, err := client.Get("http://" + address + "/livez")
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("serve did not listen over HTTP\n%s", p.output(t))
}

func (p *serveShutdownProcess) stop(t *testing.T) time.Duration {
	t.Helper()
	start := time.Now()
	if err := p.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-p.done:
		p.stopped = true
		elapsed := time.Since(start)
		if err != nil || elapsed >= 15*time.Second {
			t.Fatalf("SIGTERM failed graceful exit in 15 seconds: elapsed=%s error=%v\n%s", elapsed, err, p.output(t))
		}
		return elapsed
	case <-time.After(15 * time.Second):
		_ = p.command.Process.Signal(syscall.SIGQUIT)
		select {
		case <-p.done:
			p.stopped = true
		case <-time.After(2 * time.Second):
		}
		t.Fatalf("SIGTERM did not exit in 15 seconds; failure cleanup must kill it\n%s", p.output(t))
		return 0
	}
}

func cleanupServeShutdownSearch(t *testing.T, cfg opensearch.Config) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	base := strings.TrimRight(cfg.URL, "/")
	response, err := client.Get(base + "/_cat/indices?format=json&h=index")
	if err != nil {
		t.Error(err)
		return
	}
	defer response.Body.Close()
	var rows []struct {
		Index string `json:"index"`
	}
	if response.StatusCode != http.StatusOK {
		t.Errorf("list owned search resources: HTTP %d", response.StatusCode)
		return
	}
	if err := json.NewDecoder(response.Body).Decode(&rows); err != nil {
		t.Error(err)
		return
	}
	scope := string(kernel.CanonicalDigest(cfg.Namespace))[:24]
	removed := 0
	for _, row := range rows {
		if row.Index != "kc-projection-control-v1-"+scope && !strings.HasPrefix(row.Index, "kc-proj-"+scope+"-") {
			continue
		}
		request, err := http.NewRequest(http.MethodDelete, base+"/"+row.Index, nil)
		if err != nil {
			t.Error(err)
			continue
		}
		result, err := client.Do(request)
		if err != nil {
			t.Error(err)
			continue
		}
		_, _ = io.Copy(io.Discard, result.Body)
		_ = result.Body.Close()
		if result.StatusCode != http.StatusOK && result.StatusCode != http.StatusNotFound {
			t.Errorf("remove owned search index %s: HTTP %d", row.Index, result.StatusCode)
		} else {
			removed++
		}
	}
	t.Logf("removed %d search indices owned by isolated namespace %s", removed, cfg.Namespace)
}
