import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import vm from "node:vm";
import test from "node:test";

const source = await readFile(new URL("./repository.js", import.meta.url), "utf8");

test("management URL opens the console repository view", () => {
  let replaced = "";
  vm.runInContext(source, vm.createContext({
    location: {
      pathname: "/repositories/kr%3A%2F%2Fowner%2Frepo",
      replace(url) { replaced = url; },
    },
  }), { filename: "repository.js" });
  assert.equal(replaced, "/console#/repos/" + encodeURIComponent("kr://owner/repo"));
});
