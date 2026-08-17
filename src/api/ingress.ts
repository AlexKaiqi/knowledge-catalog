/**
 * Ingress — the write boundary. Executes COMMIT / APPEND mechanically:
 * idempotency (command_id), target routing, then delegate to the Repository
 * contract (store-agnostic). It does NOT judge content correctness.
 */

import type {
  AppendEntries,
  AppendReceipt,
  CommitChangeSet,
  CommitReceipt,
} from "../contracts/index.ts";
import { IngressError } from "../contracts/errors.ts";
import { canonicalDigest } from "../digest.ts";
import type { Store } from "../store.ts";

type AnyReceipt = CommitReceipt | AppendReceipt;

export class Ingress {
  private readonly idempotency = new Map<string, { digest: string; receipt: AnyReceipt }>();

  constructor(private readonly store: Store) {}

  commit(commandId: string, cs: CommitChangeSet): CommitReceipt {
    const repo = this.store.repos.get(cs.targetRepository);
    if (!repo) {
      throw new IngressError("TARGET_REPOSITORY_DENIED", `unknown repository ${cs.targetRepository}`);
    }
    const digest = canonicalDigest(cs);
    const prior = this.idempotency.get(commandId);
    if (prior) {
      if (prior.digest !== digest) {
        throw new IngressError("IDEMPOTENCY_CONFLICT", `command ${commandId} reused with different payload`);
      }
      return { ...(prior.receipt as CommitReceipt), disposition: "REPLAYED" };
    }

    const oldCommit = repo.head(cs.targetRef);
    const commitId = repo.applyCommit(cs);
    const receipt: CommitReceipt = {
      receiptRef: `receipt:commit:${commitId}`,
      commandId,
      surface: "COMMIT",
      disposition: "APPLIED",
      result: {
        repositoryId: cs.targetRepository,
        commitId,
        targetRef: cs.targetRef,
        oldCommit,
        newCommit: commitId,
      },
    };
    this.idempotency.set(commandId, { digest, receipt });
    return receipt;
  }

  append(commandId: string, ae: AppendEntries): AppendReceipt {
    const digest = canonicalDigest(ae);
    const prior = this.idempotency.get(commandId);
    if (prior) {
      if (prior.digest !== digest) {
        throw new IngressError("IDEMPOTENCY_CONFLICT", `command ${commandId} reused with different payload`);
      }
      return { ...(prior.receipt as AppendReceipt), disposition: "REPLAYED" };
    }

    const repo = this.store.repos.get(ae.targetRepository);
    if (!repo) {
      throw new IngressError("TARGET_REPOSITORY_DENIED", `unknown repository ${ae.targetRepository}`);
    }
    const recordIds = repo.append(ae.streamRef, ae.entries);
    const receipt: AppendReceipt = {
      receiptRef: `receipt:append:${commandId}`,
      commandId,
      surface: "APPEND",
      disposition: "APPLIED",
      result: {
        repositoryId: ae.targetRepository,
        streamRef: ae.streamRef,
        cursor: repo.streamCursor(ae.streamRef),
        appended: recordIds,
      },
    };
    this.idempotency.set(commandId, { digest, receipt });
    return receipt;
  }
}
