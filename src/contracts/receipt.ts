/**
 * Durable Receipts — a success response means the write crossed the durable
 * boundary. "Received" alone is never a success receipt.
 */

import type { CommitIdentity, RepositoryIdentity } from "./identity.ts";

export type ReceiptDisposition = "APPLIED" | "REPLAYED";

export interface CommitReceipt {
  readonly receiptRef: string;
  readonly commandId: string;
  readonly surface: "COMMIT";
  readonly disposition: ReceiptDisposition;
  readonly result: {
    readonly repositoryId: RepositoryIdentity;
    readonly commitId: CommitIdentity;
    readonly targetRef: string;
    readonly oldCommit: CommitIdentity;
    readonly newCommit: CommitIdentity;
  };
}

export interface AppendReceipt {
  readonly receiptRef: string;
  readonly commandId: string;
  readonly surface: "APPEND";
  readonly disposition: ReceiptDisposition;
  readonly result: {
    readonly repositoryId: RepositoryIdentity;
    readonly streamRef: string;
    readonly cursor: string;
    readonly appended: readonly string[]; // record ids
  };
}
