package testkit

import (
	"io"
	"net/http"
)

// The stock lakeFS server data plane accepts raw bytes at POST objects. It
// populates the same isolated branch staging state as presigned backing PUT.
func (f *LakeFSFake) uploadObject(w http.ResponseWriter, r *http.Request, repo *lakefsFakeRepo, branch string) {
	path := r.URL.Query().Get("path")
	if _, exists := repo.branches[branch]; !exists {
		http.NotFound(w, r)
		return
	}
	if path == "" {
		http.Error(w, "missing object path", http.StatusBadRequest)
		return
	}
	content, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read upload", http.StatusBadRequest)
		return
	}
	stageLakeFSBytes(repo, branch, path, content)
	writeLakeFSJSON(w, http.StatusCreated, map[string]any{"path": path, "path_type": "object", "size_bytes": len(content), "checksum": "etag", "mtime": 1})
}

func stageLakeFSBytes(repo *lakefsFakeRepo, branch, path string, content []byte) {
	if repo.staged[branch] == nil {
		repo.staged[branch] = map[string]*[]byte{}
	}
	copied := append([]byte(nil), content...)
	repo.staged[branch][path] = &copied
}
