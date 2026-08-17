/**
 * Ingestion & Grounding reference implementation — the two "first-order gaps"
 * (ingestion-and-grounding block). Both are thin orchestrations ABOVE the
 * COMMIT surface, NOT new surfaces:
 *
 *  - ingest(dir): scan a directory and produce a preview ChangeSet (one file
 *    = one object, provenance originKind=SOURCE).
 *  - reconcile(snapshot): set-diff an external snapshot against the current
 *    repo state and produce a preview ChangeSet.
 *  - groundingCitation(): project a KnowledgeValue into a GroundingCitation
 *    (PinnedKnowledgeRef + provenance summary) for the UI.
 */

import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import type {
  CommitChangeSet,
  CommitIdentity,
  GroundingCitation,
  KnowledgeValue,
  ObjectIdentity,
  Operation,
  RepositoryIdentity,
} from "../contracts/index.ts";
import { canonicalDigest } from "../digest.ts";
import { pinnedKnowledgeRef } from "../contracts/identity.ts";

export interface IngestPreview {
  readonly changeSet: CommitChangeSet;
  readonly files: readonly { path: string; objectId: ObjectIdentity }[];
}

/** Scan a directory; one file = one object (originKind SOURCE). Returns a preview. */
export function ingest(
  dir: string,
  repositoryId: RepositoryIdentity,
  baseCommit: CommitIdentity,
): IngestPreview {
  const operations: Operation[] = [];
  const files: { path: string; objectId: ObjectIdentity }[] = [];

  const walk = (d: string): void => {
    for (const entry of readdirSync(d, { withFileTypes: true })) {
      if (entry.name.startsWith(".")) continue; // skip hidden/.git
      const full = path.join(d, entry.name);
      if (entry.isDirectory()) {
        walk(full);
      } else if (entry.isFile()) {
        const rel = path.relative(dir, full);
        const objectId = rel.replace(/\.(md|json|ya?ml|txt)$/i, "").replace(/\\/g, "/");
        const content = readFileSync(full, "utf8");
        operations.push({
          op: "PUT",
          address: { kind: "Entity", objectId },
          value: content,
          pathHint: rel,
        });
        files.push({ path: rel, objectId });
      }
    }
  };
  walk(dir);

  return {
    files,
    changeSet: {
      targetRepository: repositoryId,
      targetRef: "refs/heads/main",
      baseCommit,
      expectedTargetCommit: baseCommit,
      operations,
      message: `ingest ${dir}`,
      provenance: { originKind: "SOURCE", sourceRefs: [dir] },
    },
  };
}

export interface ReconcilePreview {
  readonly changeSet: CommitChangeSet;
  readonly summary: { added: number; updated: number; removed: number };
}

/**
 * Set-diff an external snapshot (Map<objectId, value>) against the current
 * repo head (Map<objectId, digest>). Produce a preview ChangeSet; caller must
 * confirm and COMMIT — reconcile never commits on its own.
 */
export function reconcile(
  snapshot: ReadonlyMap<ObjectIdentity, unknown>,
  current: ReadonlyMap<ObjectIdentity, string>, // objectId -> current digest
  repositoryId: RepositoryIdentity,
  baseCommit: CommitIdentity,
): ReconcilePreview {
  const operations: Operation[] = [];
  let added = 0;
  let updated = 0;
  let removed = 0;

  for (const [objectId, value] of snapshot) {
    const digest = canonicalDigest(value);
    const existing = current.get(objectId);
    if (existing === undefined) {
      operations.push({ op: "PUT", address: { kind: "Entity", objectId }, value, precondition: { type: "IF_ABSENT" } });
      added += 1;
    } else if (existing !== digest) {
      operations.push({ op: "PUT", address: { kind: "Entity", objectId }, value, precondition: { type: "IF_DIGEST_EQUALS", digest: existing } });
      updated += 1;
    }
  }
  for (const objectId of current.keys()) {
    if (!snapshot.has(objectId)) {
      operations.push({ op: "REMOVE", address: { kind: "Entity", objectId }, reason: "absent-from-snapshot" });
      removed += 1;
    }
  }

  return {
    summary: { added, updated, removed },
    changeSet: {
      targetRepository: repositoryId,
      targetRef: "refs/heads/main",
      baseCommit,
      expectedTargetCommit: baseCommit,
      operations,
      message: "reconcile",
    },
  };
}

/** Project a KnowledgeValue into a GroundingCitation (for the UI). */
export function groundingCitation(value: KnowledgeValue): GroundingCitation {
  return {
    knowledgeRef: value.knowledgeRef,
    pinnedRef: pinnedKnowledgeRef(value.repository, value.commit, value.address.objectId),
    provenanceSummary: value.provenance
      ? {
          actorRef: value.provenance.actorRef,
          sourceRefs: value.provenance.sourceRefs,
          originKind: value.provenance.originKind,
        }
      : undefined,
  };
}
