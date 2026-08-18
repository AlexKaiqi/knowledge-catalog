import { describe, expect, it } from "vitest";
import { previewFromRows } from "../../collectors/pipeline.ts";
import { commitPhysicalSchemas } from "../../collectors/commit-schemas.ts";
import { defaultRecipe, DEFAULT_ETL_TABLES } from "../../collectors/recipe.ts";
import { parseQualifiedTable } from "../../collectors/sources/etl-instance.ts";
import { Ingress } from "../../src/api/ingress.ts";
import { FileGitRepository } from "../../src/adapters/file-git/repository.ts";
import { Store } from "../../src/store.ts";
import { Access } from "../../src/api/access.ts";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const sqlSpec = DEFAULT_ETL_TABLES[0]!;
const sparkSpec = DEFAULT_ETL_TABLES[1]!;
const draftsDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../.data/schemas/physical");

describe("etl-instance collector → Ingress preview", () => {
  it("parses TDW three-part names", () => {
    expect(parseQualifiedTable("tl.u_isd_qzone.r_qz_elec_d")).toEqual({
      cluster: "tl",
      db: "u_isd_qzone",
      table: "r_qz_elec_d",
    });
    expect(parseQualifiedTable("db.table")).toEqual({ db: "db", table: "table" });
  });

  it("aggregates envelopes into Aspect PUT io + empty table structure", () => {
    const preview = previewFromRows(
      [
        {
          spec: sqlSpec,
          rows: [
            {
              task_id: "t1",
              instance_id: "t1_20260712",
              source_type: "hive",
              input_tables: "tl.u_isd_qzone.r_isd_online_d,tl.hlw.t_dw_qzone",
              output_tables: "tl.u_isd_qzone.r_qz_elec_d",
              source_sql: "INSERT INTO r_qz_elec_d SELECT 1",
              project_id: "p1",
              tdbank_imp_date: "2026071712",
            },
            {
              task_id: "t1",
              instance_id: "t1_20260713",
              source_type: "hive",
              input_tables: "tl.u_isd_qzone.f_qz_read_d",
              output_tables: "tl.u_isd_qzone.r_qz_elec_d",
              source_sql: "INSERT INTO r_qz_elec_d SELECT 2",
              tdbank_imp_date: "2026071812",
            },
          ],
        },
        {
          spec: sparkSpec,
          rows: [
            {
              task_id: "s1",
              instance_id: "s1_1",
              source_type: "spark",
              input_tables: "tl.app.events",
              output_tables: "tl.app.agg",
              code_content: "spark.sql('select 1')",
            },
          ],
        },
      ],
      defaultRecipe(),
    );

    expect(preview.summary).toMatchObject({ rows: 3, tasks: 2, tables: 6, instances: 3, skippedRows: 0 });
    expect(preview.changeSet.provenance?.originKind).toBe("SOURCE");
    expect(preview.append.entries).toHaveLength(3);
    expect(preview.changeSet.operations.every((op) => op.op === "PUT" && op.address.kind === "Aspect")).toBe(true);
    expect(preview.changeSet.operations.some((op) => op.op === "PUT" && op.address.aspectName === "definition")).toBe(
      false,
    );

    const io = preview.changeSet.operations.find(
      (op) => op.op === "PUT" && op.address.objectId === "ETLTask:t1" && op.address.aspectName === "io",
    );
    expect(io && io.op === "PUT" ? io.value : undefined).toEqual({
      reads: [
        "Table:tl.u_isd_qzone.r_isd_online_d",
        "Table:tl.hlw.t_dw_qzone",
        "Table:tl.u_isd_qzone.f_qz_read_d",
      ],
      writes: ["Table:tl.u_isd_qzone.r_qz_elec_d"],
    });
    expect(
      preview.changeSet.operations.some(
        (op) => op.op === "PUT" && op.address.objectId === "Table:tl.u_isd_qzone.r_qz_elec_d" && op.address.aspectName === "structure",
      ),
    ).toBe(true);
  });

  it("COMMITs schemas then Aspect units; Access reads io without definition", () => {
    const preview = previewFromRows(
      [
        {
          spec: sqlSpec,
          rows: [
            {
              task_id: "t-ok",
              instance_id: "t-ok_1",
              input_tables: "tl.db.in_t",
              output_tables: "tl.db.out_t",
              source_sql: "select 1",
            },
          ],
        },
      ],
      defaultRecipe({ repositoryId: "kr://acme/public/physical" }),
    );

    const dir = mkdtempSync(path.join(tmpdir(), "kc-collect-"));
    try {
      const repo = new FileGitRepository(dir, "kr://acme/public/physical");
      const store = new Store();
      store.addRepository(repo);
      const ingress = new Ingress(store);
      const access = new Access(store);

      const schemaReceipt = commitPhysicalSchemas(ingress, repo, draftsDir);
      expect(schemaReceipt.disposition).toBe("APPLIED");
      expect(access.read({ repository: repo.repositoryId, object: "schema/table" }, schemaReceipt.result.commitId).value).toMatchObject({
        objectId: "schema/table",
        draft: true,
      });

      const base = repo.head();
      const receipt = ingress.commit("cmd-preview-check", {
        ...preview.changeSet,
        baseCommit: base,
        expectedTargetCommit: base,
      });
      expect(receipt.disposition).toBe("APPLIED");
      const commit = receipt.result.commitId;
      expect(repo.read("ETLTask:t-ok", commit).value).toEqual({
        io: {
          reads: ["Table:tl.db.in_t"],
          writes: ["Table:tl.db.out_t"],
        },
      });
      expect(
        access.readAddress(
          repo.repositoryId,
          { kind: "Aspect", objectId: "ETLTask:t-ok", aspectName: "io" },
          commit,
        ).value,
      ).toEqual({
        reads: ["Table:tl.db.in_t"],
        writes: ["Table:tl.db.out_t"],
      });
      expect(repo.read("Table:tl.db.out_t", commit).value).toEqual({ structure: {} });
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
