import type { SourceEvent } from "../events.ts";
import { etlTaskKey, tableKey } from "../keys.ts";

const EMPTY = new Set(["", "null", "NULL", "\\N", "None"]);

export type QualifiedTable = {
  readonly cluster?: string;
  readonly db: string;
  readonly table: string;
};

export type EnvelopeRow = {
  readonly taskId: string;
  readonly instanceId?: string;
  readonly sourceType?: string;
  readonly inputTables: string;
  readonly outputTables: string;
  readonly sourceSql?: string;
  readonly codeContent?: string;
  readonly taskContent?: string;
  readonly taskName?: string;
  readonly projectId?: string;
  readonly updateTime?: string;
  readonly observedAt?: string;
};

export type EtlTableSpec = {
  readonly id: string;
  readonly database: string;
  readonly table: string;
  readonly kind: "sql" | "spark";
  readonly dialect: string;
  readonly system?: string;
};

export type NormalizedEnvelope = {
  readonly source: string;
  readonly kind: "sql" | "spark";
  readonly dialect: string;
  readonly system: string;
  readonly row: EnvelopeRow;
  readonly taskId: string;
  readonly instanceId?: string;
  readonly reads: readonly QualifiedTable[];
  readonly writes: readonly QualifiedTable[];
  readonly definition?: string;
  readonly taskName?: string;
  readonly projectId?: string;
  readonly observedAt?: string;
};

const COLUMN_ALIASES: Record<keyof EnvelopeRow, readonly string[]> = {
  taskId: ["task_id", "taskid", "id"],
  instanceId: ["instance_id", "instanceid"],
  sourceType: ["source_type"],
  inputTables: ["input_tables", "inputs", "in_tables", "src_tables"],
  outputTables: ["output_tables", "outputs", "out_tables", "dst_tables"],
  sourceSql: ["source_sql", "sql", "us_sql", "sql_content"],
  codeContent: ["code_content", "spark_code", "code"],
  taskContent: ["task_content"],
  taskName: ["task_name", "name"],
  projectId: ["project_id"],
  updateTime: ["update_time", "updated_at"],
  observedAt: ["observed_at", "tdbank_imp_date"],
};

export function isBlank(value: unknown): boolean {
  if (value === undefined || value === null) return true;
  return EMPTY.has(String(value).trim());
}

export function textOf(value: unknown): string | undefined {
  if (isBlank(value)) return undefined;
  return String(value);
}

export function parseQualifiedTable(raw: string): QualifiedTable | undefined {
  const name = raw.trim();
  if (!name || isBlank(name)) return undefined;
  const parts = name.split(".").filter(Boolean);
  if (parts.length >= 3) {
    const [cluster, db, ...rest] = parts;
    const table = rest.join(".");
    if (!cluster || !db || !table) return undefined;
    return { cluster, db, table };
  }
  if (parts.length === 2) {
    const [db, table] = parts;
    if (!db || !table) return undefined;
    return { db, table };
  }
  return undefined;
}

export function tableFqn(table: QualifiedTable): string {
  return table.cluster ? `${table.cluster}.${table.db}.${table.table}` : `${table.db}.${table.table}`;
}

export function parseTableList(raw: string | undefined): QualifiedTable[] {
  if (!raw || isBlank(raw)) return [];
  const seen = new Set<string>();
  const out: QualifiedTable[] = [];
  for (const part of raw.split(",")) {
    const table = parseQualifiedTable(part);
    if (!table) continue;
    const key = tableFqn(table);
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(table);
  }
  return out;
}

export function pickColumn(row: Record<string, unknown>, names: readonly string[]): unknown {
  const lower = new Map(Object.keys(row).map((k) => [k.toLowerCase(), k]));
  for (const name of names) {
    const actual = lower.get(name.toLowerCase());
    if (actual !== undefined) return row[actual];
  }
  return undefined;
}

