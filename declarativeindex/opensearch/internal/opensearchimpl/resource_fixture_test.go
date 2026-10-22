package opensearchimpl

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// The small HTTP fixtures for root-pointer isolation also implement durable
// document CAS for resource records. Real lifecycle tests use OpenSearch.
type resourceControlFixture struct {
	mu      sync.Mutex
	records map[string]json.RawMessage
	seq     map[string]int64
}

func (f *resourceControlFixture) serve(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/_search") {
		_ = json.NewEncoder(w).Encode(map[string]any{"_shards": map[string]any{"total": 1, "successful": 1, "failed": 0}, "hits": map[string]any{"hits": []any{}}})
		return true
	}
	if !strings.Contains(r.URL.Path, "/_doc/") || (!strings.Contains(r.URL.Path, "-resource-") && !strings.Contains(r.URL.Path, "-publication-recovery") && !strings.HasSuffix(r.URL.Path, "-scope")) {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.records == nil {
		f.records, f.seq = map[string]json.RawMessage{}, map[string]int64{}
	}
	previous, exists := f.records[r.URL.Path]
	if r.Method == http.MethodGet {
		if !exists {
			http.NotFound(w, r)
			return true
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"_source": previous, "_seq_no": f.seq[r.URL.Path], "_primary_term": 1})
		return true
	}
	if r.Method != http.MethodPut {
		http.Error(w, "unsupported fixture operation", http.StatusMethodNotAllowed)
		return true
	}
	query := r.URL.Query()
	if (query.Get("op_type") == "create" && exists) || (query.Has("if_seq_no") && (!exists || query.Get("if_seq_no") != strconv.FormatInt(f.seq[r.URL.Path], 10) || query.Get("if_primary_term") != "1")) {
		w.WriteHeader(http.StatusConflict)
		return true
	}
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return true
	}
	f.records[r.URL.Path] = raw
	f.seq[r.URL.Path]++
	_ = json.NewEncoder(w).Encode(map[string]any{"_seq_no": f.seq[r.URL.Path], "_primary_term": 1})
	return true
}
