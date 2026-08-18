/**
 * FileGitRepository — THE repository implementation: real files + real git.
 * This is the store; there is no separate "memory semantics" — git IS the
 * version kernel (object/tree/commit/ref/branch/merge/CAS).
 *
 * Identity carrier decision (minimal-core-contracts A.2 / A.3):
 *  - object_id lives INSIDE the file (frontmatter), not in the path.
 *  - path is a movable `pathHint`; address-map is a rebuildable projection.
 *  - uniqueness is Address (object_id + aspect_name + member_key), DataHub-style.
 *    One file = one maintenance unit. Same object_id may have many aspect files.
 *  - PUT Aspect replaces that unit only (not a JSON PATCH of the entity blob).
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
  KnowledgeAddress,
  KnowledgeValue,
  ObjectIdentity,
  Operation,
  ProvenanceEnvelope,
  ProvenanceTrace,
  Repository,
  RepositoryIdentity,
  Resolution,
} from "../../contracts/index.ts";
import {
  addressKey,
  assertWritableAddress,
  isEntityBlob,
} from "../../contracts/address.ts";
import { IngressError } from "../../contracts/errors.ts";
import { canonicalDigest } from "../../digest.ts";

export interface ParsedObject {
  readonly objectId: ObjectIdentity;
  readonly address: KnowledgeAddress;
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

interface ScanIndex {
  readonly units: Map<string, FileObjectState>;
  readonly byObject: Map<ObjectIdentity, FileObjectState[]>;
}

function serializeFile(o: {
  address: KnowledgeAddress;
  pathHint?: string;
  schemaRef?: string;
  provenance?: ProvenanceEnvelope;
  value: unknown;
}): string {
  const fm: string[] = [`object_id: ${o.address.objectId}`];
  if (o.address.aspectName) fm.push(`aspect_name: ${o.address.aspectName}`);
  if (o.address.memberKey) fm.push(`member_key: ${o.address.memberKey}`);
  if (o.address.kind !== "Entity") fm.push(`kind: ${o.address.kind}`);
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

  const aspectName = typeof obj["aspect_name"] === "string" && obj["aspect_name"] ? obj["aspect_name"] : undefined;
  const memberKey = typeof obj["member_key"] === "string" && obj["member_key"] ? obj["member_key"] : undefined;
  const kindField = obj["kind"];
  const address = inferAddress(objectId, aspectName, memberKey, typeof kindField === "string" ? kindField : undefined);

  const body = lines.slice(endIdx + 1).join("\n").trim();
  let value: unknown = body;
  try {
    value = JSON.parse(body);
  } catch {
    // keep raw text body
  }
  return {
    objectId,
    address,
    pathHint: typeof obj["path_hint"] === "string" ? obj["path_hint"] : undefined,
    schemaRef: typeof obj["schema_ref"] === "string" ? obj["schema_ref"] : undefined,
    provenance: (obj["provenance"] as ProvenanceEnvelope) ?? undefined,
    value,
  };
}

function inferAddress(
  objectId: ObjectIdentity,
  aspectName: string | undefined,
  memberKey: string | undefined,
  kindField: string | undefined,
): KnowledgeAddress {
  if (memberKey && aspectName) {
    return { kind: "Member", objectId, aspectName, memberKey };
  }
  if (aspectName) {
    const kind = kindField === "Record" ? "Record" : "Aspect";
    return { kind, objectId, aspectName };
  }
  const kind = kindField === "Relation" ? "Relation" : "Entity";
  return { kind, objectId };
}

function defaultPath(address: KnowledgeAddress): string {
  if (address.memberKey && address.aspectName) {
    return `objects/${address.objectId}/${address.aspectName}/${address.memberKey}.json`;
  }
  if (address.aspectName) {
    return `objects/${address.objectId}/${address.aspectName}.json`;
  }
  return `objects/${address.objectId}.json`;
}

function emptyIndex(): ScanIndex {
  return { units: new Map(), byObject: new Map() };
}

function upsertUnit(index: ScanIndex, unit: FileObjectState): void {
  const key = addressKey(unit.address);
  const prev = index.units.get(key);
  index.units.set(key, unit);
  const list = index.byObject.get(unit.address.objectId) ?? [];
  const next = prev ? list.filter((u) => addressKey(u.address) !== key) : list.slice();
  next.push(unit);
  index.byObject.set(unit.address.objectId, next);
}

function removeUnit(index: ScanIndex, address: KnowledgeAddress): FileObjectState | undefined {
  const key = addressKey(address);
  const prev = index.units.get(key);
  if (!prev) return undefined;
  index.units.delete(key);
  const list = (index.byObject.get(address.objectId) ?? []).filter((u) => addressKey(u.address) !== key);
  if (list.length) index.byObject.set(address.objectId, list);
  else index.byObject.delete(address.objectId);
  return prev;
}

function objectUnits(index: ScanIndex, objectId: ObjectIdentity): FileObjectState[] {
  return index.byObject.get(objectId) ?? [];
}

function assertLayout(units: readonly FileObjectState[], incoming: KnowledgeAddress): void {
  if (units.length === 0) return;
  const hasBlob = units.some((u) => isEntityBlob(u.address));
  const hasAspect = units.some((u) => !isEntityBlob(u.address));
  if (hasBlob && hasAspect) {
    throw new IngressError("OBJECT_ID_CONFLICT", `${incoming.objectId} mixes entity blob and aspects`);
  }
  if (isEntityBlob(incoming) && hasAspect) {
    throw new IngressError("PRECONDITION_FAILED", `cannot PUT Entity blob on aspected object ${incoming.objectId}`);
  }
  if (!isEntityBlob(incoming) && hasBlob) {
    throw new IngressError("PRECONDITION_FAILED", `cannot PUT Aspect on entity blob ${incoming.objectId}`);
  }
}

function assembleValue(units: readonly FileObjectState[]): unknown {
  if (units.length === 0) return undefined;
  const blobs = units.filter((u) => isEntityBlob(u.address));
  const parts = units.filter((u) => !isEntityBlob(u.address));
  if (blobs.length && parts.length) {
    throw new IngressError("OBJECT_ID_CONFLICT", `${units[0]?.objectId} mixes entity blob and aspects`);
  }
  if (blobs.length > 1) {
    throw new IngressError("OBJECT_ID_CONFLICT", `duplicate object_id ${blobs[0]?.objectId}`);
  }
  if (blobs.length === 1) return blobs[0].value;

  const recordNames = new Set<string>();
  const memberNames = new Set<string>();
  const out: Record<string, unknown> = {};
  const members: Record<string, Record<string, unknown>> = {};

  for (const unit of parts) {
    const name = unit.address.aspectName;
    if (!name) continue;
    if (unit.address.memberKey) {
      memberNames.add(name);
      const bucket = members[name] ?? {};
      bucket[unit.address.memberKey] = unit.value;
      members[name] = bucket;
    } else {
      recordNames.add(name);
      out[name] = unit.value;
    }
  }
  for (const name of memberNames) {
    if (recordNames.has(name)) {
      throw new IngressError("OBJECT_ID_CONFLICT", `aspect ${name} is both Record and Member`);
    }
    out[name] = members[name];
  }
  return out;
}

function entityPathHint(units: readonly FileObjectState[], objectId: ObjectIdentity): string {
  if (units.length === 1) return units[0].pathHint ?? units[0].path;
  if (units.length === 0) return "";
  return `objects/${objectId}`;
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

function validateProvenance(provenance: ProvenanceEnvelope | undefined): void {
  if (provenance?.originKind !== "DERIVATION") return;
  const algorithm = provenance.algorithm;
  const hasAlgorithmIdentity = Boolean(
    algorithm?.derivationSpecRef || algorithm?.modelRef || algorithm?.codeHash,
  );
  if (!provenance.inputViewReadVersionRef || !hasAlgorithmIdentity) {
    throw new IngressError(
      "PRECONDITION_FAILED",
      "DERIVATION provenance requires a fixed input ViewReadVersion and algorithm identity",
    );
  }
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
    const commit = this.getRef(ref);
    if (!commit) throw new IngressError("VERSION_UNRESOLVED", `ref ${ref} is unresolved`);
    return commit;
  }

  getRef(ref: string): CommitIdentity | undefined {
    return gitOk(this.rootDir, "rev-parse", "--verify", ref)
      ? git(this.rootDir, "rev-parse", ref)
      : undefined;
  }

  hasCommit(commitId: CommitIdentity): boolean {
    return Boolean(commitId) && gitOk(this.rootDir, "cat-file", "-e", `${commitId}^{commit}`);
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
    validateProvenance(cs.provenance);
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
      const index = this.scan();
      const toWrite = new Map<string, string>();
      const toDelete = new Set<string>();

      for (const op of cs.operations) {
        this.applyOp(index, op, cs.provenance, toWrite, toDelete);
      }

      for (const p of toDelete) {
        if (!toWrite.has(p)) rmSync(path.join(this.rootDir, p), { force: true });
      }
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

  private applyOp(
    index: ScanIndex,
    op: Operation,
    changesetProvenance: ProvenanceEnvelope | undefined,
    toWrite: Map<string, string>,
    toDelete: Set<string>,
  ): void {
    assertWritableAddress(op.address);
    if (op.op === "PUT") {
      const siblings = objectUnits(index, op.address.objectId);
      const existing = index.units.get(addressKey(op.address));
      assertLayout(siblings, op.address);
      const pc = op.precondition;
      if (pc?.type === "IF_ABSENT" && existing) {
        throw new IngressError("PRECONDITION_FAILED", `${addressKey(op.address)} already exists`);
      }
      if ((pc?.type === "IF_OBJECT_EQUALS" || pc?.type === "IF_DIGEST_EQUALS") && pc.digest) {
        if (!existing || existing.digest !== pc.digest) {
          throw new IngressError("PRECONDITION_FAILED", `digest mismatch for ${addressKey(op.address)}`);
        }
      }
      const newPath = safeRelativePath(op.pathHint ?? existing?.path ?? defaultPath(op.address));
      if (existing && existing.path !== newPath) {
        toDelete.add(existing.path);
        toWrite.delete(existing.path);
      }
      const unit: FileObjectState = {
        objectId: op.address.objectId,
        address: op.address,
        pathHint: op.pathHint !== undefined ? newPath : existing?.pathHint,
        schemaRef: op.schemaRef ?? existing?.schemaRef,
        provenance: changesetProvenance ?? existing?.provenance,
        value: op.value,
        path: newPath,
        digest: canonicalDigest(op.value),
      };
      toWrite.set(newPath, serializeFile({
        address: op.address,
        pathHint: unit.pathHint,
        schemaRef: unit.schemaRef,
        provenance: unit.provenance,
        value: op.value,
      }));
      upsertUnit(index, unit);
      return;
    }

    // REMOVE
    if (isEntityBlob(op.address)) {
      const units = objectUnits(index, op.address.objectId);
      if (units.length === 0) {
        throw new IngressError("PRECONDITION_FAILED", `${op.address.objectId} does not exist`);
      }
      const pc = op.precondition;
      if ((pc?.type === "IF_OBJECT_EQUALS" || pc?.type === "IF_DIGEST_EQUALS") && pc.digest) {
        const assembled = assembleValue(units);
        if (canonicalDigest(assembled) !== pc.digest) {
          throw new IngressError("PRECONDITION_FAILED", `digest mismatch for ${op.address.objectId}`);
        }
      }
      for (const unit of units) {
        toDelete.add(unit.path);
        toWrite.delete(unit.path);
        removeUnit(index, unit.address);
      }
      return;
    }

    const existing = index.units.get(addressKey(op.address));
    if (!existing) {
      throw new IngressError("PRECONDITION_FAILED", `${addressKey(op.address)} does not exist`);
    }
    const pc = op.precondition;
    if ((pc?.type === "IF_OBJECT_EQUALS" || pc?.type === "IF_DIGEST_EQUALS") && pc.digest !== existing.digest) {
      throw new IngressError("PRECONDITION_FAILED", `digest mismatch for ${addressKey(op.address)}`);
    }
    toDelete.add(existing.path);
    toWrite.delete(existing.path);
    removeUnit(index, op.address);
  }

  /** Append entries to a JSONL side stream (append-only; event-id idempotent). */
  append(streamRef: string, entries: readonly AppendEntry[], expectedCursor?: string): readonly string[] {
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
    if (expectedCursor !== undefined && expectedCursor !== String(count)) {
      throw new IngressError("PRECONDITION_FAILED", `expected stream cursor ${expectedCursor} but cursor is ${count}`);
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
    const units = objectUnits(this.scanAt(commitId), objectId);
    if (units.length) {
      const assembled = assembleValue(units);
      return {
        repository: this.repositoryId,
        commit: commitId,
        objectId,
        address: { kind: "Entity", objectId },
        pathHint: entityPathHint(units, objectId),
        digest: canonicalDigest(assembled),
        schemaRef: units.length === 1 ? units[0].schemaRef : undefined,
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
    const units = objectUnits(this.scanAt(commitId), objectId);
    if (!units.length) throw new IngressError("KNOWLEDGE_REF_UNRESOLVED", `${objectId} not resolvable at ${commitId}`);
    return {
      knowledgeRef: { repository: this.repositoryId, object: objectId },
      repository: this.repositoryId,
      commit: commitId,
      address: { kind: "Entity", objectId },
      value: assembleValue(units),
      provenance: units.length === 1 ? units[0].provenance : undefined,
      units: units.some((u) => u.address.aspectName) ? units.map((u) => u.address) : undefined,
    };
  }

  resolveAddress(address: KnowledgeAddress, commitId: CommitIdentity): Resolution {
    assertWritableAddress(address);
    const unit = this.scanAt(commitId).units.get(addressKey(address));
    if (unit) {
      return {
        repository: this.repositoryId,
        commit: commitId,
        objectId: address.objectId,
        address: unit.address,
        pathHint: unit.pathHint ?? unit.path,
        digest: unit.digest,
        schemaRef: unit.schemaRef,
        status: "RESOLVED",
      };
    }
    return {
      repository: this.repositoryId,
      commit: commitId,
      objectId: address.objectId,
      address,
      pathHint: "",
      status: this.everExisted(address.objectId) ? "REMOVED" : "UNRESOLVED",
    };
  }

  readAddress(address: KnowledgeAddress, commitId: CommitIdentity): KnowledgeValue {
    assertWritableAddress(address);
    const unit = this.scanAt(commitId).units.get(addressKey(address));
    if (!unit) {
      throw new IngressError("KNOWLEDGE_REF_UNRESOLVED", `${addressKey(address)} not resolvable at ${commitId}`);
    }
    return {
      knowledgeRef: { repository: this.repositoryId, object: address.objectId },
      repository: this.repositoryId,
      commit: commitId,
      address: unit.address,
      value: unit.value,
      provenance: unit.provenance,
    };
  }

  origin(objectId: ObjectIdentity, commitId: CommitIdentity): ProvenanceTrace {
    const units = objectUnits(this.scanAt(commitId), objectId);
    if (!units.length) throw new IngressError("KNOWLEDGE_REF_UNRESOLVED", `${objectId} not resolvable at ${commitId}`);
    const chain: ProvenanceEnvelope[] = [];
    const sorted = [...units].sort((a, b) => addressKey(a.address).localeCompare(addressKey(b.address)));
    for (const unit of sorted) {
      if (unit.provenance) chain.push(unit.provenance);
    }
    return {
      value: assembleValue(units),
      repository: this.repositoryId,
      commit: commitId,
      objectId,
      chain,
    };
  }

  search(query: string, commitId: CommitIdentity): KnowledgeValue[] {
    const index = this.scanAt(commitId);
    const needle = query.toLowerCase();
    const out: KnowledgeValue[] = [];
    for (const objectId of index.byObject.keys()) {
      const value = this.read(objectId, commitId);
      if (JSON.stringify(value.value).toLowerCase().includes(needle)) out.push(value);
    }
    return out;
  }

  list(commitId: CommitIdentity): KnowledgeValue[] {
    const out: KnowledgeValue[] = [];
    for (const objectId of this.scanAt(commitId).byObject.keys()) {
      out.push(this.read(objectId, commitId));
    }
    return out;
  }

  // ---- internals ----

  private scan(): ScanIndex {
    const index = emptyIndex();
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
          this.ingestParsed(index, parsed, path.relative(this.rootDir, full));
        }
      }
    };
    walk(this.rootDir);
    return index;
  }

  /** Scan an immutable Git tree; pinned reads never consult the working tree. */
  private scanAt(commitId: CommitIdentity): ScanIndex {
    if (!this.hasCommit(commitId)) {
      throw new IngressError("VERSION_UNRESOLVED", `commit ${commitId} is unresolved`);
    }
    const index = emptyIndex();
    const files = git(this.rootDir, "ls-tree", "-r", "--name-only", commitId).split("\n").filter(Boolean);
    for (const rel of files) {
      if (!/\.(json|md|ya?ml|txt)$/i.test(rel)) continue;
      let parsed: ParsedObject | null;
      try {
        parsed = parseFile(git(this.rootDir, "show", `${commitId}:${rel}`));
      } catch {
        throw new IngressError("TEMPORARY_UNAVAILABLE", `failed to read ${rel} at ${commitId}`);
      }
      if (!parsed) continue;
      this.ingestParsed(index, parsed, rel);
    }
    return index;
  }

  private ingestParsed(index: ScanIndex, parsed: ParsedObject, relPath: string): void {
    const key = addressKey(parsed.address);
    if (index.units.has(key)) {
      throw new IngressError("OBJECT_ID_CONFLICT", `duplicate address ${key}`);
    }
    const siblings = objectUnits(index, parsed.objectId);
    const incomingBlob = isEntityBlob(parsed.address);
    const siblingBlob = siblings.some((u) => isEntityBlob(u.address));
    const siblingAspect = siblings.some((u) => !isEntityBlob(u.address));
    if ((incomingBlob && siblingAspect) || (!incomingBlob && siblingBlob)) {
      throw new IngressError("OBJECT_ID_CONFLICT", `${parsed.objectId} mixes entity blob and aspects`);
    }
    upsertUnit(index, {
      ...parsed,
      path: relPath,
      digest: canonicalDigest(parsed.value),
    });
  }

  private everExisted(objectId: ObjectIdentity): boolean {
    try {
      const log = git(this.rootDir, "log", "--all", "--format=%H");
      for (const commitId of log.split("\n").filter(Boolean)) {
        if (this.scanAt(commitId).byObject.has(objectId)) return true;
      }
    } catch {
      // ignore
    }
    return false;
  }
}
