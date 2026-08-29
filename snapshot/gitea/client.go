package gitea

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kc/kernel"
)

type client struct {
	http  *http.Client
	api   string
	token string
}

const maxResponseBytes = 64 << 20

func newClient(api, token string) *client {
	return &client{
		http:  &http.Client{Timeout: 60 * time.Second},
		api:   strings.TrimRight(api, "/"),
		token: token,
	}
}

type apiError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *apiError) Error() string {
	msg := strings.TrimSpace(e.Body)
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	return fmt.Sprintf("gitea %s %s: %s", e.Method, e.Path, msg)
}

func (e *apiError) Unwrap() error {
	if e.Status == http.StatusRequestTimeout || e.Status == http.StatusTooManyRequests || e.Status >= http.StatusInternalServerError {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea %s %s returned HTTP %d", e.Method, e.Path, e.Status)
	}
	return nil
}

func statusOf(err error) int {
	var a *apiError
	if err != nil && asAPIError(err, &a) {
		return a.Status
	}
	return 0
}

func asAPIError(err error, out **apiError) bool {
	if err == nil {
		return false
	}
	var e *apiError
	if !errors.As(err, &e) {
		return false
	}
	*out = e
	return true
}

func missingCommit(status int, err error) bool {
	if status == http.StatusNotFound {
		return true
	}
	if status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
		return false
	}
	msg := ""
	if err != nil {
		msg = strings.ToLower(err.Error())
	}
	return strings.Contains(msg, "not found") || strings.Contains(msg, "invalid")
}

func (c *client) do(method, path string, body any, out any) (int, []byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.api+path, rdr)
	if err != nil {
		return 0, nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea %s: %v", path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return resp.StatusCode, nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea read %s response: %v", path, err)
	}
	if len(raw) > maxResponseBytes {
		return resp.StatusCode, nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea %s response exceeds %d bytes", path, maxResponseBytes)
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, raw, &apiError{Method: method, Path: path, Status: resp.StatusCode, Body: string(raw)}
	}
	if out != nil && len(raw) > 0 && resp.StatusCode != http.StatusNoContent {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, raw, kernel.Fail(kernel.ErrTemporaryUnavailable, "gitea decode %s response: %v", path, err)
		}
	}
	return resp.StatusCode, raw, nil
}

type gitIdentity struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

type gitRefObject struct {
	SHA  string `json:"sha"`
	Type string `json:"type,omitempty"`
}

type gitRef struct {
	Ref    string       `json:"ref"`
	Object gitRefObject `json:"object"`
}

type gitTreeEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"`
	SHA  string `json:"sha"`
	Size int64  `json:"size,omitempty"`
}

type gitTree struct {
	SHA       string         `json:"sha"`
	Tree      []gitTreeEntry `json:"tree"`
	Truncated bool           `json:"truncated"`
}

type gitBlob struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
	SHA      string `json:"sha"`
}

type gitCommit struct {
	SHA     string `json:"sha"`
	ID      string `json:"id,omitempty"`
	Parents []struct {
		SHA string `json:"sha"`
		ID  string `json:"id,omitempty"`
	} `json:"parents"`
}

type repoInfo struct {
	DefaultBranch string `json:"default_branch"`
	Empty         bool   `json:"empty"`
}

type branchInfo struct {
	Name   string `json:"name"`
	Commit struct {
		ID  string `json:"id"`
		SHA string `json:"sha"`
	} `json:"commit"`
}

func (b branchInfo) commitID() kernel.CommitID {
	if b.Commit.ID != "" {
		return kernel.CommitID(b.Commit.ID)
	}
	return kernel.CommitID(b.Commit.SHA)
}

type tagInfo struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
		ID  string `json:"id"`
	} `json:"commit"`
}

func (t tagInfo) commitID() kernel.CommitID {
	if t.Commit.SHA != "" {
		return kernel.CommitID(t.Commit.SHA)
	}
	return kernel.CommitID(t.Commit.ID)
}

type userInfo struct {
	Login string `json:"login"`
}

type createRepoBody struct {
	Name          string `json:"name"`
	Private       bool   `json:"private"`
	AutoInit      bool   `json:"auto_init"`
	DefaultBranch string `json:"default_branch"`
}

type createBranchBody struct {
	NewBranchName string `json:"new_branch_name"`
	OldRefName    string `json:"old_ref_name,omitempty"`
}

type updateBranchBody struct {
	NewCommitID string `json:"new_commit_id"`
	OldCommitID string `json:"old_commit_id,omitempty"`
	Force       bool   `json:"force,omitempty"`
}

type createTagBody struct {
	TagName string `json:"tag_name"`
	Message string `json:"message,omitempty"`
	Target  string `json:"target"`
}

type changeFileOp struct {
	Operation string `json:"operation"`
	Path      string `json:"path"`
	Content   string `json:"content,omitempty"`
	SHA       string `json:"sha,omitempty"`
	FromPath  string `json:"from_path,omitempty"`
}

type changeFilesBody struct {
	Message   string         `json:"message"`
	Branch    string         `json:"branch,omitempty"`
	NewBranch string         `json:"new_branch,omitempty"`
	Author    gitIdentity    `json:"author"`
	Committer gitIdentity    `json:"committer"`
	Files     []changeFileOp `json:"files"`
}

type filesResponse struct {
	Commit struct {
		SHA string `json:"sha"`
		ID  string `json:"id"`
	} `json:"commit"`
}

func (f filesResponse) sha() string {
	if f.Commit.SHA != "" {
		return f.Commit.SHA
	}
	return f.Commit.ID
}

type commitRow struct {
	SHA    string `json:"sha"`
	SHAAlt string `json:"id"`
}

func (c commitRow) id() string {
	if c.SHA != "" {
		return c.SHA
	}
	return c.SHAAlt
}
