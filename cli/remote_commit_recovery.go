package cli

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	kcclient "kc/client"
	"kc/kernel"
	"kc/knowledge/writer"
)

// commitReceiptWaitEnv optionally overrides how long a remote commit keeps
// polling the durable receipt ledger after the POST outcome became unknown.
// Accepts a Go duration ("90s", "10m") or plain seconds; "0" disables polling.
const commitReceiptWaitEnv = "KC_COMMIT_RECEIPT_WAIT"

const (
	defaultCommitReceiptWait = 10 * time.Minute
	receiptPollInterval      = 2 * time.Second
)

// commitReceiptWaitBudget is the polling budget used by remote writer commits.
// It only covers receipt lookups after an outcome-unknown POST; the POST
// itself keeps the client transport's own timeout.
func commitReceiptWaitBudget() time.Duration {
	raw := strings.TrimSpace(os.Getenv(commitReceiptWaitEnv))
	if raw == "" {
		return defaultCommitReceiptWait
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if parsed, err := time.ParseDuration(raw); err == nil {
		return parsed
	}
	return defaultCommitReceiptWait
}

// remoteCommitWithReceiptRecovery submits one Writer commit and returns the
// authoritative outcome. A POST that fails without a deterministic server
// rejection leaves the command applying server-side: the apply chain does not
// ride the request context, and the command ledger durably records
// PENDING→APPLIED before the authority is touched. Instead of inventing a
// failure, the helper polls the existing receipt endpoint and returns the
// ledger's receipt. It never re-submits the commit — replay stays the
// server's decision via command_id and digest.
func remoteCommitWithReceiptRecovery(ctx context.Context, client *kcclient.Client, repository string, request kcclient.CommitRequest, options kcclient.RequestOptions, waitBudget time.Duration) (any, error) {
	var output any
	err := client.WriterService().Commit(ctx, repository, request, options, &output)
	if err == nil || waitBudget <= 0 || !commitOutcomeUnknown(err) {
		return output, err
	}
	deadline := time.Now().Add(waitBudget)
	for {
		receipt, found, lookupErr := remoteAppliedReceipt(ctx, client, request.CommandID, options)
		if lookupErr == nil && found {
			return receipt, nil
		}
		if ctx.Err() != nil || !time.Now().Before(deadline) {
			return output, err
		}
		sleep := time.NewTimer(receiptPollInterval)
		select {
		case <-ctx.Done():
			sleep.Stop()
			return output, err
		case <-sleep.C:
		}
	}
}

// commitOutcomeUnknown reports whether err leaves the commit outcome
// undecided. Deterministic kernel rejections (CAS, authorization, usage,
// validation, idempotency conflict) mean the server refused the command and
// are returned as-is. Only transport losses and temporary server failures
// justify polling the ledger.
func commitOutcomeUnknown(err error) bool {
	code := kernel.CodeOf(err)
	return code == "" || code == kernel.ErrTemporaryUnavailable
}

// remoteAppliedReceipt returns the durable receipt for commandID once the
// ledger has one. A missing command or a PENDING entry (no receipt yet) is
// reported as not found.
func remoteAppliedReceipt(ctx context.Context, client *kcclient.Client, commandID string, options kcclient.RequestOptions) (writer.CommitReceipt, bool, error) {
	var raw any
	if err := client.WriterService().Receipt(ctx, commandID, options, &raw); err != nil {
		return writer.CommitReceipt{}, false, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return writer.CommitReceipt{}, false, err
	}
	var entry struct {
		CommandID string              `json:"commandId"`
		Status    string              `json:"status"`
		Receipt   writer.CommitReceipt `json:"receipt"`
	}
	if err := json.Unmarshal(encoded, &entry); err != nil {
		return writer.CommitReceipt{}, false, err
	}
	if entry.Receipt.CommandID == "" {
		return writer.CommitReceipt{}, false, nil
	}
	return entry.Receipt, true, nil
}
