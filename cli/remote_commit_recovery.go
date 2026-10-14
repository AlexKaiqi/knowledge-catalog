package cli

import (
	"context"
	kcclient "kc/client"

	objectclient "kc/object/client"
	"time"
)

// The publishing flow shares the SDK's post-once, query-original recovery.
func remoteCommitWithReceiptRecovery(ctx context.Context, remote *kcclient.Client, repository string, request kcclient.CommitRequest, options kcclient.RequestOptions, waitBudget time.Duration) (any, error) {
	return objectclient.CommitWithReceiptRecovery(ctx, remote.ObjectService(), repository, request, objectclient.RequestOptions{RequestID: options.RequestID}, waitBudget)
}
