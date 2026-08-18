import { describe, expect, it } from "vitest";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { defaultRecipe } from "../../collectors/recipe.ts";
import { previewFromStarRocks } from "../../collectors/pipeline.ts";
import { openSr, srConfigFromEnv, SrAccessError } from "../../collectors/sources/sr.ts";

const sceneRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

describe("StarRocks etl-task tables", () => {
  it("reads oms_thive_test content tables into a ChangeSet preview", async () => {
    let client;
    try {
      client = await openSr(srConfigFromEnv(sceneRoot));
    } catch (err) {
      if (err instanceof SrAccessError) {
        console.warn(`SR skipped (${err.reason}): ${err.message}`);
        return;
      }
      throw err;
    }

    try {
      const sql = defaultRecipe().tables![0]!;
      const spark = defaultRecipe().tables![1]!;
      const sqlCols = await client.describe(sql);
      const sparkCols = await client.describe(spark);
      expect(sqlCols.length).toBeGreaterThan(0);
      expect(sparkCols.length).toBeGreaterThan(0);
      console.log("sql columns", sqlCols.join(","));
      console.log("spark columns", sparkCols.join(","));

      const preview = await previewFromStarRocks(
        defaultRecipe({ limit: 20 }),
        sceneRoot,
        client,
      );
      expect(preview.changeSet.operations.length).toBeGreaterThan(0);
      expect(preview.changeSet.provenance?.originKind).toBe("SOURCE");
      console.log("preview", preview.summary, "ops", preview.changeSet.operations.length);
    } finally {
      await client.close();
    }
  });
});
