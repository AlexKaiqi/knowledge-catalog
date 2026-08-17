/**
 * FileGitRepository — THE repository implementation: real files + real git.
 * This is the store; there is no separate "memory semantics" — git IS the
 * version kernel (object/tree/commit/ref/branch/merge/CAS).
 *
 * Identity carrier decision (minimal-core-contracts A.2):
 *  - object_id lives INSIDE the file (frontmatter), not in the path.
 *  - path is a movable `pathHint`; address-map is a rebuildable projection.
 *
 * Collections:
 *  - Snapshot  -> files under the repo root, versioned by git.
 *  - Append    -> JSONL side files under streams/ (gitignored, NOT in the git tree):
 *                 append-only streams keep non-git semantics (ADR-005).
 */

import { execFileSync } from "node:child_process";
import {
  appendFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import type {
  AppendEntry,
  CommitChangeSet,
  CommitIdentity,
  Digest,
  KnowledgeValue,
  ObjectIdentity,
  Operation,
  ProvenanceEnvelope,
  ProvenanceTrace,
  Repository,
  RepositoryIdentity,
  Resolution,
} from "../../contracts/index.ts";
import { IngressError } from "../../contracts/errors.ts";
import { canonicalDigest } from "../../digest.ts";

export interface ParsedObject {
  readonly objectId: ObjectIdentity;
  readonly pathHint?: string;
  readonly schemaRef?: string;
  readonly provenance?: ProvenanceEnvelope;
  readonly value: unknown;
}

interface AppendRecord {
  readonly recordId: string;
  readonly eventId: string;
  readonly payload: unknown;
  readonly digest: string;
  readonly recordedAt: string;
  readonly schemaRef?: string;
}

interface FileObjectState extends ParsedObject {
  readonly path: string; // relative to rootDir
  readonly digest: Digest;
}

function serializeFile(o: {
  objectId: ObjectIdentity;
  pathHint?: string;
  schemaRef?: string;
  provenance?: ProvenanceEnvelope;
  value: unknown;
}): string {
  const fm: string[] = [`object_id: ${o.objectId}`];
  if (o.pathHint) fm.push(`path_hint: ${o.pathHint}`);
  if (o.schemaRef) fm.push(`schema_ref: ${o.schemaRef}`);
  if (o.provenance) fm.push(`provenance: ${JSON.stringify(o.provenance)}`);
  return `---\n${fm.join("\n")}\n---\n${JSON.stringify(o.value, null, 2)}\n`;
}

function parseFile(content: string): ParsedObject | null {
  const lines = content.split("\n");
  if (lines[0] !== "---") return null;
  const endIdx = lines.indexOf("---", 1);
  if (endIdx === -1) return null;

  const obj: Record<string, unknown> = {};
  for (const line of lines.slice(1, endIdx)) {
    const idx = line.indexOf(":");
    if (idx === -1) continue;
    const key = line.slice(0, idx).trim();
    const value = line.slice(idx + 1).trim();
    if (key === "provenance") obj[key] = value ? JSON.parse(value) : undefined;
    else obj[key] = value;
  }
  const objectId = obj["object_id"];
  if (typeof objectId !== "string" || !objectId) return null;

  const body = lines.slice(endIdx + 1).join("\n").trim();
  let value: unknown = body;
  try {
    value = JSON.parse(body);
  } catch {
    // keep raw text body
  }
  return {
    objectId,
    pathHint: typeof obj["path_hint"] === "string" ? obj["path_hint"] : undefined,
    schemaRef: typeof obj["schema_ref"] === "string" ? obj["schema_ref"] : undefined,
    provenance: (obj["provenance"] as ProvenanceEnvelope) ?? undefined,
    value,
  };
}

function git(cwd: string, ...args: string[]): string {
  return execFileSync("git", args, {
    cwd,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "ignore"],
  }).trim();
}

function gitOk(cwd: string, ...args: string[]): boolean {
  try {
    git(cwd, ...args);
    return true;
  } catch {
    return false;
  }
}

function checkoutName(ref: string): string {
  return ref.startsWith("refs/heads/") ? ref.slice("refs/heads/".length) : ref;
}

function safeRelativePath(value: string): string {
  if (!value || path.isAbsolute(value)) {
    throw new IngressError("PRECONDITION_FAILED", `path must be relative: ${value}`);
  }
  const normalized = path.normalize(value);
  if (normalized === ".." || normalized.startsWith(`..${path.sep}`)) {
    throw new IngressError("PRECONDITION_FAILED", `path escapes repository root: ${value}`);
  }
  return normalized;
}

