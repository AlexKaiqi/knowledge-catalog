/**
 * Cross-process command_id log. Writer uses this so retry is not process-local.
 */

import { existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import path from "node:path";
import type { WriterIdempotencyEntry } from "./writer.ts";

export interface IdempotencyStore {
  load(): readonly WriterIdempotencyEntry[];
  save(entries: readonly WriterIdempotencyEntry[]): void;
}

export class FileIdempotencyStore implements IdempotencyStore {
  constructor(private readonly file: string) {}

  load(): readonly WriterIdempotencyEntry[] {
    if (!existsSync(this.file)) return [];
    return JSON.parse(readFileSync(this.file, "utf8")) as WriterIdempotencyEntry[];
  }

  save(entries: readonly WriterIdempotencyEntry[]): void {
    mkdirSync(path.dirname(this.file), { recursive: true });
    const tmp = `${this.file}.tmp`;
    writeFileSync(tmp, `${JSON.stringify(entries, null, 2)}\n`, "utf8");
    renameSync(tmp, this.file);
  }
}
