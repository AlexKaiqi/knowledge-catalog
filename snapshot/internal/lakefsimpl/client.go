package lakefsimpl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"kc/kernel"
)

const (
	maxResponseBytes = 64 << 20
	maxPageSize      = 1000
)

type apiClient struct {
	api        string
	repository string
	accessKey  string
	secretKey  string
	http       *http.Client
	direct     *http.Client
	// ctx, when set, cancels every read this client issues. A nil ctx is the
	// unbound client used for writes and for callers without a lifecycle.
	ctx context.Context
}

func (c *apiClient) withContext(ctx context.Context) *apiClient {
	if c == nil || ctx == nil {
		return c
	}
	bound := *c
	bound.ctx = ctx
	return &bound
}

func (c *apiClient) context() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

// canceled reports the bound context's error once it has ended, so a probe
// that folds every failure into "absent" cannot turn cancellation into a
// missing commit.
func (c *apiClient) canceled() error { return c.context().Err() }

// requestError reports a failed request. A canceled or expired bound context
// is returned as that context error, not as a retryable lakeFS failure.
func (c *apiClient) requestError(what string, err error) error {
	if ctxErr := c.context().Err(); ctxErr != nil {
		return ctxErr
	}
	return kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs %s: %v", what, err)
}

type apiError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *apiError) Error() string {
	body := strings.TrimSpace(e.Body)
	if body == "" {
		body = http.StatusText(e.Status)
	}
	return fmt.Sprintf("lakefs %s %s: %s", e.Method, e.Path, body)
}

func (e *apiError) Unwrap() error {
	switch {
	case e.Status == http.StatusUnauthorized:
		return kernel.Fail(kernel.ErrUnauthenticated, "lakefs rejected repository credential")
	case e.Status == http.StatusForbidden:
		return kernel.Fail(kernel.ErrForbidden, "lakefs denied repository operation")
	case e.Status == http.StatusRequestTimeout || e.Status == http.StatusTooManyRequests || e.Status >= 500:
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs returned HTTP %d", e.Status)
	default:
		return nil
	}
}

func statusOf(err error) int {
	var api *apiError
	if errors.As(err, &api) {
		return api.Status
	}
	return 0
}