function streamPath(rootDir: string, streamRef: string): string {
  return path.join(rootDir, "streams", `${encodeURIComponent(streamRef)}.jsonl`);
}

const GIT_IDENTITY = [
  "-c",
  "user.name=knowledge-catalog",
  "-c",
  "user.email=dev@knowledge-catalog.local",
] as const;

export class FileGitRepository implements Repository {
  readonly repositoryId: RepositoryIdentity;
  readonly rootDir: string;

  constructor(rootDir: string, repositoryId: RepositoryIdentity) {
    this.rootDir = rootDir;
    this.repositoryId = repositoryId;
    mkdirSync(rootDir, { recursive: true });
    if (!existsSync(path.join(rootDir, ".git"))) {
      execFileSync("git", ["init", "-q"], { cwd: rootDir });
      git(rootDir, "branch", "-M", "main"); // normalize default branch to main
      git(rootDir, ...GIT_IDENTITY, "commit", "--allow-empty", "-q", "-m", "root");
    }
    // Append streams are side files, not part of the git snapshot tree. Use the
    // local exclude file so the adapter never mutates a user's tracked .gitignore.
    const excludePath = path.join(rootDir, ".git", "info", "exclude");
    const exclude = existsSync(excludePath) ? readFileSync(excludePath, "utf8") : "";
    if (!exclude.split("\n").includes("streams/")) {
      writeFileSync(excludePath, `${exclude}${exclude && !exclude.endsWith("\n") ? "\n" : ""}streams/\n`, "utf8");
    }
  }

  // ---- version core (real git) ----

  head(ref = "HEAD"): CommitIdentity {
    try {
      return git(this.rootDir, "rev-parse", ref);
    } catch {
      return "";
    }
  }

  getRef(ref: string): CommitIdentity | undefined {
    return gitOk(this.rootDir, "rev-parse", "--verify", ref)
      ? git(this.rootDir, "rev-parse", ref)
      : undefined;
  }

  createRef(ref: string, commitId: CommitIdentity): void {
    if (this.getRef(ref) !== undefined) {
      throw new IngressError("PRECONDITION_FAILED", `ref ${ref} already exists`);
    }
    if (!gitOk(this.rootDir, "cat-file", "-e", `${commitId}^{commit}`)) {
      throw new IngressError("PRECONDITION_FAILED", `unknown commit ${commitId}`);
    }
    git(this.rootDir, "update-ref", ref, commitId);
  }

  merge(targetRef: string, candidateCommit: CommitIdentity, expectedTargetCommit: CommitIdentity): CommitIdentity {
    // git update-ref <ref> <new> <old> is an exact CAS (K-06).
    const checkedOutRef = gitOk(this.rootDir, "symbolic-ref", "-q", "HEAD")
      ? git(this.rootDir, "symbolic-ref", "-q", "HEAD")
      : undefined;
    if (!gitOk(this.rootDir, "merge-base", "--is-ancestor", expectedTargetCommit, candidateCommit)) {
      throw new IngressError("NON_FAST_FORWARD", `${candidateCommit} is not a descendant of ${expectedTargetCommit}`);
    }
    if (!gitOk(this.rootDir, "update-ref", targetRef, candidateCommit, expectedTargetCommit)) {
      throw new IngressError("NON_FAST_FORWARD", `expected ${expectedTargetCommit} but ref is ${this.getRef(targetRef)}`);
    }
    // update-ref moves the ref, not the worktree. Keep a checked-out target in sync.
    if (checkedOutRef === targetRef) git(this.rootDir, "reset", "--hard", "-q", candidateCommit);
    return candidateCommit;
  }

  // ---- write ----

