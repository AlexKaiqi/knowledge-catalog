package cli_test

import (
	serv "kc/server"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
)

func TestDeploymentSingleRegistryRecoversAllDatasetHistory(t *testing.T) {
	cfg, path := declaredDeployment(t, false)
	body(t, deploymentCommand(t, "deployment", "init", "--config", path))
	start := func() (*httptest.Server, http.Handler) {
		h, err := serv.HTTPHandlerFromConfig(path, serv.HTTPServerOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return httptest.NewServer(h), h
	}
	stop := func(server *httptest.Server, h http.Handler) {
		server.Close()
		if err := h.(interface{ Close() error }).Close(); err != nil {
			t.Fatal(err)
		}
	}
	server, h := start()
	t.Cleanup(func() {
		if server != nil {
			stop(server, h)
		}
	})
	call := func(args ...string) kcRunResult { return kcRemote(t, server.URL, cfg.BootstrapPrincipal, args...) }
	for _, name := range []string{"public-task", "restricted-task"} {
		for _, revision := range []string{"1", "2"} {
			body(t, call("dataset", "define", name, "--revision", revision, "--source", "kr://kc/system"))
		}
	}
	before := body(t, call("show"))
	audit := body(t, call("registry", "audit"))
	expectCode(t, call("catalog", "list"), "USAGE_INVALID")
	expectCode(t, call("catalog", "use", "old"), "USAGE_INVALID")
	expectCode(t, call("show", "--catalog", "old"), "USAGE_INVALID")
	stop(server, h)
	server = nil
	if err := os.RemoveAll(cfg.CacheDir); err != nil {
		t.Fatal(err)
	}
	server, h = start()
	if got := body(t, call("show")); !reflect.DeepEqual(before, got) {
		t.Fatalf("registry did not recover all Datasets: %#v != %#v", before, got)
	}
	if got := body(t, call("registry", "audit")); !reflect.DeepEqual(audit, got) {
		t.Fatalf("registry history changed during recovery: %#v != %#v", audit, got)
	}
}
