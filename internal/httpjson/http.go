package httpjson

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"kc/kernel"
	"net/http"
	"strings"
	"time"
)

const MaxResponseBytes = 8 << 20
const gzipRequestBodyThreshold = 512 << 10

type Transport interface {
	Do(context.Context, string, *http.Request) (*http.Response, error)
}

// Do owns JSON request encoding and response limits; callers own typed routes.
func Do(ctx context.Context, transport Transport, baseURL, method, path string, input any, requestID string, output any) error {
	var body []byte
	var err error
	if input != nil {
		body, err = json.Marshal(input)
	}
	if err != nil {
		return err
	}
	contentEncoding := ""
	// Native lakeFS requests keep their original encoding: its API is not the
	// KC facade that owns transparent gzip decompression.
	if len(body) >= gzipRequestBodyThreshold && !strings.HasPrefix(path, "/api/v1/") {
		var compressed bytes.Buffer
		gz := gzip.NewWriter(&compressed)
		if _, err := gz.Write(body); err != nil {
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "compress request body: %v", err)
		}
		if err := gz.Close(); err != nil {
			return kernel.Fail(kernel.ErrTemporaryUnavailable, "compress request body: %v", err)
		}
		body = compressed.Bytes()
		contentEncoding = "gzip"
	}
	request, err := http.NewRequestWithContext(ctx, method, baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
		if contentEncoding != "" {
			request.Header.Set("Content-Encoding", contentEncoding)
		}
	}
	request.Header.Set("Accept", "application/json")
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		requestID = newRequestID()
	}
	request.Header.Set("X-Kc-Request-Id", requestID)
	response, err := transport.Do(ctx, baseURL, request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, MaxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > MaxResponseBytes {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "kc server response exceeds %d bytes", MaxResponseBytes)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var envelope struct {
			Error *kernel.IngressError `json:"error"`
		}
		if json.Unmarshal(raw, &envelope) == nil && envelope.Error != nil {
			return envelope.Error
		}
		if strings.HasPrefix(path, "/api/v1/") {
			return nativeJSONError(response.StatusCode, raw)
		}
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "kc server returned HTTP %d", response.StatusCode)
	}
	if output == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := kernel.UnmarshalJSON(raw, output); err != nil {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "decode kc server response: %v", err)
	}
	return nil
}

func newRequestID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("kc-client-%d", time.Now().UnixNano())
	}
	return "kc-client-" + hex.EncodeToString(raw)
}
