import type { SourceEvent } from "./events.ts";
import { encodeSourceKey } from "./keys.ts";
import type { QualifiedTable } from "./sources/etl-instance.ts";

type TaskIo = { reads: QualifiedTable[]; writes: QualifiedTable[] };

function tableId(t: QualifiedTable): string {
  return t.cluster ? `${t.cluster}.${t.db}.${t.table}` : `${t.db}.${t.table}`;
}

function mergeTables(into: QualifiedTable[], extra: readonly QualifiedTable[]): QualifiedTable[] {
  const seen = new Set(into.map(tableId));
  const out = [...into];
  for (const table of extra) {
    const id = tableId(table);
    if (seen.has(id)) continue;
    seen.add(id);
    out.push(table);
  }
  return out;
}

/**
 * Collapse per-row io/definition/orchestration into one event per task aspect.
 * Instance observations and table stubs pass through.
 */
export function aggregateEvents(events: readonly SourceEvent[]): SourceEvent[] {
  const io = new Map<string, { event: SourceEvent; payload: TaskIo }>();
  const definition = new Map<string, SourceEvent>();
  const orchestration = new Map<string, SourceEvent>();
  const rest: SourceEvent[] = [];

  for (const event of events) {
    const key = encodeSourceKey(event.sourceKey);
    if (event.op === "upsert" && event.aspect === "io" && event.sourceKey.type === "etlTask") {
      const payload = event.payload as TaskIo;
      const current = io.get(key);
      if (!current) {
        io.set(key, {
          event,
          payload: {
            reads: [...payload.reads],
            writes: [...payload.writes],
          },
        });
      } else {
        current.payload = {
          reads: mergeTables(current.payload.reads, payload.reads),
          writes: mergeTables(current.payload.writes, payload.writes),
        };
      }
      continue;
    }
    if (event.op === "upsert" && event.aspect === "definition" && event.sourceKey.type === "etlTask") {
      const prev = definition.get(key);
      if (!prev || (event.observedAt ?? "") >= (prev.observedAt ?? "")) {
        definition.set(key, event);
      }
      continue;
    }
    if (event.op === "upsert" && event.aspect === "orchestration" && event.sourceKey.type === "etlTask") {
      orchestration.set(key, event);
      continue;
    }
    if (event.op === "upsert" && event.aspect === "structure" && event.sourceKey.type === "table") {
      const id = encodeSourceKey(event.sourceKey);
      if (rest.some((e) => e.aspect === "structure" && encodeSourceKey(e.sourceKey) === id)) continue;
      rest.push(event);
      continue;
    }
    rest.push(event);
  }

  const aggregated: SourceEvent[] = [...rest];
  for (const { event, payload } of io.values()) {
    aggregated.push({ ...event, eventId: `${event.source}:io:${event.sourceKey.type === "etlTask" ? event.sourceKey.taskId : event.eventId}`, payload });
  }
  aggregated.push(...definition.values());
  aggregated.push(...orchestration.values());
  return aggregated;
}