export function rowToEnvelope(row: Record<string, unknown>): EnvelopeRow | undefined {
  const taskId = textOf(pickColumn(row, COLUMN_ALIASES.taskId));
  if (!taskId) return undefined;
  return {
    taskId,
    instanceId: textOf(pickColumn(row, COLUMN_ALIASES.instanceId)),
    sourceType: textOf(pickColumn(row, COLUMN_ALIASES.sourceType)),
    inputTables: textOf(pickColumn(row, COLUMN_ALIASES.inputTables)) ?? "",
    outputTables: textOf(pickColumn(row, COLUMN_ALIASES.outputTables)) ?? "",
    sourceSql: textOf(pickColumn(row, COLUMN_ALIASES.sourceSql)),
    codeContent: textOf(pickColumn(row, COLUMN_ALIASES.codeContent)),
    taskContent: textOf(pickColumn(row, COLUMN_ALIASES.taskContent)),
    taskName: textOf(pickColumn(row, COLUMN_ALIASES.taskName)),
    projectId: textOf(pickColumn(row, COLUMN_ALIASES.projectId)),
    updateTime: textOf(pickColumn(row, COLUMN_ALIASES.updateTime)),
    observedAt: textOf(pickColumn(row, COLUMN_ALIASES.observedAt)),
  };
}

function definitionOf(spec: EtlTableSpec, row: EnvelopeRow): string | undefined {
  if (spec.kind === "spark") return row.codeContent ?? row.taskContent;
  return row.sourceSql ?? row.taskContent;
}

export function normalizeEnvelope(spec: EtlTableSpec, row: EnvelopeRow): NormalizedEnvelope {
  return {
    source: spec.id,
    kind: spec.kind,
    dialect: spec.dialect,
    system: spec.system ?? row.sourceType ?? "thive",
    row,
    taskId: row.taskId,
    instanceId: row.instanceId,
    reads: parseTableList(row.inputTables),
    writes: parseTableList(row.outputTables),
    definition: definitionOf(spec, row),
    taskName: row.taskName,
    projectId: row.projectId,
    observedAt: row.observedAt ?? row.updateTime,
  };
}

export function eventsFromEnvelope(envelope: NormalizedEnvelope, index: number): SourceEvent[] {
  const rawRef = `${envelope.source}#${envelope.instanceId ?? envelope.taskId}`;
  const events: SourceEvent[] = [
    {
      eventId: `${envelope.source}:row:${index}`,
      source: envelope.source,
      observedAt: envelope.observedAt,
      sourceKey: envelope.instanceId
        ? { type: "etlInstance", instanceId: envelope.instanceId }
        : etlTaskKey(envelope.taskId),
      op: "observe",
      originKind: "OBSERVATION",
      payload: {
        taskId: envelope.taskId,
        instanceId: envelope.instanceId,
        kind: envelope.kind,
        system: envelope.system,
        projectId: envelope.projectId,
      },
      rawRef,
    },
    {
      eventId: `${envelope.source}:io:${envelope.taskId}:${index}`,
      source: envelope.source,
      observedAt: envelope.observedAt,
      sourceKey: etlTaskKey(envelope.taskId),
      aspect: "io",
      op: "upsert",
      originKind: "SOURCE",
      payload: {
        reads: envelope.reads,
        writes: envelope.writes,
      },
      rawRef,
    },
  ];

  const tables = [...envelope.reads, ...envelope.writes];
  const seen = new Set<string>();
  for (const table of tables) {
    const key = tableFqn(table);
    if (seen.has(key)) continue;
    seen.add(key);
    events.push({
      eventId: `${envelope.source}:table:${key}:${index}`,
      source: envelope.source,
      observedAt: envelope.observedAt,
      sourceKey: tableKey(table.cluster, table.db, table.table),
      aspect: "structure",
      op: "upsert",
      originKind: "SOURCE",
      payload: {
        identity: {
          ...(table.cluster ? { cluster: table.cluster } : {}),
          db_name: table.db,
          table_name: table.table,
        },
      },
      rawRef,
    });
  }

  return events;
}

export function eventsFromRows(
  spec: EtlTableSpec,
  rows: readonly Record<string, unknown>[],
): { events: SourceEvent[]; skipped: number } {
  const events: SourceEvent[] = [];
  let skipped = 0;
  rows.forEach((raw, index) => {
    const envelope = rowToEnvelope(raw);
    if (!envelope) {
      skipped += 1;
      return;
    }
    events.push(...eventsFromEnvelope(normalizeEnvelope(spec, envelope), index));
  });
  return { events, skipped };
}
