import type { AppendEntries, CommitChangeSet } from "../src/contracts/index.ts";
import { aggregateEvents } from "./aggregate.ts";
import type { SourceEvent } from "./events.ts";
import { defaultRecipe, type CollectorRecipe } from "./recipe.ts";
import { eventsFromRows, type EtlTableSpec } from "./sources/etl-instance.ts";
import { openSr, srConfigFromEnv, type SrQuery } from "./sources/sr.ts";
import { eventsToPreview, previewSummary } from "./to-changeset.ts";

export type CollectorPreview = {
  readonly events: readonly SourceEvent[];
  readonly changeSet: CommitChangeSet;
  readonly append: AppendEntries;
  readonly summary: {
    readonly rows: number;
    readonly tasks: number;
    readonly tables: number;
    readonly instances: number;
    readonly skippedRows: number;
  };
};

export function previewFromRows(
  tables: ReadonlyArray<{ spec: EtlTableSpec; rows: readonly Record<string, unknown>[] }>,
  recipe: CollectorRecipe = defaultRecipe(),
): CollectorPreview {
  const events: SourceEvent[] = [];
  let skippedRows = 0;
  for (const { spec, rows } of tables) {
    const part = eventsFromRows(spec, rows);
    events.push(...part.events);
    skippedRows += part.skipped;
  }
  const aggregated = aggregateEvents(events);
  const { changeSet, append } = eventsToPreview(aggregated, recipe);
  return {
    events: aggregated,
    changeSet,
    append,
    summary: { ...previewSummary(events), skippedRows },
  };
}

export async function previewFromStarRocks(
  recipe: CollectorRecipe = defaultRecipe(),
  envDir = process.cwd(),
  client?: SrQuery,
): Promise<CollectorPreview> {
  const owned = client === undefined;
  const sr = client ?? await openSr(srConfigFromEnv(envDir));
  try {
    const tables = [];
    for (const spec of recipe.tables ?? []) {
      const rows = await sr.query(spec, recipe.limit);
      tables.push({ spec, rows });
    }
    return previewFromRows(tables, recipe);
  } finally {
    if (owned) await sr.close();
  }
}
