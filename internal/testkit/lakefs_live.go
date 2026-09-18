package testkit

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// LakeFSLive creates empty Graveler repositories for live scene walks.
// It is a testkit fixture, not a Snapshot adapter. ForkStamps is a no-op:
// live Graveler cannot preserve commit IDs across physical copies, so the
// live scene DFS uses a fresh cache per node (replay from root) instead of
// parent-home clones.
type LakeFSLive struct {
	origin    string
	cred      string
	namespace string
	prefix    string
	seq       atomic.Uint64
	client    *http.Client
	mu        sync.Mutex
	created   []string
}

// NewLakeFSLive talks to an already running lakeFS. origin is
// http://host:port without a repository path.
func NewLakeFSLive(t *testing.T, origin, credential, storageNamespace string) *LakeFSLive {
	t.Helper()
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	credential = strings.TrimSpace(credential)
	if origin == "" || credential == "" || !strings.Contains(credential, ":") {
		t.Fatal("live lakeFS origin and access-key:secret are required")
	}
	if storageNamespace == "" {
		storageNamespace = "s3://kc-authority"
	}
	live := &LakeFSLive{
		origin:    origin,
		cred:      credential,
		namespace: strings.TrimRight(storageNamespace, "/"),
		prefix:    liveRepoPrefix(),
		client:    &http.Client{Timeout: 30 * time.Second},
	}
	t.Cleanup(live.cleanup)
	return live
}

func liveRepoPrefix() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("scnl%x", time.Now().UnixNano())
	}
	return "scnl" + hex.EncodeToString(b[:])
}

func (f *LakeFSLive) Credential() string { return f.cred }

func (f *LakeFSLive) NewRepo() string {
	user, pass, _ := strings.Cut(f.cred, ":")
	var last string
	for attempt := 0; attempt < 64; attempt++ {
		name := fmt.Sprintf("%s%04d", f.prefix, f.seq.Add(1))
		body, _ := json.Marshal(map[string]string{
			"name":              name,
			"storage_namespace": f.namespace + "/" + name,
			"default_branch":    "main",
		})
		req, err := http.NewRequest(http.MethodPost, f.origin+"/api/v1/repositories", bytes.NewReader(body))
		if err != nil {
			panic(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth(user, pass)
		resp, err := f.client.Do(req)
		if err != nil {
			panic(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusConflict {
			last = fmt.Sprintf("lakefs create %s: HTTP %d %s", name, resp.StatusCode, raw)
			continue
		}
		if resp.StatusCode >= 300 {
			panic(fmt.Sprintf("lakefs create %s: HTTP %d %s", name, resp.StatusCode, raw))
		}
		f.mu.Lock()
		f.created = append(f.created, name)
		f.mu.Unlock()
		return name
	}
	if last == "" {
		last = "lakefs create: exhausted unique repository names"
	}
	panic(last)
}

func (f *LakeFSLive) DSN(name string) string {
	return f.origin + "/" + name
}

func (f *LakeFSLive) OwnsDSN(dsn string) bool {
	dsn = strings.TrimRight(strings.TrimSpace(dsn), "/")
	return strings.HasPrefix(dsn, f.origin+"/")
}

// ForkStamps is intentionally a no-op. See type comment.
func (f *LakeFSLive) ForkStamps(dst string) error {
	return nil
}

func (f *LakeFSLive) cleanup() {
	f.mu.Lock()
	names := append([]string(nil), f.created...)
	f.mu.Unlock()
	user, pass, _ := strings.Cut(f.cred, ":")
	for i := len(names) - 1; i >= 0; i-- {
		req, err := http.NewRequest(http.MethodDelete, f.origin+"/api/v1/repositories/"+names[i]+"?force=true", nil)
		if err != nil {
			continue
		}
		req.SetBasicAuth(user, pass)
		resp, err := f.client.Do(req)
		if err != nil {
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}
