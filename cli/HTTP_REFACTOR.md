# HTTP 重构方案

审查结论的落地稿，不是已生效合同。改完之前路径权威仍是 `service_routes.go` / `service_management_routes.go` / `serve_facade.go` 的 `HandleFunc` 登记。
批时在节下写 `批：`。

与 [`REFACTOR.md`](REFACTOR.md)（CLI）配套：同一套尺子打 **typed HTTP**。两面独立注册（`API-01`），对齐的是操作与 action，不是把 argv 抄进 URL。

现行分母 **67** 条（`TestEveryPublicHTTPRouteIsRegisteredWithOnlyItsDeclaredMethod`）。CLI 旧稿写 66，以本计数为准。

---

## Goal

人看见 `METHOD /{plane}/v1/{resource}…` 就能预期：在哪一类资源上、副作用类型、身份在 path 还是 body。自定义方法（`:verb`）是闭集。Client（`client/`）与 CLI remote dispatch 打同一条权威路由。

## Non-Goals

- 不恢复 `POST /v1/<verb>` 或任意 flags DTO（`SERVICE_ARCHITECTURE.md`）。
- 不把 CLI 命令表登记成 HTTP；不把 HTTP 层级抄成 CLI（CLI 已否决 `catalog workspace` 深嵌套）。
- 不新造 `/v2/`、不把 Workspace 提成 `/workspace/v1`（组合所有权仍在 Catalog Plane）。
- 不把 `kc pack` / `writer put|remove` 做成 HTTP 写面。
- 不改 semantic action、授权求值、DTO 字段名（除路径/方法本身要求的迁移）。
- 不把 `GET /readyz/{surface}` 的 `consumer|writer|search` 改成 help 主题（那是就绪平面，不是 `kc help`）。
- 不把命令/路由穷尽清单写进 `docs/*.md`。

---

## 方法论（HTTP 尺子）

CLI 的 U/N/M 在 HTTP 上的对应。能用来否决一条路由。

| ID | 准则 | 检验 |
|---|---|---|
| U1 | 符合常见 HTTP/AIP：集合 GET list、成员 GET show；非 CRUD 用自定义方法 | `/archive` 看起来像子资源还是动作？ |
| U2 | 学会一个 namespace 能猜相邻资源 | Catalog 下 list/show/archive 是否同一套标点 |
| H1 | **语法一套，例外闭集。** 标准方法 + 冒号自定义方法。禁止同一动作有的走 `/resolve`、有的走 `:resolve` | 自定义方法词表见下 |
| H2 | **读的副作用类型要从方法上看出来。** 无大 body 的读用 GET；pin/过滤在 JSON 里的查询用 POST `:query`/`:read`。禁止 `POST …:get` | `schemas:get`、`traces:get`、`log:get` |
| H3 | **Server 面名词下只放该面请求** | `/writer/v1/…/proposals` 的 action 却是 `governance.proposal.create` |
| H4 | **同一资源 list/show 成对** | `GET /catalogs` 与 `GET /catalogs/{id}` 已过；Knowledge 没有对象 LIST（设计如此） |
| H5 | **一词一义；别名不进分母** | `objects:read` 与 `addresses:read` 同一 handler |
| H6 | **协议已占用的词跟协议** | HTTP 保留 `resolve`（不是 CLI 的 `pin`）；`define` 仍是 POST 集合 |
| H7 | **CLI 与 HTTP 操作对齐、字符串不必相同** | `kc workspace pin` → `POST …/workspaces/{id}:resolve` |
| H8 | **HTTP-only / CLI-only 必须明示** | rerank、四条 retrieval 查询、pack |
| H9 | **身份参数位置一致** | Catalog id 在 Catalog 路由的 path；Knowledge 坐标在 body（pin），不要第三种 |
| H10 | **删除用 DELETE**，生命周期结束用 `:archive` / `:retire`，不用 `POST …/remove` | grants/hooks/gates |
| H11 | **无隐含副作用；无 CLI 分发器** | handler 不收 verb/flags map 当公开 DTO（现行已是 typed struct） |
| H12 | **幂等键可见** | COMMIT body 继续 `commandId` |

自定义方法闭集（目标面仅这些冒号词）：

```text
:read :describe :list :query :resolve :check
:access :archive :retire :sync :notice :validate :merge
```

