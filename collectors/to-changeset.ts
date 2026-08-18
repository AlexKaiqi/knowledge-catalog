import type {
  AppendEntries,
  AppendEntry,
  CommitChangeSet,
  CommitIdentity,
  Operation,
  RepositoryIdentity,
} from "../src/contracts/index.ts";
import type { SourceEvent } from "./events.ts";
import { encodeSourceKey, objectIdFor } from "./keys.ts";
import { SourceKeyMap } from "./source-key-map.ts";
import type { QualifiedTable } from "./sources/etl-instance.ts";

export type PreviewTarget = {
  readonly repositoryId: RepositoryIdentity;
  readonly targetRef?: string;
  readonly baseCommit?: CommitIdentity;
};

function tableObjectId(table: QualifiedTable): string {
  return objectIdFor({ type: "table", cluster: table.cluster, db: table.db, table: table.table });
}

function putAspect(
  objectId: string,
  aspectName: string,
  schemaRef: string,
  pathHint: string,
  value: unknown,
): Operation {
  return {
    op: "PUT",
    address: { kind: "Aspect", objectId, aspectName },
    schemaRef,
    pathHint,
    value,
  };
}

function tablePath(cluster: string | undefined, db: string, table: string): string {
  return cluster
    ? `physical/table/${cluster}/${db}/${table}/structure.json`
    : `physical/table/${db}/${table}/structure.json`;
}

/**
 * First slice: ETLTask.io + empty Table.structure.
 * Instance SQL is not a job definition — do not PUT definition/orchestration.
 */
export function eventsToPreview(
  events: readonly SourceEvent[],
  target: PreviewTarget,
  keys = new SourceKeyMap(),
): { changeSet: CommitChangeSet; append: AppendEntries } {
  const operations: Operation[] = [];
  const seen = new Set<string>();
  const appendEntries: AppendEntry[] = [];
  const sourceRefs = new Set<string>();

  const add = (op: Operation) => {
    if (op.op !== "PUT") return;
    const key = `${op.address.objectId}\u001f${op.address.aspectName ?? ""}`;
    if (seen.has(key)) return;
    seen.add(key);
    operations.push(op);
  };

  for (const event of events) {
    sourceRefs.add(event.source);
    if (event.op === "observe") {
      appendEntries.push({
        eventId: event.eventId,
        eventType: "etl.instance",
        payload: event.payload,
        observedAt: event.observedAt,
        schemaRef: "schema/etlInstance",
      });
      continue;
    }

    if (event.sourceKey.type === "table" && event.aspect === "structure") {
      const objectId = keys.resolve(event.sourceKey);
      add(
        putAspect(
          objectId,
          "structure",
          "schema/table",
          tablePath(event.sourceKey.cluster, event.sourceKey.db, event.sourceKey.table),
          {},
        ),
      );
      continue;
    }

    if (event.sourceKey.type !== "etlTask" || event.aspect !== "io") continue;
    const objectId = keys.resolve(event.sourceKey);
    const payload = event.payload as { reads: QualifiedTable[]; writes: QualifiedTable[] };
    add(
      putAspect(
        objectId,
        "io",
        "schema/etlTask",
        `physical/etl-task/${event.sourceKey.taskId}/io.json`,
        {
          reads: payload.reads.map(tableObjectId),
          writes: payload.writes.map(tableObjectId),
        },
      ),
    );
  }

  const baseCommit = target.baseCommit ?? "PREVIEW";
  return {
    changeSet: {
      targetRepository: target.repositoryId,
      targetRef: target.targetRef ?? "refs/heads/main",
      baseCommit,
      expectedTargetCommit: baseCommit,
      operations,
      message: "collect etl-instance preview",
      provenance: {
        originKind: "SOURCE",
        activityRef: "collectors/etl-instance",
        sourceRefs: [...sourceRefs],
      },
    },
    append: {
      targetRepository: target.repositoryId,
      streamRef: "append/etl-instance",
      entries: appendEntries,
    },
  };
}

export function previewSummary(events: readonly SourceEvent[]): {
  rows: number;
  tasks: number;
  tables: number;
  instances: number;
} {
  const tasks = new Set<string>();
  const tables = new Set<string>();
  let rows = 0;
  let instances = 0;
  for (const event of events) {
    if (event.op === "observe") {
      rows += 1;
      instances += 1;
    }
    if (event.sourceKey.type === "etlTask") tasks.add(encodeSourceKey(event.sourceKey));
    if (event.sourceKey.type === "table") tables.add(encodeSourceKey(event.sourceKey));
  }
  return { rows, tasks: tasks.size, tables: tables.size, instances };
}
