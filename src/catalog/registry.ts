/**
 * CatalogRegistry — durable view / generation / release state.
 * Member knowledge stays in Repository; this registry is the Catalog's own store.
 */

import { existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import path from "node:path";
import type { CatalogState } from "./catalog.ts";

export interface CatalogRegistry {
  load(): CatalogState;
  save(state: CatalogState, message?: string): void;
}

export const EMPTY_CATALOG_STATE: CatalogState = {
  views: [],
  generations: [],
  releases: {},
};

export class FileCatalogRegistry implements CatalogRegistry {
  constructor(private readonly file: string) {}

  load(): CatalogState {
    if (!existsSync(this.file)) return EMPTY_CATALOG_STATE;
    const raw = JSON.parse(readFileSync(this.file, "utf8")) as Partial<CatalogState>;
    return {
      views: raw.views ?? [],
      generations: raw.generations ?? [],
      releases: raw.releases ?? {},
    };
  }

  save(state: CatalogState, _message?: string): void {
    mkdirSync(path.dirname(this.file), { recursive: true });
    const tmp = `${this.file}.tmp`;
    writeFileSync(tmp, `${JSON.stringify(state, null, 2)}\n`, "utf8");
    renameSync(tmp, this.file);
  }
}