否决再引入 `:get`、`:page`、`:notify`、路径后缀 `/remove`。

---

## 选定 / 否决（结构）

| 选定 | 否决 |
|---|---|
| Workspace 仍是 `/catalog/v1/catalogs/{catalog}/workspaces` | `/workspace/v1`（第二棵资源树） |
| 解配方 HTTP 仍叫 `:resolve`（协议词） | URL 改成 `:pin`（那是 CLI 用户词） |
| Knowledge 读继续 POST（body 里有 pin） | 改 GET + query 塞 ResolvedWorkspace |
| 提案公开 URL 只留 `/governance/v1/proposals` | 同时保留 `/writer/v1/…/proposals` 进分母 |
| 读对象只留 `/objects:read` | 分母里再挂 `/addresses:read` |
| 非 CRUD 一律冒号自定义方法 | Catalog 继续 `/resolve` `/archive` 这种「假子资源」 |
| 规则删除用 DELETE | `POST …/remove` |
| 观察入站 `:notice`（body 仍是 `ChangeNotice`） | 继续 `:notify`（与 CLI `notice`、类型名都不齐） |
| Schema 目录 `:list`、单条 `:describe` | `:page` / `:get` |
| 权威路由表 `httpSurface`（类似 `cliSurface`） | 继续用正则扫全部 `HandleFunc` 当合同分母（别名会把计数撑爆） |
| 留在 `/v1/`，旧 path 做 mux 别名再删 | `/v2/` |

设计张力（不收窄设计）：`SERVICE_ARCHITECTURE.md` §7.1 写 Writer 逻辑动作含 PROPOSAL。现行两条 URL 打**同一个** `verbPropose` / `governance.proposal.create`。选定「公开 HTTP 只挂 governance」不是取消 Writer 提案能力，只禁止第二张门。

---

## 现行不合格（举一反三）

| 现象 | 根因 | 同类 |
|---|---|---|
| Catalog 动作用路径后缀 `/resolve`，Knowledge 用 `:resolve` | 没有闭集语法（H1） | `/archive` `/retire` `/check` `/remove` vs `:query` `:describe` |
| `POST …:get` | 方法是 POST、自定义词却是 GET（H2） | `schemas:get` `log:get` `provenance:get` `traces:get` |
| `/writer/…/proposals` 与 `/governance/v1/proposals` | 面挂错（H3），与 CLI `writer ingest` 同构 | — |
| `addresses:read` = `objects:read` | 别名进了分母（H5） | 若 mux 再挂旧 path，67 会继续涨 |
| `schemas:page` 不像 list | 分页实现词进了 URL | CLI `browse` 同病 |
| `:notify` vs 类型 `ChangeNotice` vs CLI 将改 `notice` | 三套词（H5/N5） | — |
| `POST …/remove` | 副作用类型藏在 path 名词里 | grants / hooks / gates 三份拷贝 |
| 临时 `POST …/workspaces/resolve` 被标成 HTTP-only 证据 | 证据分区按「remoteDispatch 表有没有单独一行」，不是按能力 | CLI `--source` 实际会打这条 |

keep 且合格（不要顺手改）：

- `GET /catalogs` + `GET /catalogs/{id}`（list/show 已对）
- Writer 只有 COMMIT / HEAD / receipt（put/remove 是 CLI 糖）
- pack 无 HTTP
- `/workspace-files/v1` 三个 `:list`/`:read`（已是冒号）
- 检索四条 `:query`、rerank 两条（HTTP-only，H8 保留）
- 基础设施 `/health` `/livez` `/readyz` `/metrics`

---

## 目标路由全表

`动作`：keep / rename / colon（路径后缀→冒号） / flatten（删重复门） / method。

### 0. 基础设施（无业务面）

| 现行 | 目标 | 动作 |
|---|---|---|
| `GET /health` `/livez` `/readyz` `/readyz/{surface}` `/metrics` | 同左 | keep。`surface∈{consumer,writer,search}` 不改 |

### 1. Identity

| 现行 | 目标 | 动作 | CLI |
|---|---|---|---|
| `GET /identity/v1/auth` | 同左 | keep | `login` 发现（无独立 whoami 以外的命令） |
| `GET /identity/v1/whoami` | 同左 | keep | `whoami`（CLI 扁平，HTTP 留在 identity 资源下） |

### 2. Catalog（Workspace 不升 namespace）

