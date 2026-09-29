import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { webcrypto } from "node:crypto";
import vm from "node:vm";
import test from "node:test";

const source = await readFile(new URL("./console.js", import.meta.url), "utf8");
const origin = "https://kc.example.test";
const turn = () => new Promise(setImmediate);
const response = (value, status = 200) => ({ ok: status < 400, status, json: async () => value });

function node(tag = "div") {
  const element = {
    tagName: tag,
    _text: "",
    hidden: false,
    value: "",
    disabled: false,
    className: "",
    href: "",
    style: {},
    children: [],
    classList: { toggle() {} },
    listeners: new Map(),
    addEventListener(type, callback) { this.listeners.set(type, callback); },
    appendChild(child) { this.children.push(child); return child; },
    get textContent() { return this._text; },
    set textContent(value) { this._text = value; this.children = []; },
    querySelector() { return node("code"); },
    removeAttribute() {},
    setAttribute() {},
    focus() {},
  };
  Object.defineProperty(element, "innerHTML", {
    set() {
      this.children = [0, 1, 2, 3, 4].map(() => {
        const cell = node("td");
        const code = node("code");
        cell.querySelector = () => code;
        return cell;
      });
    },
    get() { return ""; },
  });
  return element;
}

async function fixture({ hash = "#/", search = "NOT_CONFIGURED" } = {}) {
  const elements = new Map();
  const stored = new Map();
  const element = (id) => {
    if (!elements.has(id)) elements.set(id, node(id));
    return elements.get(id);
  };
  const location = { origin, hash };
  const document = { title: "观察台 · Knowledge Catalog", getElementById: element, createElement: node };
  const context = vm.createContext({
    document,
    location,
    sessionStorage: { getItem: (key) => stored.get(key) ?? null, setItem: (key, value) => stored.set(key, value), removeItem: (key) => stored.delete(key) },
    window: { addEventListener() {} },
    URL, TextEncoder, crypto: webcrypto, setTimeout,
    btoa: (value) => Buffer.from(value, "binary").toString("base64"),
    fetch: async (path, options) => {
      const principal = options.headers && options.headers["X-Kc-As"];
      if (path === "/livez") return response({ status: "live" });
      if (path === "/readyz") return response({ status: "not_ready", surface: "all", reasonCode: "SEARCH_BACKEND_UNAVAILABLE" }, 503);
      if (path === "/readyz/search") return response({ status: "not_ready", surface: "search", reasonCode: "SEARCH_BACKEND_UNAVAILABLE" }, 503);
      if (path === "/identity/v1/auth") return response({ localAssertion: true });
      if (path === "/identity/v1/whoami") {
        if (!principal) return response({ error: { code: "UNAUTHENTICATED", message: "sign in" } }, 401);
        return response({ principal });
      }
      if (path === "/catalog/v1/catalogs") return response({ catalogs: [{ id: "kr://acme/catalog" }] });
      if (path === "/operations/v1/stores") {
        return response({
          snapshot: { driver: "lakefs", profile: "scale", authorities: [
            { id: "kr://acme/catalog", role: "catalog", driver: "lakefs", origin: "http://127.0.0.1:18000", name: "acme-catalog" },
            { id: "kr://acme/core", role: "repository", driver: "lakefs", origin: "http://127.0.0.1:18000", name: "acme-core" },
          ]},
          retrieval: { driver: "none" },
        });
      }
      if (path === "/operations/v1/projections:describe") {
        return response({ state: "", basisCommit: "", lagBehindHead: false });
      }
      if (path === "/knowledge/v1/schemas:list") {
        return response({ schemas: [{ objectId: "schema/metric", entity: "Metric", description: "指标" }] });
      }
      if (path === "/knowledge/v1/schemas:describe") {
        return response({
          schemas: [{
            objectId: "schema/metric", entity: "Metric", aspect: "definition",
            fields: [
              {path: "name", type: "string", access: ["text", "filter"]},
              {path: "unit", type: "string", access: ["filter"]},
            ],
          }],
        });
      }
      if (path === "/knowledge/v1/search") {
        const body = JSON.parse(options.body || "{}");
        if (body.repository !== "kr://acme/core") {
          throw new Error("unexpected search " + options.body);
        }
        if (body.query === "merchandise" || (Array.isArray(body.equal) && body.equal[0] === "unit=CNY")) {
          return response({
            completeness: "complete",
            hits: [{ knowledge: { address: { objectId: "metric/gmv" }, repository: "kr://acme/core", commit: "c1" } }],
          });
        }
        throw new Error("unexpected search " + options.body);
      }
      if (decodeURIComponent(path) === "/catalog/v1/repositories/kr://acme/core") {
        return response({
          repositoryId: "kr://acme/core", store: "lakefs", catalog: "kr://acme/catalog",
          provisioningState: "READY", head: "c1",
          readiness: { publication: "PUBLISHED", publishedCommit: "c1", schemaCount: 4, search },
        });
      }
      if (path.endsWith("/datasets/payments") && (!options.method || options.method === "GET")) {
        return response({
          id: "payments", revision: 2, repositories: ["kr://acme/core"],
          items: [{ target: "policies", repository: "kr://acme/core", commit: "c1", kind: "prefix", prefix: "policies" }],
        });
      }
      if (path.endsWith("/datasets/payments/resolve")) {
        return response({ setId: "payments", revision: 2, ref: "v2", repositories: { "kr://acme/core": "c1" }, items: [{ target: "policies" }] });
      }
      if (path.endsWith("/datasets") && (!options.method || options.method === "GET")) {
        return response({ catalogId: "kr://acme/catalog", datasets: [{ id: "payments", revision: 2, repositories: ["kr://acme/core"], itemCount: 1 }] });
      }
      if (path.endsWith("/repositories") && (!options.method || options.method === "GET")) {
        return response({ catalogId: "kr://acme/catalog", repositories: [{ id: "kr://acme/core", schemaCount: 4 }] });
      }
      throw new Error("unexpected endpoint " + (options.method || "GET") + " " + path);
    },
  });
  vm.runInContext(source, context, { filename: "console.js" });
  await turn();
  return {
    element,
    location,
    login(principal) { element("credential").value = principal; return element("credentials").listeners.get("submit")({ preventDefault() {} }); },
  };
}