func newAPIClient(endpoint Endpoint, credential string) (*apiClient, error) {
	accessKey, secretKey, ok := strings.Cut(strings.TrimSpace(credential), ":")
	if !ok || strings.TrimSpace(accessKey) == "" || secretKey == "" {
		return nil, kernel.Fail(kernel.ErrUnauthenticated,
			"lakefs credential must be supplied as access-key:secret-key")
	}
	return &apiClient{
		api:        endpoint.API,
		repository: endpoint.Repository,
		accessKey:  accessKey,
		secretKey:  secretKey,
		http: &http.Client{
			Transport:     pooledTransport(),
			Timeout:       60 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		direct: &http.Client{Transport: pooledTransport(), Timeout: 5 * time.Minute},
	}, nil
}

// pooledTransport sizes the idle pool to the adapter's own fan-out; staging
// and planning reads each hold API and data-plane connections. Go's default
// keeps two idle connections per host, so a bounded fan-out would reopen TCP
// on almost every request and exhaust local ephemeral ports on large uploads.
// Kept local so the adapter does not depend on internal/httpjson.
func pooledTransport() *http.Transport {
	perHost := stageConcurrency() * 2
	if perHost < minIdleConnsPerHost {
		perHost = minIdleConnsPerHost
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = perHost
	if transport.MaxIdleConns < perHost*2 {
		transport.MaxIdleConns = perHost * 2
	}
	return transport
}

const minIdleConnsPerHost = 64

func (c *apiClient) repositoryPath(suffix string) string {
	base := "/repositories/" + url.PathEscape(c.repository)
	if suffix == "" {
		return base
	}
	return base + "/" + strings.TrimPrefix(suffix, "/")
}

func (c *apiClient) doJSON(method, path string, body, out any) (int, http.Header, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(c.context(), method, c.api+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.SetBasicAuth(c.accessKey, c.secretKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, c.requestError(path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return resp.StatusCode, resp.Header, c.requestError("read response", err)
	}
	if len(raw) > maxResponseBytes {
		return resp.StatusCode, resp.Header, kernel.Fail(kernel.ErrTemporaryUnavailable,
			"lakefs response exceeds %d bytes", maxResponseBytes)
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, resp.Header, &apiError{Method: method, Path: path, Status: resp.StatusCode, Body: string(raw)}
	}
	if out != nil && len(raw) != 0 && resp.StatusCode != http.StatusNoContent {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, resp.Header, kernel.Fail(kernel.ErrTemporaryUnavailable,
				"lakefs decode %s response: %v", path, err)
		}
	}
	return resp.StatusCode, resp.Header, nil
}

func (c *apiClient) getObject(ref, objectPath string) ([]byte, error) {
	path := c.repositoryPath("refs/"+url.PathEscape(ref)+"/objects") +
		"?path=" + url.QueryEscape(objectPath) + "&presign=true"
	req, err := http.NewRequestWithContext(c.context(), http.MethodGet, c.api+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.accessKey, c.secretKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, c.requestError("read object", err)
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		// Consume the bounded redirect body so repeated immutable reads reuse
		// the API connection rather than exhausting local ephemeral ports.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if location == "" {
			return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs presigned read omitted Location")
		}
		direct, err := http.NewRequestWithContext(c.context(), http.MethodGet, location, nil)
		if err != nil {
			return nil, err
		}
		resp, err = c.direct.Do(direct)
		if err != nil {
			return nil, c.requestError("direct read", err)
		}
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if readErr != nil {
		return nil, c.requestError("read object body", readErr)
	}
	if resp.StatusCode >= 400 {
		return nil, &apiError{Method: http.MethodGet, Path: path, Status: resp.StatusCode, Body: string(raw)}
	}
	if len(raw) > maxResponseBytes {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs object exceeds %d bytes", maxResponseBytes)
	}
	return raw, nil
}

type stagingLocation struct {
	PhysicalAddress string `json:"physical_address"`
	PresignedURL    string `json:"presigned_url"`
}

func (c *apiClient) stage(branch, objectPath string, content []byte) error {
	path := c.repositoryPath("branches/"+url.PathEscape(branch)+"/staging/backing") +
		"?path=" + url.QueryEscape(objectPath)
	for attempt := 0; attempt < 3; attempt++ {
		var location stagingLocation
		if _, _, err := c.doJSON(http.MethodGet, path+"&presign=true", nil, &location); err != nil {
			return err
		}
		if location.PresignedURL == "" {
			return kernel.Fail(kernel.ErrCapabilityUnsatisfied, "lakefs backing store does not provide presigned writes")
		}
		req, err := http.NewRequest(http.MethodPut, location.PresignedURL, bytes.NewReader(content))
		if err != nil {
			return err
		}
		req.ContentLength = int64(len(content))
		resp, err := c.direct.Do(req)
		if err != nil {
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs direct write: %v", err)
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs backing store PUT returned HTTP %d", resp.StatusCode)
		}
		checksum := strings.Trim(resp.Header.Get("ETag"), "\"")
		if checksum == "" {
			sum := sha256.Sum256(content)
			checksum = hex.EncodeToString(sum[:])
		}
		metadata := struct {
			Staging  stagingLocation `json:"staging"`
			Checksum string          `json:"checksum"`
			Size     int64           `json:"size_bytes"`
		}{Staging: location, Checksum: checksum, Size: int64(len(content))}
		status, _, err := c.doJSON(http.MethodPut, path, metadata, nil)
		if status == http.StatusConflict {
			continue // staging token rotated: obtain a new location and re-upload.
		}
		return err
	}
	return kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs staging changed concurrently")
}

// uploadObject stages one object through the lakeFS server data plane: a
// single request carries the bytes and lakeFS places them on the backing
// store. The bulk commit path uses it to trade the three-round-trip presigned
// staging dance for one round trip per object on many-small-object commits;
// large objects stay on the presigned path so their bytes never cross the
// lakeFS server.
// lakeFS answers 5xx when its own backing-store hop fails transiently (for
// example local ephemeral-port exhaustion under a burst of small PUTs). The
// upload targets a private hidden wip branch and overwrites one path, so
// repeating it is idempotent; a bounded backoff lets the pressure drain.
const (
	uploadAttempts   = 6
	uploadMaxBackoff = 4 * time.Second
)

// uploadBaseDelay is the first retry wait; tests shorten it.
var uploadBaseDelay = 250 * time.Millisecond

func (c *apiClient) uploadObject(branch, objectPath string, content []byte) error {
	endpoint := c.repositoryPath("branches/"+url.PathEscape(branch)+"/objects") +
		"?path=" + url.QueryEscape(objectPath)
	delay := uploadBaseDelay
	var err error
	for attempt := 0; attempt < uploadAttempts; attempt++ {
		var retry bool
		retry, err = c.uploadObjectOnce(endpoint, content)
		if err == nil || !retry {
			return err
		}
		if attempt < uploadAttempts-1 {
			time.Sleep(delay)
			if delay *= 2; delay > uploadMaxBackoff {
				delay = uploadMaxBackoff
			}
		}
	}
	return err
}

// uploadObjectOnce performs one upload attempt and reports whether a failure
// is transient enough to repeat.
func (c *apiClient) uploadObjectOnce(endpoint string, content []byte) (retry bool, err error) {
	req, err := http.NewRequest(http.MethodPost, c.api+endpoint, bytes.NewReader(content))
	if err != nil {
		return false, err
	}
	req.SetBasicAuth(c.accessKey, c.secretKey)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(content))
	resp, err := c.http.Do(req)
	if err != nil {
		return true, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs object upload: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		transient := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return transient, &apiError{Method: http.MethodPost, Path: endpoint, Status: resp.StatusCode, Body: string(body)}
	}
	return false, nil
}

type pagination struct {
	HasMore    bool   `json:"has_more"`
	NextOffset string `json:"next_offset"`
}

type objectStat struct {
	Path     string `json:"path"`
	PathType string `json:"path_type"`
}

type objectList struct {
	Pagination pagination   `json:"pagination"`
	Results    []objectStat `json:"results"`
}

func pageAmount(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > maxPageSize {
		return maxPageSize
	}
	return limit
}

func addPage(query url.Values, limit int, continuation string) {
	query.Set("amount", strconv.Itoa(pageAmount(limit)))
	if continuation != "" {
		query.Set("after", continuation)
	}
}
