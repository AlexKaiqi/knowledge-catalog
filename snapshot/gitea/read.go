package gitea

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"kc/kernel"
)

// treeAt caches the immutable path -> blob SHA response from Gitea. It is a
// layer ⓪ transport cache: no object_id, Aspect, or parsed knowledge survives.
func (r *Repository) treeAt(commitID kernel.CommitID) (map[string]string, error) {
	r.mu.Lock()
	if blobs, ok := r.trees[commitID]; ok {
		r.mu.Unlock()
		return blobs, nil
	}
	r.mu.Unlock()

	blobs := map[string]string{}
	page := 1
	for {
		q := "?recursive=true&page=" + strconv.Itoa(page) + "&per_page=1000"
		var tree gitTree
		status, _, err := r.cli.do(http.MethodGet, r.ep.repoPath("git/trees/"+url.PathEscape(string(commitID))+q), nil, &tree)
		if missingCommit(status, err) {
			return nil, kernel.Fail(kernel.ErrVersionUnresolved, "commit %s does not exist", commitID)
		}
		if err != nil {
			return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea tree at %s: %v", commitID, err)
		}
		for _, e := range tree.Tree {
			if e.Type != "blob" {
				continue
			}
			blobs[e.Path] = e.SHA
		}
		if !tree.Truncated {
			break
		}
		page++
	}
	r.mu.Lock()
	if existing, ok := r.trees[commitID]; ok {
		blobs = existing
	} else {
		r.trees[commitID] = blobs
	}
	r.mu.Unlock()
	return blobs, nil
}

func (r *Repository) readBlob(sha string) (string, error) {
	r.mu.Lock()
	if content, ok := r.blobBodies[sha]; ok {
		r.mu.Unlock()
		return content, nil
	}
	r.mu.Unlock()

	var blob gitBlob
	if _, _, err := r.cli.do(http.MethodGet, r.ep.repoPath("git/blobs/"+url.PathEscape(sha)), nil, &blob); err != nil {
		return "", kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea blob %s: %v", sha, err)
	}
	content := blob.Content
	if strings.EqualFold(blob.Encoding, "base64") {
		b, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(blob.Content), ""))
		if err != nil {
			return "", err
		}
		content = string(b)
	}
	r.mu.Lock()
	if existing, ok := r.blobBodies[sha]; ok {
		content = existing
	} else {
		r.blobBodies[sha] = content
	}
	r.mu.Unlock()
	return content, nil
}