test("home shows a server verdict and catalog repository cards", async () => {
  const app = await fixture();
  await app.login("alice");
  await turn();
  assert.equal(app.element("identity").textContent, "alice");
  assert.equal(app.element("app").hidden, false);
  assert.equal(app.element("catalog").value, "kr://acme/catalog");
  assert.match(app.element("verdict").textContent, /检索分面未就绪/);
  assert.equal(app.element("repo-cards").children.length, 1);
  assert.equal(app.element("repo-cards").children[0].children[0].textContent, "kr://acme/core");
  assert.match(app.element("dataset-empty").textContent === "" ? "ok" : app.element("dataset-cards").children[0].children[0].textContent, /payments|ok/);
  assert.equal(app.element("dataset-cards").children[0].children[0].textContent, "payments");
  assert.equal(app.element("plane-cards").children.length, 2);
  assert.equal(app.element("plane-cards").children[0].children[0].textContent, "Snapshot 权威");
  assert.match(app.element("plane-cards").children[0].children[1].textContent, /lakefs/);
  assert.equal(app.element("plane-cards").children[1].children[0].textContent, "检索投影");
  assert.match(app.element("plane-cards").children[1].children[1].textContent, /未配置 OpenSearch/);
});

test("snapshot page is an authority entry not a bucket console", async () => {
  const app = await fixture({ hash: "#/stores/snapshot" });
  await app.login("alice");
  await turn();
  assert.equal(app.element("page-snapshot").hidden, false);
  assert.match(app.element("snapshot-summary").textContent, /lakefs/);
  assert.equal(app.element("authority-list").children.length, 2);
  assert.match(app.element("page-lead").textContent, /Snapshot/);
});

