/**
 * Writer — the write API. Executes COMMIT / APPEND mechanically:
 * idempotency (command_id), target routing, then delegate to the Repository
 * contract (store-agnostic). It does NOT judge content correctness.
 *
 * command_id covers the request body. CAS fields (expectedTargetCommit /
 * expectedCursor) are part of that body. A retry must resubmit the same body;
 * filling "current head" again is a different command.
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
import type { IdempotencyStore } from "./idempotency.ts";

type AnyReceipt = CommitReceipt | AppendReceipt;

export type WriterRequest =
  | { readonly kind: "COMMIT"; readonly changeSet: CommitChangeSet }
  | { readonly kind: "APPEND"; readonly entries: AppendEntries };

export interface WriterIdempotencyEntry {
  readonly commandId: string;
  readonly digest: string;
  readonly receipt: AnyReceipt;
  readonly request?: WriterRequest;
}

export class Writer {
  private readonly idempotency = new Map<string, Omit<WriterIdempotencyEntry, "commandId">>();

  constructor(
    private readonly store: Store,
    private readonly log?: IdempotencyStore,
  ) {
    if (log) this.loadIdempotency(log.load());
  }

  lookup(commandId: string): WriterIdempotencyEntry | undefined {
    const entry = this.idempotency.get(commandId);
    if (!entry) return undefined;
    return { commandId, ...entry };
  }

  dumpIdempotency(): readonly WriterIdempotencyEntry[] {
    return [...this.idempotency.entries()].map(([commandId, entry]) => ({
      commandId,
      ...entry,
    }));
  }

  loadIdempotency(entries: readonly WriterIdempotencyEntry[]): void {
    this.idempotency.clear();
    for (const entry of entries) {
      this.idempotency.set(entry.commandId, {
        digest: entry.digest,
        receipt: entry.receipt,
        request: entry.request,
      });
    }
  }

  /**
   * First call fills CAS from the current Ref head.
   * Retry reuses the stored ChangeSet's CAS tokens and compares the rest.
   * Same ops → REPLAYED; different ops → IDEMPOTENCY_CONFLICT.
   */
  commitIntent(
    commandId: string,
    intent: Omit<CommitChangeSet, "baseCommit" | "expectedTargetCommit"> & {
      baseCommit?: string;
      expectedTargetCommit?: string;
    },
  ): CommitReceipt {
    const prior = this.lookup(commandId);
    if (prior?.request?.kind === "COMMIT") {
      const stored = prior.request.changeSet;
      return this.commit(commandId, {
        ...intent,
        targetRef: intent.targetRef,
        targetRepository: intent.targetRepository,
        baseCommit: stored.baseCommit,
        expectedTargetCommit: stored.expectedTargetCommit,
      });
    }
    const repo = this.store.repos.get(intent.targetRepository);
    if (!repo) {
      throw new IngressError("TARGET_REPOSITORY_DENIED", `unknown repository ${intent.targetRepository}`);
    }
    const head = repo.head(intent.targetRef);
    return this.commit(commandId, {
      ...intent,
      baseCommit: intent.baseCommit ?? head,
      expectedTargetCommit: intent.expectedTargetCommit ?? head,
    });
  }

  appendIntent(
    commandId: string,
    intent: Omit<AppendEntries, "expectedCursor"> & { expectedCursor?: string },
  ): AppendReceipt {
    const prior = this.lookup(commandId);
    if (prior?.request?.kind === "APPEND") {
      return this.append(commandId, {
        ...intent,
        expectedCursor: prior.request.entries.expectedCursor,
      });
    }
    return this.append(commandId, intent);
  }

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
    this.remember(commandId, digest, receipt, { kind: "COMMIT", changeSet: cs });
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
    const recordIds = repo.append(ae.streamRef, ae.entries, ae.expectedCursor);
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
    this.remember(commandId, digest, receipt, { kind: "APPEND", entries: ae });
    return receipt;
  }

  private remember(commandId: string, digest: string, receipt: AnyReceipt, request: WriterRequest): void {
    this.idempotency.set(commandId, { digest, receipt, request });
    this.log?.save(this.dumpIdempotency());
  }
}
