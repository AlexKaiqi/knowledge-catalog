import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { webcrypto } from "node:crypto";
import vm from "node:vm";
import test from "node:test";

const source = await readFile(new URL("./repository.js", import.meta.url), "utf8");
const origin = "https://kc.example.test";
const storageKey = "kc.repository.login.v1:" + origin;
const turn = () => new Promise(setImmediate);
const response = (value, status = 200) => ({ ok: status < 400, status, json: async () => value });

async function fixture({ readiness } = {}) {
  const elements = new Map(), stored = new Map();
  const element = (id) => {
    if (!elements.has(id)) elements.set(id, {
      textContent: "", hidden: false, value: "", disabled: false, listeners: new Map(),
      addEventListener(type, callback) { this.listeners.set(type, callback); },
      removeAttribute(name) { delete this[name]; },
    });
    return elements.get(id);
  };
  const pending = new Map();
  let denyRepository = false;
  const document = { title: "仓库管理 · Knowledge Catalog", getElementById: element };
  const context = vm.createContext({
    document, location: { origin, pathname: "/repositories/kr%3A%2F%2Fowner%2Frepo" },
    sessionStorage: { getItem: (key) => stored.get(key) ?? null, setItem: (key, value) => stored.set(key, value), removeItem: (key) => stored.delete(key) },
    URL, TextEncoder, crypto: webcrypto, setTimeout,
    btoa: (value) => Buffer.from(value, "binary").toString("base64"),
    fetch: async (path, options) => {
      const principal = options.headers["X-Kc-As"];
      if (path === "/identity/v1/auth") return response({ localAssertion: true });
      if (path === "/identity/v1/whoami") {
        if (!principal) return response({ error: { code: "UNAUTHENTICATED", message: "sign in" } }, 401);
        if (principal === "slow-alice") return new Promise((resolve) => pending.set(principal, () => resolve(response({ principal }))));
        return response({ principal });
      }
      if (path.startsWith("/catalog/v1/repositories/")) {
        if (denyRepository) return response({ error: { code: "FORBIDDEN", message: "permission revoked" } }, 403);
        return response({ repositoryId: "kr://owner/repo", owner: principal, name: principal + " notes", store: "gitea", catalog: "kr://platform/catalog", provisioningState: "READY", head: "first", readiness, managementURL: origin + "/repositories/kr%3A%2F%2Fowner%2Frepo" });
      }
      if (path.startsWith("/writer/v1/repositories/")) return response({ head: "current-head" });
      throw new Error("unexpected endpoint " + path);
    },
  });
  vm.runInContext(source, context, { filename: "repository.js" });
  await turn();
  return {
    element, document, stored, pending,
    login(principal) { element("credential").value = principal; return element("credentials").listeners.get("submit")({ preventDefault() {} }); },
    click(id) { return element(id).listeners.get("click")(); },
    revokeMetadata() { denyRepository = true; },
    principal() { const value = stored.get(storageKey); return value ? JSON.parse(value).principal : null; },
  };
}

test("a slower login cannot replace the newer successful user", async () => {
  const app = await fixture();
  const slow = app.login("slow-alice");
  await turn();
  assert.ok(app.pending.has("slow-alice"));
  await app.login("bob");
  assert.equal(app.principal(), "bob");
  assert.equal(app.element("owner").textContent, "bob");
  app.pending.get("slow-alice")();
  await slow;
  assert.equal(app.principal(), "bob");
  assert.equal(app.element("identity").textContent, "bob");
  assert.equal(app.document.title, "bob notes · Knowledge Catalog");
});

test("logout cancels a pending login and clears all prior repository details", async () => {
  const app = await fixture();
  await app.login("bob");
  const slow = app.login("slow-alice");
  await turn();
  app.click("logout");
  app.pending.get("slow-alice")();
  await slow;
  assert.equal(app.principal(), null);
  assert.equal(app.element("identity").textContent, "尚未登录");
  assert.equal(app.element("details").hidden, true);
  assert.equal(app.element("owner").textContent, "");
  assert.equal(app.element("management-url").href, undefined);
  assert.equal(app.document.title, "仓库管理 · Knowledge Catalog");
});

test("revoked metadata permission clears an earlier successful card", async () => {
  const app = await fixture();
  await app.login("bob");
  assert.equal(app.element("details").hidden, false);
  app.revokeMetadata();
  await app.click("refresh");
  assert.equal(app.element("details").hidden, true);
  assert.equal(app.element("owner").textContent, "");
  assert.equal(app.element("error").textContent, "permission revoked");
});

for (const [search, expected, forbidden] of [
  ["BUILDING", /构建中/, /失败|已可检索/],
  ["UPDATING", /更新中/, /失败|已可检索/],
  ["FAILED", /失败/, /正在准备|构建中|已可检索/],
  ["RETIRED", /停用/, /正在准备|构建中|已可检索/],
  ["NOT_READY", /尚无可用/, /正在准备|构建中|已可检索/],
  ["UNAVAILABLE", /无法确认/, /正在准备|已可检索/],
  ["READY", /已可检索/, /尚无可用|失败/],
]) {
  test(`search ${search} has an honest publication-independent explanation`, async () => {
    const app = await fixture({ readiness: { publication: "PUBLISHED", publishedCommit: "fixed-publication", profile: "PRESENT", schemaCount: 1, search } });
    await app.login("bob");
    assert.equal(app.element("head").textContent, "fixed-publication");
    assert.match(app.element("projection").textContent, expected);
    assert.doesNotMatch(app.element("projection").textContent, forbidden);
  });
}