test("index page explains projection lag not index admin", async () => {
  const app = await fixture({ hash: "#/stores/index" });
  await app.login("alice");
  await turn();
  assert.equal(app.element("page-index").hidden, false);
  assert.match(app.element("index-summary").textContent, /index=none|失败关闭/);
  assert.equal(app.element("projection-list").children.length, 1);
  assert.equal(app.element("projection-list").children[0].children[0].querySelector().textContent, "kr://acme/core");
});

test("opening a dataset shows frozen file items without mutation controls", async () => {
  const app = await fixture({ hash: "#/datasets/payments" });
  await app.login("alice");
  await turn();
  assert.equal(app.element("page-dataset").hidden, false);
  assert.equal(app.element("dataset-title").textContent, "payments");
  assert.equal(app.element("item-list").children[0].children[0].querySelector().textContent, "policies");
  assert.equal(app.element("item-list").children[0].children[3].textContent, "prefix");
  assert.match(app.element("dataset-count").textContent, /1 条文件指针/);
  assert.match(app.element("dataset-count").textContent, /dataset-files/);
  assert.doesNotMatch(app.element("dataset-count").textContent, /kset/);
  assert.match(app.element("dataset-meta").textContent, /file\.read/);
  await app.element("dataset-resolve").listeners.get("click")();
  await turn();
  assert.equal(app.element("pin-list").children[0].children[0].querySelector().textContent, "kr://acme/core");
});

test("repository view lists schemas and keeps search probe on the repo", async () => {
  const app = await fixture({ hash: "#/repos/" + encodeURIComponent("kr://acme/core") });
  await app.login("alice");
  await turn();
  assert.equal(app.element("page-repo").hidden, false);
  assert.equal(app.element("repo-heading").textContent, "kr://acme/core");
  assert.equal(app.element("repo-head").textContent, "c1");
  assert.match(app.element("repo-search-state").textContent, /尚未配置检索服务/);
  assert.equal(app.element("schema-list").children[0].children[0].children[0].textContent, "schema/metric");
  app.element("search-query").value = "merchandise";
  await app.element("search-form").listeners.get("submit")({ preventDefault() {} });
  await turn();
  assert.match(app.element("search-out").textContent, /metric\/gmv/);
});

test("search face fills query equal prefix from schema describe fields", async () => {
  const app = await fixture({ hash: "#/repos/" + encodeURIComponent("kr://acme/core") + "/search" });
  await app.login("alice");
  await turn();
  assert.equal(app.element("face-search").hidden, false);
  assert.equal(app.element("search-fields").hidden, false);
  assert.equal(app.element("search-fields").children.length, 2);
  const unit = app.element("search-fields").children[1];
  assert.equal(unit.children[0].children[0].textContent, "unit");
  assert.equal(unit.children[1].value, "equal");
  unit.children[2].value = "CNY";
  await app.element("search-form").listeners.get("submit")({ preventDefault() {} });
  await turn();
  assert.match(app.element("search-out").textContent, /metric\/gmv/);
  assert.match(app.element("search-cli").textContent, /--eq unit=CNY/);
  assert.equal(app.element("search-hits").children.length, 1);
  assert.equal(app.element("search-hits").children[0].children[0].textContent, "metric/gmv");
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
  test(`search ${search} stays publication-independent on the repository`, async () => {
    const app = await fixture({ hash: "#/repos/" + encodeURIComponent("kr://acme/core"), search });
    await app.login("bob");
    await turn();
    assert.equal(app.element("repo-head").textContent, "c1");
    assert.match(app.element("repo-search-state").textContent, expected);
    assert.doesNotMatch(app.element("repo-search-state").textContent, forbidden);
  });
}
