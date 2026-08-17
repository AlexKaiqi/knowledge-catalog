/**
 * SqliteProjection — Phase 2 Embedded Access. An FTS5 index over a repository's
 * Canonical state. The projection is REBUILDABLE and NON-AUTHORITATIVE:
 *  - it only LOCATES object ids (via FTS5); values are always read back from
 *    the canonical repository (read(objectId, commit)).
 *  - it records its basis (repository + commit) and reports lag behind head.
 *  - deleting/rebuilding the index never touches canonical state (K-19).
 */

import { createRequire } from "node:module";
import type { CommitIdentity, KnowledgeValue, ObjectIdentity, RepositoryIdentity } from "../../contracts/index.ts";

// node:sqlite is a very recent Node built-in that Vite's resolver does not yet
// recognize; load it through createRequire to bypass static analysis.
interface SqliteStatement {
  run(...params: unknown[]): unknown;
  get(...params: unknown[]): unknown;
  all(...params: unknown[]): unknown[];
}
interface SqliteDatabase {
  exec(sql: string): void;
  prepare(sql: string): SqliteStatement;
}

const require = createRequire(import.meta.url);
const { DatabaseSync } = require("node:sqlite") as {
  DatabaseSync: new (location: string) => SqliteDatabase;
};

/** Minimal read surface both MemoryRepository and FileGitRepository satisfy. */
export interface ReadableRepository {
  readonly repositoryId: RepositoryIdentity;
  head(): CommitIdentity;
  list(commitId: CommitIdentity): KnowledgeValue[];
  read(objectId: ObjectIdentity, commitId: CommitIdentity): KnowledgeValue;
}

export interface IndexDescriptor {
  readonly basisRepository: RepositoryIdentity;
  readonly basisCommit: CommitIdentity;
  readonly objectCount: number;
  readonly headCommit: CommitIdentity;
  readonly lagBehindHead: boolean;
}

export class SqliteProjection {
  private db: SqliteDatabase;
  private basisRepository: RepositoryIdentity = "";
  private basisCommit: CommitIdentity = "";

  constructor() {
    this.db = new DatabaseSync(":memory:");
  }

  /** (Re)build the FTS5 index from a pinned commit. */
  build(repository: ReadableRepository, commit: CommitIdentity): void {
    this.db.exec("DROP TABLE IF EXISTS idx");
    this.db.exec("CREATE VIRTUAL TABLE idx USING fts5(object_id UNINDEXED, value_text)");
    const insert = this.db.prepare("INSERT INTO idx(object_id, value_text) VALUES (?, ?)");
    for (const value of repository.list(commit)) {
      insert.run(value.address.objectId, JSON.stringify(value.value));
    }
    this.basisRepository = repository.repositoryId;
    this.basisCommit = commit;
  }

  /** FTS5 word-level search; values are read back from Canonical (not the index). */
  search(repository: ReadableRepository, query: string): KnowledgeValue[] {
    const fts = query
      .trim()
      .split(/\s+/)
      .filter(Boolean)
      .map((w) => `"${w.replaceAll('"', '""')}"`)
      .join(" AND ");
    const rows = this.db.prepare("SELECT object_id FROM idx WHERE value_text MATCH ?").all(fts) as {
      object_id: string;
    }[];
    return rows.map((r) => repository.read(r.object_id, this.basisCommit));
  }

  describeIndex(repository: ReadableRepository): IndexDescriptor {
    const row = this.db.prepare("SELECT count(*) AS c FROM idx").get() as { c: number };
    const head = repository.head();
    return {
      basisRepository: this.basisRepository,
      basisCommit: this.basisCommit,
      objectCount: row.c,
      headCommit: head,
      lagBehindHead: head !== this.basisCommit,
    };
  }
}
