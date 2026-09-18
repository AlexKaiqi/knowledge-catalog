"use strict";
(() => {
  const $ = (id) => document.getElementById(id);
  const sessionKey = "kc.session.v1:" + location.origin;
  const reasons = {
    SEARCH_BACKEND_UNAVAILABLE: "检索后端连不上",
    HOME_NOT_INITIALIZED: "部署尚未初始化",
    HOME_CONFIGURATION_INVALID: "部署配置无效",
    CATALOG_STATE_UNAVAILABLE: "Catalog 状态读不到",
    SNAPSHOT_UNAVAILABLE: "Snapshot 权威读不到",
    SURFACE_UNKNOWN: "未知分面",
    COMMAND_LOG_UNAVAILABLE: "Writer 账本读不到",
    CONTROL_STATE_UNAVAILABLE: "控制状态读不到",
    EVIDENCE_STORE_UNWRITABLE: "证据存不下去",
    DEPLOYMENT_STATE_UNAVAILABLE: "部署状态读不到",
  };
  const searchStates = {
    READY: "当前发布版本已可检索。",
    BUILDING: "当前发布版本的检索索引构建中。",
    UPDATING: "当前发布版本的检索索引更新中。",
    FAILED: "检索索引构建失败；已发布的知识版本仍可读取。",
    RETIRED: "检索索引已停用；已发布的知识版本仍可读取。",
    NOT_READY: "当前发布版本尚无可用的检索索引。",
    UNAVAILABLE: "检索状态暂时无法确认，可继续查询发布版本。",
    NOT_CONFIGURED: "此部署尚未配置检索服务。",
    NOT_AUTHORIZED: "当前账号没有查询检索准备状态的权限。",
  };
  const objectVerbs = {
    read: {path: "/knowledge/v1/objects:read", body: (repo, object) => ({repository: repo, object})},
    resolve: {path: "/knowledge/v1/objects:resolve", body: (repo, object) => ({repository: repo, object})},
    relations: {path: "/knowledge/v1/relations:query", body: (repo, object) => ({repository: repo, endpoint: object, limit: 20})},
    provenance: {path: "/knowledge/v1/provenance:describe", body: (repo, object) => ({repository: repo, object})},
    log: {path: "/knowledge/v1/log:query", body: (repo, object) => ({repository: repo, object, limit: 20})},
  };
  let session = null, discovery = null, requestVersion = 0, loginVersion = 0, lastRoute = {page: "", id: "", face: ""};
  let catalogs = [], repos = [], datasets = [], described = [], currentRepo = "";
  let stores = null, searchReady = {}, schemaRows = [], searchFields = [], searchFieldRepo = "";
  const showError = (error) => { $("error").textContent = error.message || String(error); $("error").hidden = false; };
  const clearError = () => { $("error").hidden = true; $("error").textContent = ""; };
  async function json(path, body, headers = {}, method) {
    const verb = method || (body === undefined ? "GET" : "POST");
    const response = await fetch(path, {method: verb, headers: {"Accept":"application/json", ...(body === undefined ? {} : {"Content-Type":"application/json"}), ...headers}, body: body === undefined ? undefined : JSON.stringify(body), credentials:"same-origin", redirect:"error", cache:"no-store"});
    const value = await response.json();
    if (!response.ok) { const error = new Error(value.error?.message || "请求失败，请稍后重试。"); error.code = value.error?.code; throw error; }
    return value;
  }
  async function probe(path) {
    const response = await fetch(path, {headers:{"Accept":"application/json"}, credentials:"same-origin", redirect:"error", cache:"no-store"});
    let value = {};
    try { value = await response.json(); } catch { value = {}; }
    return {ok: response.ok, value};
  }
  async function authHeaders() {
    if (!session) return {};
    if (session.expiresAt && session.expiresAt <= Date.now()+30000) {
      const prior = session, login = loginVersion;
      if (!session.refreshToken) throw new Error("登录已过期，请重新登录。");
      const renewed = await json("/identity/v1/token", {grantType:"refresh_token", refreshToken:session.refreshToken});
      const who = await json("/identity/v1/whoami", undefined, {Authorization:"Bearer "+renewed.accessToken});
      if (prior !== session || login !== loginVersion) throw new Error("登录状态已改变，请重新操作。");
      if (who.principal !== session.principal) throw new Error("续期后的身份发生变化，请重新登录。");
      session = {...session, ...renewed, expiresAt: Date.now()+renewed.expiresIn*1000};
      sessionStorage.setItem(sessionKey, JSON.stringify(session));
    }
    return session.local ? {"X-Kc-As":session.principal} : {Authorization:"Bearer "+session.accessToken};
  }
  async function api(path, body, method) { return json(path, body, await authHeaders(), method); }
  function route() {
    const parts = (location.hash || "#/").replace(/^#\/?/, "").split("/").filter(Boolean);
    if (parts[0] === "stores" && parts[1] === "snapshot") return {page: "snapshot", id: "", face: ""};
    if (parts[0] === "stores" && parts[1] === "index") return {page: "index", id: "", face: ""};
    if (parts[0] === "repos" && parts[1]) {
      const face = (parts[2] === "search" || parts[2] === "object" || parts[2] === "schema") ? parts[2] : "schema";
      return {page: "repo", id: decodeURIComponent(parts[1]), face};
    }
    if (parts[0] === "datasets" && parts[1]) return {page: "dataset", id: decodeURIComponent(parts.slice(1).join("/")), face: ""};
    return {page: "home", id: "", face: ""};
  }
  function repoHash(id, face) { return "#/repos/"+encodeURIComponent(id)+"/"+(face || "schema"); }
  function catalogID() { return $("catalog").value; }
  function catalogPath(suffix) { return "/catalog/v1/catalogs/"+encodeURIComponent(catalogID())+suffix; }
  function setPage() {
    const {page, id} = route();
    $("page-home").hidden = page !== "home";
    $("page-snapshot").hidden = page !== "snapshot";
    $("page-index").hidden = page !== "index";
    $("page-repo").hidden = page !== "repo";
    $("page-dataset").hidden = page !== "dataset";
    $("back-home").hidden = page === "home";
    const face = route().face || "schema";
    $("face-schema").hidden = page !== "repo" || face !== "schema";
    $("face-object").hidden = page !== "repo" || face !== "object";
    $("face-search").hidden = page !== "repo" || face !== "search";
    $("page-title").textContent = page === "repo" ? (id || "仓")
      : page === "dataset" ? (id || "Dataset")
      : page === "snapshot" ? "Snapshot 权威"
      : page === "index" ? "检索投影"
      : "观察台";
    $("page-lead").textContent = page === "home"
      ? "先看存储面和检索面，再进仓。"
      : page === "snapshot" ? "⓪ Snapshot 才是版本图。对象桶和 COS 版本控制都不是权威。"
      : page === "index" ? "③ 检索是已发布 HEAD 的投影。没配 OpenSearch 时 SEARCH 失败关闭。"
      : page === "repo" && face === "schema" ? "schema list 列出实体。列表里做 schema describe。"
      : page === "repo" && face === "object" ? "填写 object，点 read / resolve / relations / provenance / log。"
      : page === "repo" ? "填关键字，或在字段行里填值。空扫不是检索。"
      : "这份 Dataset 的消费真相是 items，不是整仓放行。";
    document.title = $("page-title").textContent + " · Knowledge Catalog";
    if (page === "repo" && id) {
      $("face-link-schema").href = repoHash(id, "schema");
      $("face-link-object").href = repoHash(id, "object");
      $("face-link-search").href = repoHash(id, "search");
      const mark = (node, on) => { if (on) node.setAttribute("aria-current", "page"); else node.removeAttribute("aria-current"); };
      mark($("face-link-schema"), face === "schema");
      mark($("face-link-object"), face === "object");
      mark($("face-link-search"), face === "search");
      if (face === "search" && currentRepo) ensureSearchFields().catch(showError);
    }
  }
  function reasonText(code) { return reasons[code] || code || ""; }
  function searchText(code) { return searchStates[code] || "检索状态尚未就绪。"; }
  function pretty(value) { return JSON.stringify(value, null, 2); }
  function showJSON(node, value, wrapId) {
    node.hidden = value == null;
    node.textContent = value == null ? "" : pretty(value);
    const wrap = wrapId ? $(wrapId) : null;
    if (wrap) wrap.hidden = value == null;
    if (value != null && wrap && typeof wrap.scrollIntoView === "function") wrap.scrollIntoView({block: "nearest"});
  }
  async function loadVerdict() {
    const live = await probe("/livez");
    const ready = await probe("/readyz");
    const search = await probe("/readyz/search");
    let text = "无法联系这台 kc serve。";
    if (live.ok || live.value.status === "live") {
      if (ready.ok && ready.value.status === "ready") text = "进程 live，可以承接流量。";
      else if (search.value.status === "not_ready") {
        const why = reasonText(search.value.reasonCode);
        text = "进程 live，检索分面未就绪" + (why ? "："+why : "。") + (search.value.reasonCode ? "（"+search.value.reasonCode+"）" : "");
      } else {
        const why = reasonText(ready.value.reasonCode);
        text = "进程 live，但还不能承接流量" + (why ? "："+why : "。") + (ready.value.reasonCode ? "（"+ready.value.reasonCode+"）" : "");
      }
    }
    $("verdict").textContent = text;
    searchReady = search.value || {};
  }
  function fillCatalogs(items) {
    const current = catalogID();
    $("catalog").textContent = "";
    catalogs = Array.isArray(items) ? items : [];
    for (const item of catalogs) {
      const option = document.createElement("option");
      option.value = item.id;
      option.textContent = item.id;
      $("catalog").appendChild(option);
    }
    if (current && catalogs.some((item) => item.id === current)) $("catalog").value = current;
    else if (catalogs[0]) $("catalog").value = catalogs[0].id;
  }
  function addLine(parent, text) {
    const p = document.createElement("p");
    p.className = "muted";
    p.textContent = text;
    parent.appendChild(p);
  }
  function renderAccessWays() {
    $("access-ways").textContent = "";
    $("access-ways").hidden = true;
  }
  function renderRepoCards(rows) {
    $("repo-cards").textContent = "";
    $("repo-empty").hidden = rows.length > 0;
    $("repo-empty").textContent = rows.length ? "" : "这个 Catalog 还没有登记成员仓。仓要先 attach，不是「没有数据」。";
    for (const row of rows) {
      const card = document.createElement("article");
      card.className = "card pick";
      const title = document.createElement("h2");
      title.textContent = row.id || "";
      card.appendChild(title);
      addLine(card, row.published || "发布状态未知");
      addLine(card, row.index || "索引尚未观察");
      card.addEventListener("click", () => { location.hash = repoHash(row.id, "schema"); });
      $("repo-cards").appendChild(card);
    }
  }
  function renderDatasetCards(items) {
    const rows = Array.isArray(items) ? items : [];
    $("dataset-section").hidden = rows.length === 0;
    $("dataset-cards").textContent = "";
    $("dataset-empty").hidden = rows.length > 0;
    $("dataset-empty").textContent = rows.length ? "" : "没有已发布 Dataset。仓仍可按 Repository 做 knowledge.read；按文件清单消费需要先 define，空清单不放行整仓。";
    for (const item of rows) {
      const card = document.createElement("article");
      card.className = "card pick";
      const title = document.createElement("h2");
      title.textContent = item.id || "";
      card.appendChild(title);
      const members = item.repositories || [];
      const count = item.itemCount;
      addLine(card, item.retired ? "已退役" : ("v"+item.revision+" · file.read 清单"));
      addLine(card, (count == null ? members.length+" 个成员仓" : count+" 条文件指针") + (members.length ? " · "+members.join(", ") : ""));
      card.addEventListener("click", () => { location.hash = "#/datasets/"+encodeURIComponent(item.id); });
      $("dataset-cards").appendChild(card);
    }
  }
  function renderPlaneCards() {
    $("plane-cards").textContent = "";
    const snapshot = stores && !stores.error ? (stores.snapshot || {}) : {};
    const retrieval = stores && !stores.error ? (stores.retrieval || {}) : {};
    const authorities = Array.isArray(snapshot.authorities) ? snapshot.authorities : [];
    const snap = document.createElement("article");
    snap.className = "card pick";
    const snapTitle = document.createElement("h2");
    snapTitle.textContent = "Snapshot 权威";
    snap.appendChild(snapTitle);
    if (stores && stores.error) addLine(snap, stores.error.code === "FORBIDDEN" ? "当前身份看不到存储绑定。" : ("无法观察 Snapshot："+stores.error.message));
    else {
      addLine(snap, (snapshot.driver || "未绑定") + (snapshot.profile ? " · "+snapshot.profile : "") + (authorities.length ? " · "+authorities.length+" 个绑定" : ""));
      const origins = [...new Set(authorities.map((item) => item.origin).filter(Boolean))];
      addLine(snap, origins[0] ? ("入口 "+origins.join(" · ")) : "本机权威，没有远程管理入口。");
    }
    snap.addEventListener("click", () => { location.hash = "#/stores/snapshot"; });
    $("plane-cards").appendChild(snap);
    const index = document.createElement("article");
    index.className = "card pick";
    const indexTitle = document.createElement("h2");
    indexTitle.textContent = "检索投影";
    index.appendChild(indexTitle);
    if (stores && stores.error) addLine(index, stores.error.code === "FORBIDDEN" ? "当前身份看不到检索绑定。" : ("无法观察检索面："+stores.error.message));
    else if ((retrieval.driver || "none") === "none") addLine(index, "未配置 OpenSearch。SEARCH 失败关闭，已发布知识仍可精确读。");
    else {
      addLine(index, "OpenSearch" + (retrieval.origin ? " · "+retrieval.origin : ""));
      addLine(index, searchReady.status === "ready" ? "检索分面 ready。" : (reasonText(searchReady.reasonCode) || "检索分面未就绪。"));
    }
    index.addEventListener("click", () => { location.hash = "#/stores/index"; });
    $("plane-cards").appendChild(index);
  }
  function renderSnapshot() {
    const snapshot = stores && !stores.error ? (stores.snapshot || {}) : {};
    const authorities = Array.isArray(snapshot.authorities) ? snapshot.authorities : [];
    if (stores && stores.error) $("snapshot-summary").textContent = stores.error.code === "FORBIDDEN" ? "当前身份看不到存储绑定。改绑定仍走 ttyd 里的 kc。" : stores.error.message;
    else $("snapshot-summary").textContent = (snapshot.driver || "未绑定") + (snapshot.profile ? " · "+snapshot.profile : "") + "。下面是 Catalog / 仓对着的权威入口，不是对象桶。";
    $("authority-empty").hidden = authorities.length > 0;
    $("authority-table").hidden = authorities.length === 0;
    $("authority-list").textContent = "";
    for (const item of authorities) {
      const tr = document.createElement("tr");
      tr.innerHTML = "<td><code></code></td><td></td><td></td><td></td>";
      tr.children[0].querySelector("code").textContent = item.id || "";
      tr.children[1].textContent = item.role === "catalog" ? "Catalog" : "成员仓";
      tr.children[2].textContent = item.driver || "";
      if (item.origin) {
        const link = document.createElement("a");
        link.className = "button";
        link.target = "_blank";
        link.rel = "noopener noreferrer";
        link.href = item.origin;
        link.textContent = item.name ? (item.origin+" / "+item.name) : item.origin;
        tr.children[3].appendChild(link);
      } else tr.children[3].textContent = "本机，无远程入口";
      $("authority-list").appendChild(tr);
    }
  }
  function renderIndex() {
    const retrieval = stores && !stores.error ? (stores.retrieval || {}) : {};
    if (stores && stores.error) $("index-summary").textContent = stores.error.code === "FORBIDDEN" ? "当前身份看不到检索绑定。" : stores.error.message;
    else if ((retrieval.driver || "none") === "none") $("index-summary").textContent = "此部署 index=none。SEARCH 必须失败关闭；精确读走 Snapshot。";
    else $("index-summary").textContent = "检索 provider 是 OpenSearch。投影可删可重建，失败不回滚已发布 HEAD。";
    $("index-ready").textContent = searchReady.status === "ready"
      ? "检索分面 ready。"
      : ("检索分面未就绪" + (searchReady.reasonCode ? "："+(reasonText(searchReady.reasonCode) || searchReady.reasonCode) : "。"));
    $("index-origin").textContent = "";
    if (retrieval.origin) {
      const link = document.createElement("a");
      link.className = "button";
      link.target = "_blank";
      link.rel = "noopener noreferrer";
      link.href = retrieval.origin;
      link.textContent = "打开 OpenSearch · " + retrieval.origin;
      $("index-origin").appendChild(link);
    } else $("index-origin").textContent = (retrieval.driver || "none") === "opensearch" ? "已声明 OpenSearch，但没有可打开的集群 URL。" : "没有 OpenSearch 集群入口。";
    $("projection-list").textContent = "";
    $("projection-empty").hidden = described.length > 0;
    $("projection-table").hidden = described.length === 0;
    for (const row of described) {
      const tr = document.createElement("tr");
      tr.innerHTML = "<td><code></code></td><td><code></code></td><td></td>";
      tr.children[0].querySelector("code").textContent = row.id || "";
      tr.children[1].querySelector("code").textContent = row.head || "尚无版本";
      tr.children[2].textContent = row.index || "尚未观察";
      $("projection-list").appendChild(tr);
    }
  }
  function renderSchemaList(rows) {
    schemaRows = Array.isArray(rows) ? rows : [];
    $("schema-empty").hidden = schemaRows.length > 0;
    $("schema-empty").textContent = schemaRows.length ? "" : "这座仓在已发布 HEAD 上还没有 Schema。";
    $("schema-list").hidden = schemaRows.length === 0;
    $("schema-list").textContent = "";
    for (const row of schemaRows) {
      const li = document.createElement("li");
      const id = document.createElement("code");
      id.textContent = row.objectId || "";
      const grow = document.createElement("span");
      grow.className = "grow";
      grow.appendChild(id);
      if (row.entity) {
        const ent = document.createElement("span");
        ent.className = "muted";
        ent.textContent = "  " + row.entity + (row.description ? "  " + row.description : "");
        grow.appendChild(ent);
      }
      li.appendChild(grow);
      const btn = document.createElement("button");
      btn.type = "button";
      btn.textContent = "schema describe";
      btn.addEventListener("click", () => { runSchemaDescribe(row.objectId).catch(showError); });
      li.appendChild(btn);
      $("schema-list").appendChild(li);
    }
  }
  async function runSchemaList() {
    if (!currentRepo) throw new Error("先打开一座仓。");
    const listed = await api("/knowledge/v1/schemas:list", {repository: currentRepo, limit: 50});
    renderSchemaList(Array.isArray(listed.schemas) ? listed.schemas : []);
    $("schema-out-meta").textContent = "";
    showJSON($("schema-out"), null, "schema-out-wrap");
  }
  async function runSchemaDescribe(objectId) {
    if (!currentRepo) throw new Error("先打开一座仓。");
    if (!objectId) throw new Error("schema list 里选一行。");
    const report = await api("/knowledge/v1/schemas:describe", {repository: currentRepo, object: objectId});
    const match = (report.schemas || []).find((item) => item.objectId === objectId);
    $("schema-out-meta").textContent = "schema describe · " + objectId;
    showJSON($("schema-out"), match || report, "schema-out-wrap");
    if (currentRepo === searchFieldRepo) searchFieldRepo = "";
    if ((route().face || "schema") === "search") await ensureSearchFields();
  }
  function fieldAccess(field) {
    return (Array.isArray(field.access) ? field.access : []).map((item) => String(item));
  }
  function isFilterField(field) {
    return fieldAccess(field).includes("filter");
  }
  function isTextField(field) {
    return fieldAccess(field).includes("text");
  }
  function isStringy(field) {
    const type = String(field.type || "string").toLowerCase();
    return !type || /string|text|varchar|keyword/.test(type);
  }
  function fieldOps(field) {
    const ops = [];
    if (isTextField(field)) ops.push(["match", "关键字"]);
    if (isFilterField(field)) {
      ops.push(["equal", "精确"]);
      if (isStringy(field)) {
        ops.push(["prefix", "前缀"]);
        ops.push(["contains", "包含"]);
      }
    }
    return ops;
  }
  function renderSearchExamples() {
    const box = $("search-examples");
    box.textContent = "";
    const chips = [];
    if (searchFields.some(isTextField)) chips.push(["试 schema", () => { $("search-query").value = "schema"; }]);
    const unit = searchFields.find((field) => field.path === "unit" && isFilterField(field));
    if (unit) chips.push(["试 unit=CNY", () => {
      $("search-query").value = "";
      if (unit.input) unit.input.value = "CNY";
      if (unit.opSelect) unit.opSelect.value = "equal";
    }]);
    const name = searchFields.find((field) => field.path === "name" && isFilterField(field));
    if (name) chips.push(["试 name 前缀", () => {
      $("search-query").value = "";
      if (name.input) name.input.value = "customer.";
      if (name.opSelect) name.opSelect.value = "prefix";
    }]);
    box.hidden = chips.length === 0;
    for (const [label, fill] of chips) {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.textContent = label;
      btn.addEventListener("click", async () => {
        clearError();
        fill();
        try { await runSearch(); } catch (error) { renderSearch(null); showError(error); }
      });
      box.appendChild(btn);
    }
  }
  function renderSearchFields(rows) {
    searchFields = Array.isArray(rows) ? rows : [];
    $("search-fields-empty").hidden = searchFields.length > 0;
    $("search-fields-empty").textContent = searchFields.length ? "" : "这座仓还没有可检索字段。先 schema list；打开 search 会按列表做 describe。只有声明了 text / filter 的字段能填。";
    $("search-fields").hidden = searchFields.length === 0;
    $("search-fields").textContent = "";
    for (const field of searchFields) {
      const li = document.createElement("li");
      li.className = "field-row";
      const grow = document.createElement("span");
      grow.className = "grow";
      const path = document.createElement("code");
      path.textContent = field.path || "";
      grow.appendChild(path);
      const meta = document.createElement("span");
      meta.className = "muted";
      meta.textContent = "  " + [field.objectId, field.type, fieldAccess(field).join(" ")].filter(Boolean).join(" · ");
      grow.appendChild(meta);
      li.appendChild(grow);
      const ops = fieldOps(field);
      const op = document.createElement("select");
      op.setAttribute("aria-label", (field.path || "field") + " 算子");
      for (const [id, label] of ops) {
        const option = document.createElement("option");
        option.value = id;
        option.textContent = label;
        op.appendChild(option);
      }
      if (isFilterField(field)) op.value = "equal";
      else if (ops[0]) op.value = ops[0][0];
      field.opSelect = op;
      if (ops.length) li.appendChild(op);
      const input = document.createElement("input");
      input.type = "text";
      input.autocomplete = "off";
      input.placeholder = "填值";
      input.setAttribute("aria-label", (field.path || "field") + " 值");
      field.input = input;
      li.appendChild(input);
      $("search-fields").appendChild(li);
    }
    renderSearchExamples();
  }
  async function ensureSearchFields() {
    if (!currentRepo) { renderSearchFields([]); return; }
    if (searchFieldRepo === currentRepo && $("search-fields").children.length) return;
    if (!schemaRows.length) {
      renderSearchFields([]);
      return;
    }
    const rows = [];
    for (const schema of schemaRows) {
      const report = await api("/knowledge/v1/schemas:describe", {repository: currentRepo, object: schema.objectId});
      const match = (report.schemas || []).find((item) => item.objectId === schema.objectId) || {};
      for (const field of match.fields || []) {
        if (!field.path || (!isTextField(field) && !isFilterField(field))) continue;
        rows.push({objectId: schema.objectId, path: field.path, type: field.type || "", access: fieldAccess(field)});
      }
    }
    searchFieldRepo = currentRepo;
    renderSearchFields(rows);
  }
  function renderItems(detail) {
    const items = Array.isArray(detail?.items) ? detail.items : [];
    $("dataset-title").textContent = detail?.id || "Dataset";
    $("dataset-meta").textContent = detail ? ((detail.retired ? "已退役 · " : "") + "v"+(detail.revision ?? "—")+" · file.read") : "";
    $("dataset-count").textContent = detail
      ? (items.length + " 条文件指针" + ((detail.repositories || []).length ? " · 成员 "+(detail.repositories || []).join(", ") : "") + "。消费走 dataset-files，不走 SEARCH。")
      : "";
    $("item-empty").hidden = items.length > 0;
    $("item-table").hidden = items.length === 0;
    $("item-list").textContent = "";
    $("pin-meta").textContent = "";
    $("pin-table").hidden = true;
    $("pin-list").textContent = "";
    for (const item of items) {
      const tr = document.createElement("tr");
      tr.innerHTML = "<td><code></code></td><td><code></code></td><td><code></code></td><td></td><td><code></code></td>";
      tr.children[0].querySelector("code").textContent = item.target || "";
      tr.children[1].querySelector("code").textContent = item.repository || "";
      tr.children[2].querySelector("code").textContent = item.commit || "";
      tr.children[3].textContent = item.kind || "";
      tr.children[4].querySelector("code").textContent = item.prefix || item.file || "";
      $("item-list").appendChild(tr);
    }
  }
  function shortCommit(value) {
    return value && value.length > 16 ? value.slice(0, 12) + "…" : (value || "");
  }
  function quoteCLI(value) {
    return /[\s*$']/.test(value) ? "'" + String(value).replace(/'/g, `'\\''`) + "'" : String(value);
  }
  function searchCLI(body) {
    const parts = ["kc search", "--repo", body.repository];
    if (body.query) parts.push("--query", quoteCLI(body.query));
    for (const item of body.match || []) parts.push("--match", quoteCLI(item));
    for (const item of body.equal || []) parts.push("--eq", quoteCLI(item));
    for (const item of body.prefix || []) parts.push("--prefix", quoteCLI(item));
    for (const item of body.contains || []) parts.push("--contains", quoteCLI(item));
    return parts.join(" ");
  }
  function renderSearch(result, request) {
    $("search-cli").hidden = !request;
    $("search-cli").textContent = request ? ("这次请求： " + searchCLI(request)) : "";
    const rows = Array.isArray(result?.hits) ? result.hits : [];
    $("search-meta").textContent = result ? ("completeness=" + (result.completeness || "—") + " · " + rows.length + " hits") : "";
    $("search-hits").hidden = rows.length === 0;
    $("search-hits").textContent = "";
    for (const hit of rows) {
      const knowledge = hit.knowledge || {};
      const version = hit.version || {};
      const objectId = knowledge.address?.objectId || version.objectId || "";
      const article = document.createElement("article");
      article.className = "hit";
      const title = document.createElement("h3");
      title.textContent = objectId || "未命名对象";
      article.appendChild(title);
      const meta = document.createElement("p");
      meta.className = "muted";
      meta.textContent = [knowledge.repository || version.repository || "", shortCommit(knowledge.commit || version.declarationCommit || ""), "点开 read"].filter(Boolean).join(" · ");
      article.appendChild(meta);
      article.addEventListener("click", () => {
        $("object-id").value = objectId;
        location.hash = repoHash(currentRepo, "object");
      });
      $("search-hits").appendChild(article);
    }
    showJSON($("search-out"), result, "search-out-wrap");
  }
  async function describeRepo(id) {
    const row = {id, published: "发布状态未知", index: "索引尚未观察", search: "", head: "", owned: null};
    const listed = repos.find((item) => item.id === id);
    if (listed && listed.schemaCount != null) row.published = "已发布 · "+listed.schemaCount+" 个 Schema";
    try {
      const desc = await api("/operations/v1/projections:describe", {repository: id});
      row.head = desc.basisCommit || "";
      if (desc.lagBehindHead) row.index = "索引落后 HEAD · "+(desc.state || "未知");
      else if (desc.basisCommit) row.index = "索引跟上已观察 basis · "+(desc.state || "未知");
      else row.index = desc.state ? ("索引 "+desc.state) : "索引尚未形成服务版投影";
    } catch (error) {
      row.index = error.code === "CAPABILITY_UNSATISFIED" ? "此部署尚未配置检索服务。" : "索引无法观察："+error.message;
    }
    try {
      row.owned = await api("/catalog/v1/repositories/"+encodeURIComponent(id));
      const ready = row.owned.readiness || {};
      if (ready.publication === "PUBLISHED") {
        row.head = ready.publishedCommit || row.head;
        row.published = "已发布" + (ready.schemaCount != null ? " · "+ready.schemaCount+" 个 Schema" : "");
      } else if (ready.publication) row.published = "发布 "+ready.publication;
      if (ready.search) row.search = ready.search;
    } catch { /* catalog member is still observable without owned metadata */ }
    return row;
  }
  async function loadHome() {
    if (!catalogID()) { renderRepoCards([]); renderDatasetCards([]); renderAccessWays(); return; }
    const [listed, listedDatasets] = await Promise.all([
      api(catalogPath("/repositories")),
      api(catalogPath("/datasets")),
    ]);
    repos = Array.isArray(listed.repositories) ? listed.repositories : [];
    datasets = Array.isArray(listedDatasets.datasets) ? listedDatasets.datasets : [];
    described = [];
    for (const item of repos) described.push(await describeRepo(item.id));
    renderRepoCards(described);
    renderDatasetCards(datasets);
    renderAccessWays();
  }
  async function loadStores() {
    try { stores = await api("/operations/v1/stores"); }
    catch (error) { stores = {error}; }
    renderPlaneCards();
  }
  function renderPin(resolved) {
    const repos = resolved?.repositories || {};
    const rows = Object.keys(repos);
    $("pin-meta").textContent = resolved
      ? ((resolved.ref || "latest") + (resolved.pinId ? " · pin "+resolved.pinId : "") + " · "+(Array.isArray(resolved.items) ? resolved.items.length : rows.length)+" 条坐标")
      : "";
    $("pin-table").hidden = rows.length === 0;
    $("pin-list").textContent = "";
    for (const id of rows) {
      const tr = document.createElement("tr");
      tr.innerHTML = "<td><code></code></td><td><code></code></td>";
      tr.children[0].querySelector("code").textContent = id;
      tr.children[1].querySelector("code").textContent = repos[id] || "";
      $("pin-list").appendChild(tr);
    }
  }
  function resetRepoFaces() {
    schemaRows = [];
    renderSchemaList([]);
    $("schema-out-meta").textContent = "";
    showJSON($("schema-out"), null, "schema-out-wrap");
    $("object-id").value = "";
    $("object-meta").textContent = "";
    showJSON($("object-readout"), null, "object-out-wrap");
    $("search-query").value = "";
    $("search-cli").hidden = true;
    $("search-cli").textContent = "";
    $("search-examples").hidden = true;
    $("search-examples").textContent = "";
    $("search-hits").hidden = true;
    $("search-hits").textContent = "";
    searchFieldRepo = "";
    renderSearchFields([]);
    renderSearch(null);
  }
  function clearRepo() {
    currentRepo = "";
    $("repo-heading").textContent = "仓";
    $("repo-identity").textContent = "";
    $("repo-head").textContent = "";
    $("repo-head-line").hidden = true;
    $("repo-pub").textContent = "";
    $("repo-search-state").textContent = "";
    $("repo-search-state").className = "badge";
    resetRepoFaces();
  }
  async function loadRepo(id) {
    const switched = currentRepo !== id;
    currentRepo = id;
    $("repo-heading").textContent = id;
    const row = await describeRepo(id);
    $("repo-pub").textContent = row.owned?.readiness?.publication === "PUBLISHED" ? "已发布" : (row.owned?.provisioningState === "READY" ? "可使用" : "观察中");
    const owned = row.owned;
    $("repo-identity").textContent = owned
      ? [owned.store, owned.catalog, owned.owner].filter(Boolean).join(" · ")
      : "Catalog 成员仓 "+id;
    const head = row.head || owned?.readiness?.publishedCommit || owned?.head || "";
    $("repo-head").textContent = shortCommit(head) || "尚无版本";
    $("repo-head-line").hidden = !head;
    const state = row.search ? searchText(row.search) : row.index;
    $("repo-search-state").textContent = state;
    $("repo-search-state").className = "badge" + ((row.search === "READY" || /READY/.test(row.index || "")) ? " ready" : "");
    if (switched) resetRepoFaces();
    try {
      await runSchemaList();
    } catch (error) {
      if (error.code === "FORBIDDEN") {
        renderSchemaList([]);
        $("schema-empty").textContent = "当前身份不能 schema list。";
        $("schema-empty").hidden = false;
        renderSearchFields([]);
        return;
      }
      throw error;
    }
    if ((route().face || "schema") === "search") await ensureSearchFields();
  }
  async function loadDataset(id) {
    if (!catalogID()) { renderItems(null); return; }
    renderItems(await api(catalogPath("/datasets/"+encodeURIComponent(id))));
  }
  function parseSearchQuery() {
    if (!currentRepo) throw new Error("先打开一座仓。");
    const query = ($("search-query").value || "").trim();
    const body = {repository: currentRepo, limit: 20};
    if (query) { body.query = query; body.matchMode = "AllTerms"; }
    for (const field of searchFields) {
      const value = ((field.input && field.input.value) || "").trim();
      if (!value) continue;
      const path = field.path || "";
      if (!path) continue;
      const op = (field.opSelect && field.opSelect.value) || (isFilterField(field) ? "equal" : "match");
      const pair = path + "=" + value;
      if (op === "match") body.match = (body.match || []).concat(pair);
      else if (op === "equal") body.equal = (body.equal || []).concat(pair);
      else if (op === "prefix") body.prefix = (body.prefix || []).concat(pair);
      else if (op === "contains") body.contains = (body.contains || []).concat(pair);
    }
    if (!body.query && !(body.match && body.match.length) && !(body.equal && body.equal.length) && !(body.prefix && body.prefix.length) && !(body.contains && body.contains.length)) {
      throw new Error("填关键字，或在下面某一行填值。");
    }
    return body;
  }
  async function runSearch() {
    const request = parseSearchQuery();
    renderSearch(await api("/knowledge/v1/search", request), request);
  }
  async function runObject(verb) {
    if (!currentRepo) throw new Error("先打开一座仓。");
    const object = $("object-id").value.trim();
    if (!object) throw new Error("填写 object。");
    const spec = objectVerbs[verb];
    if (!spec) throw new Error("未知访问方式。");
    const result = await api(spec.path, spec.body(currentRepo, object));
    $("object-meta").textContent = verb + " · " + spec.path;
    showJSON($("object-readout"), result, "object-out-wrap");
  }
  async function load() {
    const version = ++requestVersion; clearError();
    try {
      const who = await api("/identity/v1/whoami");
      if (version !== requestVersion) return;
      $("identity").textContent = who.principal;
      $("logout").hidden = false;
      $("login").hidden = true;
      $("app").hidden = false;
      const listed = await api("/catalog/v1/catalogs");
      if (version !== requestVersion) return;
      fillCatalogs(listed.catalogs);
      await loadVerdict();
      if (version !== requestVersion) return;
      await loadStores();
      if (version !== requestVersion) return;
      setPage();
      const {page, id} = route();
      lastRoute = {page, id, face: route().face || ""};
      if (page === "home") await loadHome();
      else if (page === "snapshot") renderSnapshot();
      else if (page === "index") { await loadHome(); if (version !== requestVersion) return; renderIndex(); }
      else if (page === "repo") { await loadHome(); if (version !== requestVersion) return; await loadRepo(id); }
      else { await loadHome(); if (version !== requestVersion) return; await loadDataset(id); }
    } catch (error) {
      if (version !== requestVersion) return;
      $("app").hidden = true;
      throw error;
    }
  }
  async function acceptLogin(candidate, version) {
    const headers = candidate.local ? {"X-Kc-As":candidate.principal} : {Authorization:"Bearer "+candidate.accessToken};
    const who = await json("/identity/v1/whoami", undefined, headers);
    if (version !== loginVersion) return;
    const verified = {...candidate, principal: who.principal};
    sessionStorage.setItem(sessionKey, JSON.stringify(verified));
    session = verified;
    await load();
  }
  function clearSession() {
    loginVersion++; session = null; sessionStorage.removeItem(sessionKey);
    $("authorization").hidden = true; $("browser-login").disabled = false;
    $("logout").hidden = true; $("identity").textContent = "尚未登录";
    $("login").hidden = false; $("app").hidden = true;
    catalogs = []; repos = []; datasets = []; described = []; stores = null; searchReady = {};
    renderRepoCards([]); renderDatasetCards([]); renderItems(null); renderPlaneCards(); renderAccessWays(); clearRepo();
    $("verdict").textContent = "登录后查看这台 kc serve。";
    requestVersion++;
  }
  $("credentials").addEventListener("submit", async (event) => {
    event.preventDefault(); clearError();
    const version = ++loginVersion;
    try {
      const value = $("credential").value;
      await acceptLogin(discovery.localAssertion ? {local:true, principal:value} : {accessToken:value.replace(/^Bearer\s+/i,"")}, version);
      if (version === loginVersion) $("credential").value = "";
    } catch (error) { if (version === loginVersion) showError(error); }
  });
  $("logout").addEventListener("click", () => { clearSession(); clearError(); });
  $("catalog").addEventListener("change", () => {
    if (route().page !== "home") location.hash = "#/";
    load().catch(showError);
  });
  $("schema-list-run").addEventListener("click", async () => {
    clearError();
    try { await runSchemaList(); } catch (error) { showError(error); }
  });
  $("face-object").addEventListener("click", async (event) => {
    const button = event.target.closest("[data-object-verb]");
    if (!button) return;
    clearError();
    try { await runObject(button.getAttribute("data-object-verb")); }
    catch (error) { $("object-meta").textContent = ""; showJSON($("object-readout"), null, "object-out-wrap"); showError(error); }
  });
  $("search-form").addEventListener("submit", async (event) => {
    event.preventDefault(); clearError();
    try { await runSearch(); } catch (error) { renderSearch(null); showError(error); }
  });
  $("dataset-resolve").addEventListener("click", async () => {
    clearError();
    const {page, id} = route();
    if (page !== "dataset" || !id || !catalogID()) return;
    try {
      renderPin(await api(catalogPath("/datasets/"+encodeURIComponent(id)+"/resolve"), {}));
    } catch (error) {
      renderPin(null);
      if (error.code === "FORBIDDEN") $("pin-meta").textContent = "当前身份不能解 pin（需要 dataset.resolve）。清单仍可按 catalog.read 看 items。";
      else showError(error);
    }
  });
  window.addEventListener("hashchange", () => {
    const next = route();
    const sameRepoFace = next.page === "repo" && lastRoute.page === "repo" && next.id && next.id === lastRoute.id;
    lastRoute = next;
    setPage();
    if ($("app").hidden) return;
    if (sameRepoFace) return;
    load().catch(showError);
  });
  $("browser-login").addEventListener("click", async () => {
    clearError(); $("browser-login").disabled = true;
    const version = ++loginVersion;
    try {
      const random = () => { const bytes = crypto.getRandomValues(new Uint8Array(32)); return btoa(String.fromCharCode(...bytes)).replaceAll("+","-").replaceAll("/","_").replaceAll("=",""); };
      const verifier = random(); const state = random();
      const digest = new Uint8Array(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier)));
      const challenge = btoa(String.fromCharCode(...digest)).replaceAll("+","-").replaceAll("/","_").replaceAll("=","");
      const pending = await json("/identity/v1/authorize", {codeChallenge:challenge, state});
      if (version !== loginVersion) return;
      const target = new URL(pending.authorizationURL, location.origin);
      if (!/^https?:$/.test(target.protocol) || target.username || target.password) throw new Error("服务返回的登录地址无效。");
      $("authorization-link").href = target.href;
      $("authorization").hidden = false;
      const until = Date.now()+Math.min(pending.expiresIn, 300)*1000;
      while (Date.now() < until && version === loginVersion) {
        await new Promise((resolve) => setTimeout(resolve, 3000));
        if (version !== loginVersion) return;
        const poll = await json("/identity/v1/authorize:poll", {requestURI: pending.requestURI});
        if (poll.status !== "completed") continue;
        const tokens = await json("/identity/v1/token", {grantType:"authorization_code", code:poll.code, codeVerifier:verifier, redirectURI:poll.redirectURI});
        await acceptLogin({...tokens, expiresAt: Date.now()+tokens.expiresIn*1000}, version);
        if (version === loginVersion) $("authorization").hidden = true;
        return;
      }
      throw new Error("本次登录已过期，请重新发起。");
    } catch (error) { if (version === loginVersion) showError(error); }
    finally { if (version === loginVersion) $("browser-login").disabled = false; }
  });
  async function start() {
    setPage();
    renderAccessWays();
    await loadVerdict().catch(() => {});
    discovery = await json("/identity/v1/auth");
    try { session = JSON.parse(sessionStorage.getItem(sessionKey) || "null"); } catch { sessionStorage.removeItem(sessionKey); }
    if (discovery.localAssertion) { $("credentials").hidden = false; $("login-description").textContent = "这是部署明确启用的本地测试登录。请输入 KC 用户名。"; }
    else if (discovery.browserLogin) { $("browser-login").hidden = false; }
    else { $("credentials").hidden = false; $("credential-label").textContent = "访问凭证"; $("credential").type = "password"; $("credential").autocomplete = "off"; $("login-description").textContent = "此部署使用访问凭证登录。"; }
    try { await load(); }
    catch (error) {
      if (error.code === "UNAUTHENTICATED" || !session) clearSession();
      else { showError(error); $("logout").hidden = false; $("identity").textContent = session.principal || "已登录"; }
    }
  }
  start().catch(showError);
})();
