/**
 * FileGitRepository — the repo-native Snapshot profile. Knowledge objects are
 * plain files (frontmatter carries the stable object_id), and versioning is
 * real git. This is the Phase 1 profile that verifies "repo-native thinness":
 * a coding agent that only knows files + git + grep can read and write.
 *
 * Identity carrier decision (minimal-core-contracts A.2):
 *  - object_id lives INSIDE the file (frontmatter), not in the path.
 *  - path is a movable `pathHint`; address-map is a rebuildable projection
 *    produced by scanning files for their object_id.
 */

import { execSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
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
import { canonicalDigest } from "../memory/repository.ts";

export interface ParsedObject {
  readonly objectId: ObjectIdentity;
  readonly pathHint?: string;
  readonly schemaRef?: string;
  readonly provenance?: ProvenanceEnvelope;
  readonly value: unknown;
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

function git(cwd: string, args: string): string {
  return execSync(`git ${args}`, { cwd, encoding: "utf8" }).trim();
}

const GIT_IDENTITY = `-c user.name="knowledge-catalog" -c user.email="dev@knowledge-catalog.local"`;

export class FileGitRepository {
  readonly repositoryId: RepositoryIdentity;
  readonly rootDir: string;
  readonly defaultRef = "HEAD";

  constructor(rootDir: string, repositoryId: RepositoryIdentity) {
    this.rootDir = rootDir;
    this.repositoryId = repositoryId;
    mkdirSync(rootDir, { recursive: true });
    if (!existsSync(path.join(rootDir, ".git"))) {
      execSync("git init -q", { cwd: rootDir });
      git(rootDir, `${GIT_IDENTITY} commit --allow-empty -q -m "root"`);
    }
  }

  head(): CommitIdentity {
    try {
      return git(this.rootDir, "rev-parse HEAD");
    } catch {
      return "";
    }
  }

  /** Apply a COMMIT ChangeSet atomically (validate in memory, then write + git commit). */
  applyCommit(cs: CommitChangeSet): CommitIdentity {
    const current = this.head();
    if (current !== cs.expectedTargetCommit) {
      throw new IngressError("NON_FAST_FORWARD", `expected ${cs.expectedTargetCommit} but HEAD is ${current}`);
    }

    // Phase 1 — load current state into memory (address-map is a rebuildable projection).
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
        const newPath = op.pathHint ?? `objects/${op.address.objectId}.json`;
        if (existing && existing.path !== newPath) toDelete.add(existing.path); // move
        toWrite.set(newPath, serializeFile({
          objectId: op.address.objectId,
          pathHint: op.pathHint ?? existing?.pathHint,
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

    git(this.rootDir, "add -A");
    git(this.rootDir, `${GIT_IDENTITY} commit -q -m "${cs.message ?? "commit"}"`);
    return this.head();
  }

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
        if (entry.name === ".git") continue;
        const full = path.join(dir, entry.name);
        if (entry.isDirectory()) walk(full);
        else if (entry.isFile() && (entry.name.endsWith(".json") || entry.name.endsWith(".md"))) {
          try {
            const parsed = parseFile(readFileSync(full, "utf8"));
            if (!parsed) continue;
            result.set(parsed.objectId, {
              ...parsed,
              path: path.relative(this.rootDir, full),
              digest: canonicalDigest(parsed.value),
            });
          } catch {
            // not a knowledge file
          }
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
      const files = git(this.rootDir, `ls-tree -r --name-only ${commitId}`).split("\n").filter(Boolean);
      for (const rel of files) {
        if (!rel.endsWith(".json") && !rel.endsWith(".md")) continue;
        try {
          const content = git(this.rootDir, `show ${commitId}:${rel}`);
          const parsed = parseFile(content);
          if (!parsed) continue;
          result.set(parsed.objectId, { ...parsed, path: rel, digest: canonicalDigest(parsed.value) });
        } catch {
          // skip
        }
      }
    } catch {
      // unknown commit
    }
    return result;
  }

  private everExisted(objectId: ObjectIdentity): boolean {
    try {
      const log = git(this.rootDir, `log --all --format=%H`);
      for (const commitId of log.split("\n").filter(Boolean)) {
        if (this.scanAt(commitId).has(objectId)) return true;
      }
    } catch {
      // ignore
    }
    return false;
  }
}