  /** Apply a COMMIT ChangeSet atomically (validate in memory, then write + git commit). */
  applyCommit(cs: CommitChangeSet): CommitIdentity {
    if (cs.targetRepository !== this.repositoryId) {
      throw new IngressError("TARGET_REPOSITORY_DENIED", `target ${cs.targetRepository} does not match ${this.repositoryId}`);
    }
    if (cs.baseCommit !== cs.expectedTargetCommit) {
      throw new IngressError("PRECONDITION_FAILED", "baseCommit must equal expectedTargetCommit");
    }
    const targetRef = cs.targetRef;
    const isHead = targetRef === "HEAD";
    const current = isHead ? this.head() : (this.getRef(targetRef) ?? "");
    if (current !== cs.expectedTargetCommit) {
      throw new IngressError("NON_FAST_FORWARD", `expected ${cs.expectedTargetCommit} but ref is ${current}`);
    }

    // Ref identity matters even when two branches point to the same commit.
    const checkedOutRef = gitOk(this.rootDir, "symbolic-ref", "-q", "HEAD")
      ? git(this.rootDir, "symbolic-ref", "-q", "HEAD")
      : undefined;
    const needsSwitch = !isHead && checkedOutRef !== targetRef;
    const restore = needsSwitch ? (checkedOutRef ?? this.head()) : null;
    if (needsSwitch) git(this.rootDir, "checkout", "-q", checkoutName(targetRef));

    try {
      const dirty = git(this.rootDir, "status", "--porcelain");
      if (dirty) {
        throw new IngressError("PRECONDITION_FAILED", "working tree must be clean before protocol COMMIT");
      }
      // Load current state into memory (address-map is a rebuildable projection).
      const state = this.scan();
      const toWrite = new Map<string, string>(); // relative path -> content
      const toDelete = new Set<string>();

      for (const op of cs.operations) {
        if (op.op === "PUT") {
          const existing = state.get(op.address.objectId);
          const pc = op.precondition;
          if (pc?.type === "IF_ABSENT" && existing) {
            throw new IngressError("PRECONDITION_FAILED", `${op.address.objectId} already exists`);
          }
          if ((pc?.type === "IF_OBJECT_EQUALS" || pc?.type === "IF_DIGEST_EQUALS") && pc.digest) {
            if (!existing || existing.digest !== pc.digest) {
              throw new IngressError("PRECONDITION_FAILED", `digest mismatch for ${op.address.objectId}`);
            }
          }
          const newPath = safeRelativePath(op.pathHint ?? `objects/${op.address.objectId}.json`);
          if (existing && existing.path !== newPath) toDelete.add(existing.path); // move
          toWrite.set(newPath, serializeFile({
            objectId: op.address.objectId,
            pathHint: op.pathHint !== undefined ? newPath : existing?.pathHint,
            schemaRef: op.schemaRef ?? existing?.schemaRef,
            provenance: cs.provenance ?? existing?.provenance,
            value: op.value,
          }));
          continue;
        }

        // REMOVE
        const existing = state.get(op.address.objectId);
        if (!existing) {
          throw new IngressError("PRECONDITION_FAILED", `${op.address.objectId} does not exist`);
        }
        toDelete.add(existing.path);
      }

      // Persist (only reached if all operations validated).
      for (const p of toDelete) rmSync(path.join(this.rootDir, p), { force: true });
      for (const [p, content] of toWrite) {
        const full = path.join(this.rootDir, p);
        mkdirSync(path.dirname(full), { recursive: true });
        writeFileSync(full, content, "utf8");
      }

      git(this.rootDir, "add", "-A");
      git(this.rootDir, ...GIT_IDENTITY, "commit", "--allow-empty", "-q", "-m", cs.message ?? "commit");
      const newCommit = this.head();
      if (restore) git(this.rootDir, "checkout", "-q", checkoutName(restore));
      return newCommit;
    } catch (e) {
      if (restore) {
        try {
          git(this.rootDir, "checkout", "-q", checkoutName(restore));
        } catch {
          // best effort restore
        }
      }
      throw e;
    }
  }

  /** Append entries to a JSONL side stream (append-only; event-id idempotent). */
  append(streamRef: string, entries: readonly AppendEntry[]): readonly string[] {
    const file = streamPath(this.rootDir, streamRef);
    mkdirSync(path.dirname(file), { recursive: true });

    const byEventId = new Map<string, AppendRecord>();
    let count = 0;
    if (existsSync(file)) {
      for (const line of readFileSync(file, "utf8").split("\n").filter(Boolean)) {
        const rec = JSON.parse(line) as AppendRecord;
        byEventId.set(rec.eventId, rec);
        count += 1;
      }
    }

    const appended: string[] = [];
    const newLines: string[] = [];
    for (const entry of entries) {
      const digest = canonicalDigest(entry.payload);
      const prior = byEventId.get(entry.eventId);
      if (prior) {
        if (prior.digest !== digest) {
          throw new IngressError("EVENT_ID_CONFLICT", `event id ${entry.eventId} already used with different payload`);
        }
        appended.push(prior.recordId); // idempotent replay
        continue;
      }
      const record: AppendRecord = {
        recordId: `rec-${count + newLines.length + 1}`,
        eventId: entry.eventId,
        payload: entry.payload,
        digest,
        recordedAt: entry.observedAt ?? new Date().toISOString(),
        schemaRef: entry.schemaRef,
      };
      newLines.push(JSON.stringify(record));
      byEventId.set(entry.eventId, record);
      appended.push(record.recordId);
    }

    if (newLines.length) appendFileSync(file, newLines.join("\n") + "\n", "utf8");
    return appended;
  }