| 现行 | 目标 | 动作 | CLI |
|---|---|---|---|
| `GET /catalog/v1/catalogs` | 同左 | keep | `catalog list` |
| `GET /catalog/v1/catalogs/{catalog}` | 同左 | keep | `catalog show <id>` |
| `GET …/audit` | 同左 | keep（登记表 git，简单 query 用 GET） | `catalog audit` |
| `POST …/archive` | `POST …/catalogs/{catalog}:archive` | colon | `catalog archive` |
| `GET …/repositories` | 同左 | keep | `catalog repo list` |
| `POST …/repositories` | 同左 | keep | `catalog repo register` |
| `POST …/repositories/{repository}/archive` | `POST …/repositories/{repository}:archive` | colon | `catalog repo archive` |
| `GET …/workspaces` | 同左 | keep | `workspace list` |
| `POST …/workspaces` | 同左 | keep | `workspace define` |
| `GET …/workspaces/{workspace}` | 同左 | keep | `workspace show` |
| `POST …/workspaces/{workspace}/retire` | `POST …/workspaces/{workspace}:retire` | colon | `workspace retire` |
| `POST …/workspaces/{workspace}/resolve` | `POST …/workspaces/{workspace}:resolve` | colon | `workspace pin` |
| `POST …/workspaces/{workspace}/check` | `POST …/workspaces/{workspace}:check` | colon | `workspace check` |
| `POST …/workspaces/resolve` | `POST …/workspaces:resolve` | colon（集合自定义方法） | `workspace pin --source` |

### 3. Writer

| 现行 | 目标 | 动作 | CLI |
|---|---|---|---|
| `POST /writer/v1/repositories/{repository}/commits` | 同左 | keep | `writer commit`（及 put/remove 糖） |
| `GET …/head` | 同左 | keep | `writer head` |
| `GET /writer/v1/receipts/{command}` | 同左 | keep | `writer receipt` |
| `POST …/proposals` | **删出分母**（别名过渡期可打到 governance） | flatten | 无对应 CLI；CLI 走 governance |

### 4. Governance

| 现行 | 目标 | 动作 | CLI |
|---|---|---|---|
| `POST /governance/v1/proposals` | 同左 | keep | `governance proposal create` |
| `POST /governance/v1/previews` | 同左 | keep | `governance preview create` |
| `POST /governance/v1/previews:validate` | 同左 | keep | `governance preview validate` |
| `POST /governance/v1/validations` | 同左 | keep | `governance validation record` |
| `POST /governance/v1/proposals:merge` | 同左 | keep | `governance proposal merge` |

### 5. Admin

| 现行 | 目标 | 动作 | CLI |
|---|---|---|---|
| `GET /admin/v1/grants` | 同左 | keep | `admin grant list` |
| `POST /admin/v1/grants` | 同左 | keep | `admin grant add` |
| `POST /admin/v1/grants/{grant}/remove` | `DELETE /admin/v1/grants/{grant}` | method | `admin grant remove` |

### 6. Knowledge

| 现行 | 目标 | 动作 | CLI |
|---|---|---|---|
| `POST /knowledge/v1/objects:read` | 同左 | keep | `knowledge read` |
| `POST /knowledge/v1/addresses:read` | **删出分母** | flatten | 无 |
| `POST /knowledge/v1/objects:resolve` | 同左 | keep | `knowledge resolve` |
| `POST /knowledge/v1/search` | 同左 | keep | `knowledge search` |
| `POST /knowledge/v1/search:rerank` | 同左 | keep | **HTTP-only** |
| `POST /knowledge/v1/rerank` | 同左 | keep | **HTTP-only** |
| `POST /knowledge/v1/relations:query` | 同左 | keep | `knowledge relations` |
| `POST /knowledge/v1/provenance:get` | `POST …/provenance:describe` | rename | `knowledge provenance` |
| `POST /knowledge/v1/log:get` | `POST …/log:query` | rename | `knowledge log` |
| `POST /knowledge/v1/schemas:get` | `POST …/schemas:describe` | rename | `knowledge schema describe` |
| `POST /knowledge/v1/schemas:page` | `POST …/schemas:list` | rename | `knowledge schema list` |
| `POST /knowledge/v1/bindings:resolve` | 同左 | keep（协议词；CLI 叫 show） | `knowledge binding show` |
| `POST /knowledge/v1/resources:access` | 同左 | keep | `knowledge access` |

