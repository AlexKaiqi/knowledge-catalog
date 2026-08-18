/**
 * Schema drafts in .data are not project source. Formal identity is
 * schema/* objects written through Ingress COMMIT.
 */
import { readFileSync } from "node:fs";
import path from "node:path";
import type {
  CommitChangeSet,
  CommitIdentity,
  Operation,
  RepositoryIdentity,
} from "../src/contracts/index.ts";
import type { Ingress } from "../src/api/ingress.ts";
import type { FileGitRepository } from "../src/adapters/file-git/repository.ts";

export const PHYSICAL_SCHEMA_DRAFTS: readonly { objectId: string; file: string }[] = [
  { objectId: "schema/table", file: "table.yaml" },
  { objectId: "schema/column", file: "column.yaml" },
  { objectId: "schema/etlTask", file: "etl-task.yaml" },
  { objectId: "schema/columnLineage", file: "column-lineage.yaml" },
  { objectId: "schema/physicalRelations", file: "relations.yaml" },
];

export function schemaOperationsFromDrafts(draftsDir: string): Operation[] {
  return PHYSICAL_SCHEMA_DRAFTS.map(({ objectId, file }) => {
    const rel = path.join("schemas/physical", file);
    const text = readFileSync(path.join(draftsDir, file), "utf8");
    return {
      op: "PUT" as const,
      address: { kind: "Entity" as const, objectId },
      schemaRef: "schema/schema",
      pathHint: rel.replace(/\\/g, "/"),
      value: { draft: true, objectId, text },
    };
  });
}

export function schemaChangeSet(
  repositoryId: RepositoryIdentity,
  baseCommit: CommitIdentity,
  draftsDir: string,
): CommitChangeSet {
  return {
    targetRepository: repositoryId,
    targetRef: "refs/heads/main",
    baseCommit,
    expectedTargetCommit: baseCommit,
    operations: schemaOperationsFromDrafts(draftsDir),
    message: "commit physical schema drafts",
    provenance: { originKind: "DEFINITION", sourceRefs: [draftsDir], activityRef: "collectors/commit-schemas" },
  };
}

export function commitPhysicalSchemas(
  ingress: Ingress,
  repo: FileGitRepository,
  draftsDir: string,
  commandId = "commit-physical-schemas",
) {
  const base = repo.head();
  return ingress.commit(commandId, schemaChangeSet(repo.repositoryId, base, draftsDir));
}
