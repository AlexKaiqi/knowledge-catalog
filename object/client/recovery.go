package client

import (
	"context"
	"encoding/json"
	"kc/kernel"
	objectmodel "kc/object"
	"time"
)

// CommitWithReceiptRecovery posts once, then queries the original durable
// result after an uncertain transport outcome. It never repeats the mutation.
func CommitWithReceiptRecovery(ctx context.Context, remote Client, repo string, request CommitRequest, options RequestOptions, wait time.Duration) (any, error) {
	return submitWithReceiptRecovery(ctx, remote, request.CommandID, options, wait, func(output any) error { return remote.Commit(ctx, repo, request, options, output) })
}

func MaintainWithReceiptRecovery(ctx context.Context, remote Client, remove bool, request MaintenanceRequest, options RequestOptions, wait time.Duration) (any, error) {
	return submitWithReceiptRecovery(ctx, remote, request.CommandID, options, wait, func(output any) error {
		if remove {
			return remote.Remove(ctx, request, options, output)
		}
		return remote.Put(ctx, request, options, output)
	})
}

func submitWithReceiptRecovery(ctx context.Context, remote Client, id string, options RequestOptions, wait time.Duration, submit func(any) error) (any, error) {
	var output any
	err := submit(&output)
	code := kernel.CodeOf(err)
	if err == nil || wait <= 0 || (code != "" && code != kernel.ErrTemporaryUnavailable) {
		return output, err
	}
	deadline := time.Now().Add(wait)
	for {
		var raw any
		if lookupErr := remote.Receipt(ctx, id, options, &raw); lookupErr == nil {
			encoded, marshalErr := json.Marshal(raw)
			var entry struct {
				Receipt objectmodel.CommitReceipt `json:"receipt"`
			}
			if marshalErr == nil && json.Unmarshal(encoded, &entry) == nil && entry.Receipt.CommandID == id {
				return entry.Receipt, nil
			}
		}
		if ctx.Err() != nil || !time.Now().Before(deadline) {
			return output, err
		}
		remaining := time.Until(deadline)
		if remaining > 2*time.Second {
			remaining = 2 * time.Second
		}
		timer := time.NewTimer(remaining)
		select {
		case <-ctx.Done():
			timer.Stop()
			return output, err
		case <-timer.C:
		}
	}
}
