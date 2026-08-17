/**
 * MemoryRepository — a git-like state kernel over immutable object/tree/commit
 * with CAS ref updates. This is the Snapshot collection's version core, held
 * entirely in memory (Phase 0).
 *
 * Key invariant modeled here (K-05): objects and commits are immutable;
 * an update produces a NEW commit whose tree is a copy with the change applied,
 * while the old commit's tree is untouched — history is replayable.
 */

import { createHash } from "node:crypto";
import type {
  CommitChangeSet,
  CommitIdentity,
  Digest,
  KnowledgeValue,
  ObjectIdentity,
  Operation,
  ProvenanceEnvelope,
  ProvenanceTrace,
  RepositoryIdentity,
  Resolution,
} from "../../contracts/index.ts";
import { IngressError } from "../../contracts/errors.ts";

export interface ObjectRecord {
  readonly value: unknown;
  readonly digest: Digest;
  readonly pathHint: string;
  readonly schemaRef?: string;
  readonly provenance?: ProvenanceEnvelope;
}

interface CommitNode {
  readonly id: CommitIdentity;
  readonly parent: CommitIdentity | null;
  readonly entries: ReadonlyMap<ObjectIdentity, ObjectRecord>;
  readonly message?: string;
}

export function canonicalDigest(value: unknown): Digest {
  return createHash("sha256").update(stableStringify(value)).digest("hex");
}

function stableStringify(value: unknown): string {
  if (value === null) return "null";
  if (Array.isArray(value)) return `[${value.map(stableStringify).join(",")}]`;
  if (typeof value === "object") {
    const obj = value as Record<string, unknown>;
    const keys = Object.keys(obj).sort();
    return `{${keys.map((k) => `${JSON.stringify(k)}:${stableStringify(obj[k])}`).join(",")}}`;
  }
  return JSON.stringify(value);
}

export class MemoryRepository {
  readonly repositoryId: RepositoryIdentity;
  readonly defaultRef = "refs/heads/main";

  private readonly commits = new Map<CommitIdentity, CommitNode>();
  private readonly refs = new Map<string, CommitIdentity>();
  private seq = 0;

  constructor(repositoryId: RepositoryIdentity, rootCommitId: CommitIdentity) {
    this.repositoryId = repositoryId;
    this.commits.set(rootCommitId, {
      id: rootCommitId,
      parent: null,
      entries: new Map(),
      message: "root",
    });
    this.refs.set(this.defaultRef, rootCommitId);
  }

  head(ref: string = this.defaultRef): CommitIdentity {
    const c = this.refs.get(ref);
    if (!c) throw new IngressError("NON_FAST_FORWARD", `ref not found: ${ref}`);
    return c;
  }

  /** Apply a COMMIT ChangeSet atomically; returns the new commit id. */
  applyCommit(cs: CommitChangeSet): CommitIdentity {
    const current = this.head(cs.targetRef);
    if (current !== cs.expectedTargetCommit) {
      throw new IngressError(
        "NON_FAST_FORWARD",
        `expected ${cs.expectedTargetCommit} but ref is ${current}`,
      );
    }
    if (cs.baseCommit !== cs.expectedTargetCommit) {
      throw new IngressError("PRECONDITION_FAILED", "baseCommit must equal expectedTargetCommit");
    }
    const parent = this.commits.get(cs.baseCommit);
    if (!parent) throw new IngressError("PRECONDITION_FAILED", `unknown base commit ${cs.baseCommit}`);

    // Immutable update: copy parent tree, apply all operations, then commit.
    // If any operation throws, nothing is persisted (atomic ChangeSet, T3).
    const entries = new Map(parent.entries);
    for (const op of cs.operations) this.applyOp(entries, op, cs.provenance);

    const id = this.newCommitId(parent.id, entries);
    this.commits.set(id, { id, parent: parent.id, entries, message: cs.message });
    this.refs.set(cs.targetRef, id);
    return id;
  }

  private applyOp(
    entries: Map<ObjectIdentity, ObjectRecord>,
    op: Operation,
    provenance?: ProvenanceEnvelope,
  ): void {
    if (op.op === "PUT") {
      const existing = entries.get(op.address.objectId);
      const pc = op.precondition;
      if (pc?.type === "IF_ABSENT" && existing) {
        throw new IngressError("PRECONDITION_FAILED", `${op.address.objectId} already exists`);
      }
      if ((pc?.type === "IF_OBJECT_EQUALS" || pc?.type === "IF_DIGEST_EQUALS") && pc.digest) {
        if (!existing || existing.digest !== pc.digest) {
          throw new IngressError("PRECONDITION_FAILED", `digest mismatch for ${op.address.objectId}`);
        }
      }
      entries.set(op.address.objectId, {
        value: op.value,
        digest: canonicalDigest(op.value),
        pathHint: op.pathHint ?? op.address.objectId,
        schemaRef: op.schemaRef,
        provenance,
      });
      return;
    }

    // REMOVE
    const existing = entries.get(op.address.objectId);
    const pc = op.precondition;
    if ((pc?.type === "IF_OBJECT_EQUALS" || pc?.type === "IF_DIGEST_EQUALS") && pc.digest) {
      if (!existing || existing.digest !== pc.digest) {
        throw new IngressError("PRECONDITION_FAILED", `digest mismatch for ${op.address.objectId}`);
      }
    }
    if (!existing) {
      throw new IngressError("PRECONDITION_FAILED", `${op.address.objectId} does not exist`);
    }
    entries.delete(op.address.objectId);
  }

