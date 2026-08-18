import type { ObjectIdentity } from "../src/contracts/identity.ts";
import type { SourceKey } from "./events.ts";

export function tableKey(cluster: string | undefined, db: string, table: string): SourceKey {
  return cluster ? { type: "table", cluster, db, table } : { type: "table", db, table };
}

export function etlTaskKey(taskId: string): SourceKey {
  return { type: "etlTask", taskId };
}

export function encodeSourceKey(key: SourceKey): string {
  if (key.type === "table") {
    return key.cluster ? `table:${key.cluster}.${key.db}.${key.table}` : `table:${key.db}.${key.table}`;
  }
  if (key.type === "etlTask") return `etlTask:${key.taskId}`;
  return `etlInstance:${key.instanceId}`;
}

/** Deterministic object_id from a source key. Persist via SourceKeyMap if the rule ever changes. */
export function objectIdFor(key: SourceKey): ObjectIdentity {
  if (key.type === "table") {
    return key.cluster ? `Table:${key.cluster}.${key.db}.${key.table}` : `Table:${key.db}.${key.table}`;
  }
  if (key.type === "etlTask") return `ETLTask:${key.taskId}`;
  throw new Error(`etlInstance ${key.instanceId} is APPEND-only; it has no object_id`);
}
