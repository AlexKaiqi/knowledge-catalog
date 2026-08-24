package connectorhost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kc/repository"
	"kc/writer"
)

type KCClient struct {
	BaseURL    string
	Principal  string
	HTTPClient *http.Client
}

func (c KCClient) RepoHead(ctx context.Context, repositoryID string) (string, error) {
	var status struct {
		Repos []struct {
			ID   string `json:"id"`
			Head string `json:"head"`
		} `json:"repos"`
	}
	if err := c.call(ctx, "status", map[string]any{}, &status); err != nil {
		return "", err
	}
	for _, repo := range status.Repos {
		if repo.ID == repositoryID {
			if repo.Head == "" {
				return "", fmt.Errorf("repository %s has no head", repositoryID)
			}
			return repo.Head, nil
		}
	}
	return "", fmt.Errorf("repository %s is not mounted in kc", repositoryID)
}

func (c KCClient) Commit(ctx context.Context, commandID string, changeSet repository.CommitChangeSet) (writer.CommitReceipt, error) {
	var receipt writer.CommitReceipt
	err := c.call(ctx, "commit", map[string]any{
		"command-id": commandID,
		"changeset":  changeSet,
		// The HTTP facade authorizes before it decodes the ChangeSet file, so the
		// same target coordinates are also supplied as flags for kc allow.
		"repo": changeSet.TargetRepository,
		"ref":  changeSet.TargetRef,
	}, &receipt)
	return receipt, err
}

func (c KCClient) call(ctx context.Context, verb string, body any, out any) error {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return fmt.Errorf("kc base URL is required")
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/"+verb, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Kc-Request-Id", "connector-host-"+time.Now().UTC().Format("20060102T150405.000000000"))
	if strings.TrimSpace(c.Principal) != "" {
		req.Header.Set("X-Kc-As", strings.TrimSpace(c.Principal))
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var fault struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &fault)
		if fault.Error.Code != "" || fault.Error.Message != "" {
			return fmt.Errorf("kc %s: %s: %s", verb, fault.Error.Code, fault.Error.Message)
		}
		return fmt.Errorf("kc %s returned %s: %s", verb, res.Status, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode kc %s response: %w", verb, err)
	}
	return nil
}
