import type { CommitIdentity, RepositoryIdentity } from "../src/contracts/identity.ts";
import type { EtlTableSpec } from "./sources/etl-instance.ts";

export const DEFAULT_ETL_TABLES: readonly EtlTableSpec[] = [
  {
    id: "lineage_sql",
    database: "oms_thive_test",
    table: "lineage_sql_content_20260717",
    kind: "sql",
    dialect: "hive",
    system: "thive",
  },
  {
    id: "lineage_spark",
    database: "oms_thive_test",
    table: "lineage_sparkjob_code_20260717",
    kind: "spark",
    dialect: "spark",
    system: "thive",
  },
];

export type CollectorRecipe = {
  readonly repositoryId: RepositoryIdentity;
  readonly targetRef?: string;
  readonly baseCommit?: CommitIdentity;
  readonly tables?: readonly EtlTableSpec[];
  /** Preview cap per table. Omit to read the full table. */
  readonly limit?: number;
};

export function defaultRecipe(overrides: Partial<CollectorRecipe> = {}): CollectorRecipe {
  return {
    repositoryId: "kr://acme/public/physical",
    tables: DEFAULT_ETL_TABLES,
    ...overrides,
  };
}