  private newCommitId(parentId: CommitIdentity, entries: ReadonlyMap<ObjectIdentity, ObjectRecord>): CommitIdentity {
    this.seq += 1;
    const keys = [...entries.keys()].sort().join(",");
    return createHash("sha256")
      .update(`${parentId}:${this.seq}:${keys}`)
      .digest("hex")
      .slice(0, 16);
  }

  /** RESOLVE one object at a commit; distinguishes REMOVED vs UNRESOLVED via history. */
  resolve(objectId: ObjectIdentity, commitId: CommitIdentity): Resolution {
    const node = this.commits.get(commitId);
    if (!node) throw new IngressError("KNOWLEDGE_REF_UNRESOLVED", `unknown commit ${commitId}`);
    const record = node.entries.get(objectId);
    if (record) {
      return {
        repository: this.repositoryId,
        commit: commitId,
        objectId,
        address: { kind: "Entity", objectId },
        pathHint: record.pathHint,
        digest: record.digest,
        schemaRef: record.schemaRef,
        status: "RESOLVED",
      };
    }
    // Not in this commit's tree: was it ever present in history?
    return {
      repository: this.repositoryId,
      commit: commitId,
      objectId,
      address: { kind: "Entity", objectId },
      pathHint: "",
      status: this.everExisted(objectId, node) ? "REMOVED" : "UNRESOLVED",
    };
  }

  private everExisted(objectId: ObjectIdentity, from: CommitNode): boolean {
    let node: CommitNode | undefined = from;
    while (node) {
      if (node.entries.has(objectId)) return true;
      node = node.parent ? this.commits.get(node.parent) : undefined;
    }
    return false;
  }

  read(objectId: ObjectIdentity, commitId: CommitIdentity): KnowledgeValue {
    const node = this.commits.get(commitId);
    if (!node) throw new IngressError("KNOWLEDGE_REF_UNRESOLVED", `unknown commit ${commitId}`);
    const record = node.entries.get(objectId);
    if (!record) throw new IngressError("KNOWLEDGE_REF_UNRESOLVED", `${objectId} not resolvable at ${commitId}`);
    return {
      knowledgeRef: { repository: this.repositoryId, object: objectId },
      repository: this.repositoryId,
      commit: commitId,
      address: { kind: "Entity", objectId },
      value: record.value,
      provenance: record.provenance,
    };
  }

  origin(objectId: ObjectIdentity, commitId: CommitIdentity): ProvenanceTrace {
    const value = this.read(objectId, commitId);
    const node = this.commits.get(commitId);
    const record = node?.entries.get(objectId);
    const chain: ProvenanceEnvelope[] = [];
    if (record?.provenance) chain.push(record.provenance);
    return {
      value: value.value,
      repository: this.repositoryId,
      commit: commitId,
      objectId,
      chain,
    };
  }

  search(query: string, commitId: CommitIdentity): KnowledgeValue[] {
    const node = this.commits.get(commitId);
    if (!node) throw new IngressError("KNOWLEDGE_REF_UNRESOLVED", `unknown commit ${commitId}`);
    const out: KnowledgeValue[] = [];
    for (const [objectId, record] of node.entries) {
      const text = stableStringify(record.value);
      if (text.toLowerCase().includes(query.toLowerCase())) {
        out.push({
          knowledgeRef: { repository: this.repositoryId, object: objectId },
          repository: this.repositoryId,
          commit: commitId,
          address: { kind: "Entity", objectId },
          value: record.value,
          provenance: record.provenance,
        });
      }
    }
    return out;
  }

  list(commitId: CommitIdentity): KnowledgeValue[] {
    const node = this.commits.get(commitId);
    if (!node) throw new IngressError("KNOWLEDGE_REF_UNRESOLVED", `unknown commit ${commitId}`);
    const out: KnowledgeValue[] = [];
    for (const [objectId, record] of node.entries) {
      out.push({
        knowledgeRef: { repository: this.repositoryId, object: objectId },
        repository: this.repositoryId,
        commit: commitId,
        address: { kind: "Entity", objectId },
        value: record.value,
        provenance: record.provenance,
      });
    }
    return out;
  }
}
