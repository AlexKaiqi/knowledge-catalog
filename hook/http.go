package hook

import (
	"bytes"
	"context"
	"net/http"
	"time"

	"kc/kernel"
)

const httpTimeout = 10 * time.Second

func postURL(rawURL string, body []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(append(append([]byte{}, body...), '\n')))
	if err != nil {
		return kernel.Fail(kernel.ErrHookDenied, "hook url: %s", err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Timeout: httpTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return kernel.Fail(kernel.ErrTemporaryUnavailable, "hook url %s: %s", rawURL, err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return kernel.Fail(kernel.ErrHookDenied, "hook url %s returned %d", rawURL, resp.StatusCode)
	}
	return nil
}
