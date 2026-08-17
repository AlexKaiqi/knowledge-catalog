/**
 * MemoryAppendStream — append-only log (the APPEND surface's state). git does
 * not give us append-only event semantics, so this is one of the "new" pieces.
 *
 * Invariant (K-17): entries are never updated in place; correction/retraction
 * is expressed as new entries. Idempotency by (streamRef, eventId): same id +
 * same digest replays, same id + different digest conflicts.
 */

import { createHash } from "node:crypto";
import type { AppendEntry, Digest } from "../../contracts/index.ts";
import { IngressError } from "../../contracts/errors.ts";

export interface AppendRecord {
  readonly recordId: string;
  readonly eventId: string;
  readonly payload: unknown;
  readonly digest: Digest;
  readonly recordedAt: string;
  readonly schemaRef?: string;
}

function entryDigest(entry: AppendEntry): Digest {
  return createHash("sha256").update(JSON.stringify(entry.payload)).digest("hex");
}

export class MemoryAppendStream {
  readonly streamRef: string;
  readonly records: AppendRecord[] = [];
  private readonly byEventId = new Map<string, AppendRecord>();
  private seq = 0;

  constructor(streamRef: string) {
    this.streamRef = streamRef;
  }

  get cursor(): string {
    return String(this.records.length);
  }

  /** Append entries; returns ids of newly appended records. */
  append(entries: readonly AppendEntry[]): readonly string[] {
    const appended: string[] = [];
    for (const entry of entries) {
      const digest = entryDigest(entry);
      const existing = this.byEventId.get(entry.eventId);
      if (existing) {
        if (existing.digest !== digest) {
          throw new IngressError("EVENT_ID_CONFLICT", `event id ${entry.eventId} already used with different payload`);
        }
        appended.push(existing.recordId); // idempotent replay
        continue;
      }
      this.seq += 1;
      const record: AppendRecord = {
        recordId: `rec-${this.seq}`,
        eventId: entry.eventId,
        payload: entry.payload,
        digest,
        recordedAt: entry.observedAt ?? new Date().toISOString(),
        schemaRef: entry.schemaRef,
      };
      this.records.push(record);
      this.byEventId.set(entry.eventId, record);
      appended.push(record.recordId);
    }
    return appended;
  }
}