  streamCursor(streamRef: string): string {
    const file = streamPath(this.rootDir, streamRef);
    if (!existsSync(file)) return "0";
    return String(readFileSync(file, "utf8").split("\n").filter(Boolean).length);
  }

  // ---- read ----

  resolve(objectId: ObjectIdentity, commitId: CommitIdentity): Resolution {
    const state = this.scanAt(commitId);
    const record = state.get(objectId);
    if (record) {
      return {
        repository: this.repositoryId,
        commit: commitId,
        objectId,
        address: { kind: "Entity", objectId },
        pathHint: record.pathHint ?? record.path,
        digest: record.digest,
        schemaRef: record.schemaRef,
        status: "RESOLVED",
      };
    }
    return {
      repository: this.repositoryId,
      commit: commitId,
      objectId,
      address: { kind: "Entity", objectId },
      pathHint: "",
      status: this.everExisted(objectId) ? "REMOVED" : "UNRESOLVED",
    };
  }

  read(objectId: ObjectIdentity, commitId: CommitIdentity): KnowledgeValue {
    const record = this.scanAt(commitId).get(objectId);
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
    const record = this.scanAt(commitId).get(objectId);
    const chain: ProvenanceEnvelope[] = [];
    if (record?.provenance) chain.push(record.provenance);
    return { value: value.value, repository: this.repositoryId, commit: commitId, objectId, chain };
  }

  search(query: string, commitId: CommitIdentity): KnowledgeValue[] {
    const out: KnowledgeValue[] = [];
    for (const [objectId, record] of this.scanAt(commitId)) {
      if (JSON.stringify(record.value).toLowerCase().includes(query.toLowerCase())) {
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
    const out: KnowledgeValue[] = [];
    for (const [objectId, record] of this.scanAt(commitId)) {
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

  // ---- internals ----

  /** Scan working tree for knowledge-object files. */
  private scan(): Map<ObjectIdentity, FileObjectState> {
    const result = new Map<ObjectIdentity, FileObjectState>();
    const walk = (dir: string): void => {
      for (const entry of readdirSync(dir, { withFileTypes: true })) {
        if (entry.name === ".git" || entry.name === "streams") continue;
        const full = path.join(dir, entry.name);
        if (entry.isDirectory()) walk(full);
        else if (entry.isFile() && /\.(json|md|ya?ml|txt)$/i.test(entry.name)) {
          let parsed: ParsedObject | null;
          try {
            parsed = parseFile(readFileSync(full, "utf8"));
          } catch {
            continue; // not a knowledge file
          }
          if (!parsed) continue;
          if (result.has(parsed.objectId)) {
            throw new IngressError("OBJECT_ID_CONFLICT", `duplicate object_id ${parsed.objectId}`);
          }
          result.set(parsed.objectId, {
            ...parsed,
            path: path.relative(this.rootDir, full),
            digest: canonicalDigest(parsed.value),
          });
        }
      }
    };
    walk(this.rootDir);
    return result;
  }

  /** Scan the tree at a specific commit (via git show). */
  private scanAt(commitId: CommitIdentity): Map<ObjectIdentity, FileObjectState> {
    if (commitId === this.head()) return this.scan();
    const result = new Map<ObjectIdentity, FileObjectState>();
    try {
      const files = git(this.rootDir, "ls-tree", "-r", "--name-only", commitId).split("\n").filter(Boolean);
      for (const rel of files) {
        if (!/\.(json|md|ya?ml|txt)$/i.test(rel)) continue;
        let parsed: ParsedObject | null;
        try {
          parsed = parseFile(git(this.rootDir, "show", `${commitId}:${rel}`));
        } catch {
          continue;
        }
        if (!parsed) continue;
        if (result.has(parsed.objectId)) {
          throw new IngressError("OBJECT_ID_CONFLICT", `duplicate object_id ${parsed.objectId} at ${commitId}`);
        }
        result.set(parsed.objectId, { ...parsed, path: rel, digest: canonicalDigest(parsed.value) });
      }
    } catch (e) {
      if (e instanceof IngressError) throw e;
      // unknown commit
    }
    return result;
  }

  private everExisted(objectId: ObjectIdentity): boolean {
    try {
      const log = git(this.rootDir, "log", "--all", "--format=%H");
      for (const commitId of log.split("\n").filter(Boolean)) {
        if (this.scanAt(commitId).has(objectId)) return true;
      }
    } catch {
      // ignore
    }
    return false;
  }
}
