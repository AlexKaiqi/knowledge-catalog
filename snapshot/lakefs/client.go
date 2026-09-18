package lakefs

import (
	"bytes"
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
			Timeout:       60 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		direct: &http.Client{Timeout: 5 * time.Minute},
	}, nil
}

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
	req, err := http.NewRequest(method, c.api+path, reader)
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
		return 0, nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs %s: %v", path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return resp.StatusCode, resp.Header, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs read response: %v", err)
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
	req, err := http.NewRequest(http.MethodGet, c.api+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.accessKey, c.secretKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs read object: %v", err)
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		resp.Body.Close()
		if location == "" {
			return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs presigned read omitted Location")
		}
		resp, err = c.direct.Get(location)
		if err != nil {
			return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs direct read: %v", err)
		}
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if readErr != nil {
		return nil, kernel.Fail(kernel.ErrTemporaryUnavailable, "lakefs read object body: %v", readErr)
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