双靶（workspace+pin XOR repo）仍在 JSON body。接受（与 CLI M4 同一 Non-Goal）。

### 7. Workspace File Gateway

| 现行 | 目标 | CLI |
|---|---|---|
| `POST /workspace-files/v1/mounts:list` | keep | `kcfs`（无一对一产品命令名） |
| `POST /workspace-files/v1/tree:list` | keep | 同上 |
| `POST /workspace-files/v1/file:read` | keep | 同上 |

### 8. Operations

| 现行 | 目标 | 动作 | CLI |
|---|---|---|---|
| `POST /operations/v1/projections:describe` | 同左 | keep | `operations projection describe` |
| `POST …/projections:sync` | 同左 | keep | `operations projection sync` |
| `POST …/projections:notify` | `POST …/projections:notice` | rename | `operations projection notice` |
| `POST …/access-specs:describe` | 同左 | keep | `operations access-spec describe` |
| `GET/POST /operations/v1/hooks` | 同左 | keep | `operations hook list\|add` |
| `POST …/hooks/{hook}/remove` | `DELETE …/hooks/{hook}` | method | `operations hook remove` |
| `GET/POST /gates` | 同左 | keep | `operations gate list\|add` |
| `POST …/gates/{gate}/remove` | `DELETE …/gates/{gate}` | method | `operations gate remove` |
| `POST …/access-log:query` | 同左 | keep | `operations audit access` |
| `POST …/traces:get` | `POST …/traces:query` | rename | `operations audit trace` |
| `POST …/hitmap:query` | 同左 | keep | `operations audit hitmap` |
| `POST …/feedback` | 同左 | keep | `operations feedback record` |
| 四条 retrieval/refine/rerank `:query` | 同左 | keep | **HTTP-only** |

---

## 目标分母

权威 **65** 条 = 现行 67 − `addresses:read` − `writer/…/proposals`。

别名不进 `httpSurface`、不进 `TestEveryPublicHTTPRoute*` 计数。过渡期 mux 可挂旧 path，测试只认权威表。

证据分区目标：

- remote CLI 覆盖：含 `workspaces:resolve`（`--source`），不再把它算 HTTP-only
- HTTP-only：基础设施 5 + auth 发现可被 login 间接覆盖、但 whoami 仍可双挂 → 保持现有 HTTP-only 里的 health/auth/rerank/四查询/kcfs 三件；**去掉** addresses 与 writer proposals 的「必须有独立成功用例」义务（删除后无路由）

---

## CLI ↔ HTTP 对齐（操作，不是字符串）

| CLI 目标 argv | HTTP 目标 | action |
|---|---|---|
| `whoami` | `GET /identity/v1/whoami` | `identity.read` |
| `catalog list` / `show` / `audit` / `archive` | 上表 Catalog | 同左 |
| `catalog repo *` | repositories 三件 | 同左 |
| `workspace list\|show\|define\|retire` | workspaces REST + `:retire` | 同左 |
| `workspace pin` | `POST …/workspaces/{id}:resolve` | `workspace.resolve` |
| `workspace pin --source` | `POST …/workspaces:resolve` | `workspace.resolve` |
| `workspace check` | `POST …/workspaces/{id}:check` | `workspace.resolve` |
| `pack` | **无** | `writer.preview` 仅本机 |
| `writer commit\|head\|receipt` | commits / head / receipts | 同左 |
| `knowledge schema list\|describe` | `schemas:list` / `schemas:describe` | `knowledge.schema.read` |
| `knowledge binding show` | `bindings:resolve` | `knowledge.binding.resolve` |
| `knowledge access` | `resources:access` | `resource.access` |
| `operations projection notice` | `projections:notice` | `projection.manage` |
| `admin grant remove` | `DELETE /admin/v1/grants/{id}` | `admin.grants.manage` |

故意不对齐的词：CLI `pin` / `show`(binding) vs HTTP `resolve`——H6/H7。help 与 client 注释各写一句。

---

## 分阶段

HA. **权威表、不计别名。** 抽出 `httpSurface`（method+pattern → handler/action）。测试改读该表，不再 `len(HandleFunc)==67`。路由字符串先不动。把 `workspaces:resolve`（现行 `/workspaces/resolve`）的证据从 HTTP-only 挪到 remote CLI。

