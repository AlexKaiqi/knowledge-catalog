/**
 * SourceEvent — collector grain. One source fact, not yet a ChangeSet.
 * sourceKey is the source identifier; object_id is assigned in mapping.
 */

export type SourceKey =
  | { readonly type: "table"; readonly cluster?: string; readonly db: string; readonly table: string }
  | { readonly type: "etlTask"; readonly taskId: string }
  | { readonly type: "etlInstance"; readonly instanceId: string };

export type SourceOp = "upsert" | "observe";

export type SourceEvent = {
  readonly eventId: string;
  readonly source: string;
  readonly observedAt?: string;
  readonly sourceKey: SourceKey;
  readonly aspect?: "structure" | "io" | "definition" | "orchestration";
  readonly op: SourceOp;
  readonly originKind: "SOURCE" | "OBSERVATION";
  readonly payload: unknown;
  readonly rawRef?: string;
};