HB. **权威 path 换成目标；旧 path 仅 mux 别名。** `client/` 改打新 path。`remote_dispatch_internal_test` 的 target 换新。`len(httpSurface)==65`。

HC. 无宿主分层（attach 不是 HTTP）。空阶段。可与 CLI C 并行。

HD. **删别名。** 旧 URL 404/405。文档示例、dsh-plugin、数仓脚本里的 curl 切新 path。

---

## 接口

| 改 | 不改 |
|---|---|
| `cli/service_*.go` 登记字符串、`client/*.go` path | JSON 字段名、`ChangeNotice`、pin 不落盘 |
| 测试分母：`http_surface_coverage_test.go`、`http_contract_inventory_internal_test.go`、`remote_dispatch_internal_test.go` | semantic action；PERMISSIONS |
| mux 别名层（过渡） | `/v1/` 前缀；namespace 集合 |

---

## 系统性交付

### 0. 范围冻结

| 动 | 不动 |
|---|---|
| 公开 method/path、权威表、别名 | 协议动词、action、授权 |
| typed `client/` | `docs/graph/` 新节点；设计篇复制路由表 |
| 文档里作为 **调用示例** 的 URL | Agent 人格名；`readyz` surface 词 |
| Writer 提案的第二张门 | Writer 逻辑 PROPOSAL 能力 |

落地默认（与 CLI 篇同一批可批掉）：

| 缺口 | 默认 |
|---|---|
| URL 叫 resolve、CLI 叫 pin | HTTP 保留 `:resolve` |
| DELETE vs POST `:remove` | DELETE |
| provenance `:describe` vs 保留 `:get` | `:describe`（禁 `:get`） |
| 提案只留 governance | 是；writer 路径过渡期别名 |

### 1. 影响面

```text
登记            cli/service_routes.go
                cli/service_management_routes.go
                cli/serve_facade.go（仅基础设施）
权威表          新建 cli/http_surface.go（或等价；测试只认它）
Client          client/{knowledge,management,operations,workspace_files,auth}.go
CLI 远程        cli/remote_*.go（只换 client 调用，不换 argv）
合同测试        http_surface_coverage_test.go（67→65）
                http_contract_inventory_internal_test.go（48+19 分区重算）
                remote_dispatch_internal_test.go
                service_surface_test / workspace_file_service_test
                凡写死旧 URL 的 cli/*_test.go
文档示例        README.md、MVP_ACCEPTANCE、WALKTHROUGH、TEST_CATALOG、
                DEPLOY_AUTH、SERVICE_ARCHITECTURE（禁止新复制全表，只改已有示例句）
                catalog/README.md、knowledge/reader/README.md、client 包注释
插件/脚本       dsh-plugin；.data/data-warehouse docker curl；scripts/*.sh
```

`make check-docs` 在改了 `docs/*.md` 示例句之后跑。

### 2. 用例对照

语义与 CLI 篇 §2 同一组旅程；HTTP 额外：

| 用例 | 现行 | 目标 | 验收 |
|---|---|---|---|
| 发现 Catalog | `GET /catalogs` → `GET /catalogs/{id}` | 同左 | 库存无宿主 path |
| 临时配方 | `POST …/workspaces/resolve` | `POST …/workspaces:resolve` | 不写登记表；返回 pinId |
| 命名解配方 | `POST …/workspaces/{id}/resolve` | `POST …/workspaces/{id}:resolve` | 同左 |
| 读对象 | `objects:read` 或 `addresses:read` | 仅 `objects:read` | 旧 path HD 后非 2xx |
| 提案 | writer 或 governance | 仅 governance | writer 路径 HD 后非 2xx |
| Schema 目录 | `schemas:page` | `schemas:list` | JSON 形状不变 |
| 观察 | `projections:notify` | `projections:notice` | body 仍是 ChangeNotice |
| 撤权 | `POST grants/{id}/remove` | `DELETE grants/{id}` | 规则消失 |

P1–P6 / C1–C6 继续成立。C2 的机器条件是 resolve 响应（HTTP 名），CLI 展示名是 pin。

### 3. 阶段验收

先红再绿。禁止 skip。别名不进分母。

#### HA — 权威表（URL 不变）

- [ ] 存在权威 `httpSurface`（或等价）；`TestEveryPublicHTTPRoute*` 读它，**67** 条，与 mux 登记的**非别名** HandleFunc 一致
- [ ] `POST …/workspaces/resolve` 的证据 owner 是 remote CLI（`--source`），不是 HTTP-only 列表
- [ ] `make test` 绿；产品 URL 零变化

批：跳过 HA 冻结旧 URL。落地直接 HB/HD，分母 65。

#### HB — 新 path 为权威

- [x] `len(httpSurface)==65`；无 `addresses:read`、无 `/writer/v1/…/proposals` 权威键
- [ ] 新 path 走通；旧 path 在 HB **仍 2xx**（别名）
- [x] `client/` 只编新 path
- [x] `remote_dispatch_internal_test` target 全是新 path；CLI 新/旧 argv（若 CLI 已 B）打同一 HTTP
- [x] `schemas:list` 与原 `:page` 同 JSON（schemas/coverage/exhausted）
- [x] `:resolve` 响应含 `pinId` 与 `repositories`；随后 `GET …/workspaces/{id}` revision 不变
- [x] `DELETE /admin/v1/grants/{id}` 与原 POST remove 同语义
- [x] `projections:notice` 仍拒绝带正文的 ChangeNotice（既有合同）
- [x] 未声明方法仍 405

批：跳过 HTTP 别名。colon 旧名已 404；CLI 无旧 argv。

#### HD — 删别名 + 文档

- [ ] 旧 URL（`/workspaces/{id}/resolve`、`schemas:page`、`schemas:get`、`projections:notify`、`…/remove`、`addresses:read`、`/writer/…/proposals`、`*:get`）**不是** 2xx
- [x] 仓库产品路径 `rg` 旧 URL 为零（允许本文件对照表）
- [x] `docs/MVP_ACCEPTANCE.md` / Walkthrough / TEST_CATALOG / DEPLOY_AUTH 里作为调用示例的 URL 已换；设计篇**没有**新的全表
- [x] `make check-docs` 绿；`make test` 无 skip
- [x] HTTP-only 证据条数 + remote CLI 证据条数 = 65，且无重叠、无漏

批：`TestRetiredHTTPRoutesAreNotFound` 覆盖 colon 旧名、`/remove`、`addresses:read`、writer proposals。条目级 `/workspaces/{id}/resolve|retire|check` 因 Go `net/http` ServeMux 不能登记 `{id}:verb` 而保留路径后缀，不是别名层。

### 4. 全局完成定义

1. **合同**：权威 65；help/CLI 树与 HTTP 树经对齐表可互推；冒号词 ∈ 闭集。
2. **一面一门**：提案只 governance；读对象只 `objects:read`。
3. **协议未收窄**：action 名、pin 不落盘、pack 无 HTTP、SEARCH 不教 sync、无 `/v1/<verb>`。
4. **Oracle**：`make test` 路由分母与证据分区绿。
5. **文档**：示例 URL 与权威表一致；`SERVICE_ARCHITECTURE.md` 仍不复制全表。

### 5. 提交切分

1. `http: canonical surface table`（HA）
2. `http: colon custom methods; drop duplicate gates`（HB）
3. `http: remove path aliases; retarget client and docs`（HD）

与 CLI 的合入顺序（A → HA → B+HB → C → D+HD）写在 [`REFACTOR.md` 联合交付](REFACTOR.md)。不要单独把 HTTP HD 合进主线而 CLI 还在旧 argv。


---

## 目标面符合性

| 准则 | 目标面 |
|---|---|
| U1/H1 一套语法 | **钉** 冒号闭集；Catalog 不再假子资源 |
| U2 可类推 | **过** archive/retire/resolve/check 同形 |
| H2 无 `:get` | **过** |
| H3 面 | **过** writer 无提案门 |
| H4 list/show | **过** Catalog；Knowledge 按设计无对象 LIST |
| H5 别名 | **过** 65 = 67−2 |
| H6 协议词 | **过** HTTP `resolve`；CLI `pin` |
| H7 对齐 | **过** 见对齐表 |
| H8 HTTP-only | **过** rerank + 四查询 + kcfs 明示 |
| H10 DELETE | **过** |
| H12 commandId | **keep** COMMIT body |

`catalog show` 仍会读仓 HEAD 拼源说明（与 CLI 相同，本方案不改 handler 语义）。
