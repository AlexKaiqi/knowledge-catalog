# 公开 CLI 操作语义（审查稿）

给人逐条批用。**路径权威**是 `surface.go`；**HTTP 面权威**是 `service_routes.go` / `service_management_routes.go`；协议动词仍在各包 README。本文不是第二份命令清单，也不是设计文档：批完要么改 `surface.go` / `help.go`，要么删掉与合同重复的段落。

分组按进程和 Client 打到的 Server namespace，不按岗位。`kc help consumer|provider|governor` 只在文末当现状附录。

批评时直接在对应节下加 `批：`。

---

## 0. 进程

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| P0 | `kc help [topic]` | 打印公开 CLI 或一条最短 Client 路径 | 不是协议动词 |
| P1 | `kc serve` | 打开 Home，启动唯一知识入口；长寿命进程把检索投影追到各源 published HEAD | 不是 Client；产品命令不经这条 argv 读知识 |
| P2 | `kcfs plan` / `mount` | 把已经固定的 Workspace pin 投影成只读目录（`/workspace-files/v1`） | 不是 `kc` 动词；不写知识；不另解对象 |

---

## 1. 宿主 `kc local`（永不进 HTTP）

塑造这台机器上的 Home。不回答「有什么知识」。

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| H1 | `local init` | 建 Home，并创建第一间 Catalog 登记表 | 不发布知识，不发权 |
| H2 | `local status` | 本机装配：挂了哪些仓、引擎、Catalog | 不是 `catalog show`（无宿主路径的组合库存） |
| H3 | `local catalog attach` | 本机再打开一间 Catalog 登记表 | 不把仓登记进配方 |
| H4 | `local repository attach` | 本机挂上某个 Snapshot 权威 | 不等于 Catalog 承认该源；不发 `knowledge.read` |
| H5 | `local store show` | 看本机 adapter / DSN（无密钥） | 不是库存，不是知识 |
| H6 | `local store set` | 改本机 adapter / DSN | 不是协议写面 |
| H7 | `local grant bootstrap` | allow 为空时写入第一个管理员；已有任何 rule 即失败 | 不是业务 `admin grant` |
| H8 | `local system publish` | 把内置 `kr://kc/system` 写到可 clone 的权威上 | 不是业务 Writer |
| H9 | `local workspace overlay` | 给某个 principal 在本机盖一层配方 | 不发布 `WorkspaceDefinition` |

---

## 2. 身份 `/identity/v1`

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| I1 | `login` | 先读 Server 接受哪种凭证，再把配对存本机 | Server 不建 session |
| I2 | `logout` | 清本机配对 | 不通知 Server |
| I3 | `identity whoami` | 当前请求被认证成哪个 principal（及可选 onBehalfOf） | 不列 grant |

---

## 3. Catalog 读 `/catalog/v1`（发现）

门槛：`catalog.read`。都不返回对象正文。消费发现：`list` → `show`（选知识集）→ `resolve`（钉版本）→ 才进知识读面。

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| CR1 | `catalog list` | 哪些 Catalog 对当前主体可见。只出 `{id}` | 不含仓、知识集、对象 |
| CR2 | `catalog show` | 这一间的**当前组合态**：承认哪些源 + 有哪些命名知识集。源上可附 title/summary（应用层读该仓 published HEAD 的保留源说明对象拼出；缺说明是 `profile: missing`） | 不是对象目录；知识集只有成员仓 id，没有 commit / selector；不是 git 历史；不含宿主路径 |
| CR3 | `catalog repository list` | CR2 的「承认哪些源」切片（同样带源说明对象） | 不是 `local repository attach` 名单 |
| CR4 | `catalog workspace list` | 命名知识集名单：id + revision + 成员仓 id | 不是 pin |
| CR5 | `catalog workspace show` | 一条配方的当前定义 | 不解 selector，不读对象 |
| CR6 | `catalog workspace resolve` | 把命名配方（或临时 `--source`）解成这次任务的 `{仓 → commit}`。命令内冻结，不落盘 | 不是对象 RESOLVE；不解 `object_id`；不发读权 |
| CR7 | `catalog workspace check` | 对**已经 resolve 的 pin**：成员仓是否仍 attach、commit 是否仍在 | 不检查配方写得对不对；不读对象 |
| CR8 | `catalog audit` | 登记表自己怎么变过来的（define / register / retire 那些 yaml 的 git） | 不是对象历史（`knowledge log`）；不是谁搜过/读过（`operations audit`）。CLI `--layer` 会改读本机 jsonl，HTTP 没有这个开关 |

---

## 4. Catalog 写 `/catalog/v1`（组合）

改「承认谁、组哪盘」。不写知识正文，不发权。

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| CW1 | `catalog repository register` | 这间 Catalog **承认**该仓可以进配方 | 不挂存储（H4）；不给任何人读权 |
| CW2 | `catalog repository archive` | 该仓在本 Catalog 生命周期结束（System 仓禁止） | 不删 Snapshot 里的对象 |
| CW3 | `catalog workspace define` | 发布/改一条命名知识集：哪些仓、跟哪根已发布 selector | 不是写入前置条件；不解成 commit；不发权 |
| CW4 | `catalog workspace retire` | 这条配方不能再被 Open / 消费 | 不归档整本 Catalog |
| CW5 | `catalog archive` | 整间 Catalog 只读历史 | 没有 DELETE |

---

## 5. Writer `/writer/v1`

一次只写一个 Repository。代数只有 PUT / REMOVE。

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| W1 | `writer ingest` | **Client 预处理**：目录 → ChangeSet（+ diagnostics）。`--out` 时 stdout 不含 ChangeSet | **不发布**；不是 Server 写面（最多先 `GET head`） |
| W2 | `writer commit` | 把已有 ChangeSet 提交进权威（CAS / command-id 幂等） | 不经 Workspace |
| W3 | `writer put` | 单条 PUT 的 commit 糖 | Server 仍是 `POST …/commits` |
| W4 | `writer remove` | 单条 REMOVE 的 commit 糖 | 同上 |
| W5 | `writer head` | 该仓某 ref 当前 commit | 不是对象内容 |
| W6 | `writer receipt` | 按 `command-id` 查那次写的回执 | 不是知识 READ |

HTTP 另有 `POST /writer/v1/…/proposals`；CLI 的 proposal 走 §7，不走 `kc writer`。

---

## 6. Knowledge `/knowledge/v1`

双靶：`--workspace` + pin（联邦 Serving）或 `--repo`（单仓维护读）。没有公开 LIST。SEARCH 命中后 hydrate，交付链按仓 `knowledge.read` 屏蔽正文。

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| K1 | `knowledge schema browse` | 一仓 Schema 目录分页（选知识集之前就能用） | 不是实例目录；不是 CR2 |
| K2 | `knowledge schema describe` | 某对象/范围字段的 `text/filter/sort` 逻辑访问语义 | 不返回实例正文 |
| K3 | `knowledge search` | 在固定 pin 上定位候选 | 零命中 ≠ 面不可用；不枚举仓；不代替 READ |
| K4 | `knowledge resolve` | 此 basis 上该对象在不在（缺 = unresolved） | 不是 Workspace pin（CR6）；不是空 READ |
| K5 | `knowledge read` | 此 basis 上的 Canonical 正文。成员读权不齐 fail closed | 不追随 live ref |
| K6 | `knowledge relations` | 从某对象查出关系边 | 不扫全仓；返回对象也要成员读权 |
| K7 | `knowledge provenance` | 单元上的来源信封 | 不是 git log |
| K8 | `knowledge log` | 该对象各 digest 是哪些 commit 引入的 | 不是登记表历史（CR8） |
| K9 | `knowledge binding resolve` | 取出 Aspect Binding **声明** | 不调 live、不取数 |
| K10 | `resource access`（`--aspect`） | 按 Binding 调用墙外 StateLookup | 不是 K9；不是 OP3 |
| K11 | `resource access`（`--operation --input`） | 调 ResourceDescriptor 上声明的一次操作 | 不是 Operations 动词 |

HTTP 还有 `POST /knowledge/v1/search:rerank`、`/rerank`、`/addresses:read`，无对应 `kc` 命令。

---

## 7. Admin `/admin/v1`

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| A1 | `admin grant add` | 给 principal 发稳定动作（如 `catalog.read`、`knowledge.read`），范围是 Catalog 或 Repository | 不是 CLI 命令名；成为 Workspace 成员不隐含读权 |
| A2 | `admin grant list` | 当前规则 | 不是 whoami |
| A3 | `admin grant remove` | 撤一条规则。已发出的 pin 不能靠旧 pin 绕过撤权 | 不删知识 |

---

## 8. Governance `/governance/v1`

只拦 **proposal → 已发布 ref**。不是日常 COMMIT，不是 READ。

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| G1 | `governance proposal create` | 登记一个候选：目标 ref + candidate ref + PUT/REMOVE | 不是 commit；不改 published HEAD |
| G2 | `governance preview create` | 把 proposal overlay 到某个 Workspace pin 上，得到不可混淆的 Preview basis | 不是 CR2；不是 W1 的 ChangeSet 预览 |
| G3 | `governance preview validate` | 对该 Preview 做协议结构检查 | 不跑业务套件 |
| G4 | `governance validation record` | 只绑定外部套件已给出的 PASSED/FAILED | 不执行检查 |
| G5 | `governance proposal merge` | 清单齐则快进仓 Ref；下次 CR6 自然看到新 HEAD | 不在 merge 时打电话问外部系统 |

---

## 9. Operations：检索派生 `/operations/v1`

索引只定位。一索引绑 `(仓, basisCommit)`，不绑 Workspace。精确 READ 不依赖投影。live published HEAD 由 P1 追。

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| OP1 | `operations projection describe` | 该仓该 commit 上派生索引是否就绪、覆盖什么 | 不是 SEARCH |
| OP2 | `operations projection sync` | 强制把 Snapshot 投影（及可选 State）建到指定 commit：历史 pin、重建、排障 | 不是写入；不是消费命令 |
| OP3 | `operations projection notify` | 观察方报告 Bound State/Stream 变了；控制器按固定 Binding 拉，不与 Snapshot HEAD 合成一个 key | 不是 Snapshot commit；不带正文 |
| OP4 | `operations access describe` | 此 pin 上每成员一份 AccessSpec（能按什么字段搜/滤） | 不是授权诊断；不是 K10 |

---

## 10. Operations：出站、Gate、过程证据

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| OE1 | `operations hook add\|list\|remove` | 某 `kc` 动作前后的出站通知/调用。pre 只能机械否决 | 不是权限；不是 merge 证据 |
| OE2 | `operations gate add\|list\|remove` | merge 必须具备的、已绑定 Preview 的证据项 | 不是 hook；不跑套件 |
| OE3 | `operations audit access` | 谁对哪个固定 `{仓,commit,object}` 做了什么（允许/拒绝） | 不是 CR8 |
| OE4 | `operations audit trace` | 按 trace 串起一次调用 | 不是检索执行 |
| OE5 | `operations audit hitmap` | 从访问账派生的命中统计 | 不是 Canonical |
| OE6 | `operations feedback record` | 按 trace 记下采用/无用等反馈 | 不写回知识 |

HTTP 还有 retrieval-log / retrieval-training / refine-log / rerank-training，无对应 `kc` 命令。

---

## 11. 现状：help 主题（不是 Server 面）

`help.go` 把 Client 最短路径收成三个 topic。协议没有这些主体；授权键仍是 `principal × action × repository|catalog`。

| topic | 实际覆盖的节 | 问题（待批） |
|---|---|---|
| `provider` | §5 + 用 `--repo` 的 K5/K7/K1 | 与 Writer 面对得上 |
| `consumer` | §3 读 + §6 | 与 Catalog 读 + Knowledge 面对得上 |
| `governor` | §4 + §7 + §8 + OP2/OP3 + CR8 | 对不上任何一个 namespace；又和 Governance API 撞名 |

`kc help` 总表的节名也和 namespace 错位：Catalog composition 混 CR+CW；Writing and governance 混 W+G；Operations 里塞了 K10/K11。

---

## 12. 易混（批命名时用）

| 易混 | 差在哪 |
|---|---|
| H4 vs CW1 | 机器有没有权威 vs Catalog 承不承认进配方 |
| H9 vs CW3 | 本机 overlay vs 发布命名知识集 |
| CR2 vs CR6 | 当前配方（id）vs 这次任务的 commit pin |
| CR6 vs K4 vs K9 | 钉版本 vs 对象在不在 vs 取出 live 句柄声明 |
| CR7 vs CR6 | 校验已有 pin vs 算出 pin |
| CR8 vs K8 vs OE3 | 登记表 git vs 对象修订 vs 访问账 |
| W1 vs W2 | 收成 ChangeSet vs 真正进权威 |
| G2 vs W1 | 治理 Preview vs Client 文件预览 |
| K9 vs K10 vs OP4 | 声明 vs 真去打外部 vs 检索能力说明书 |
| OP2 vs OP3 vs P1 | 排障重建 vs 动态观察通知 vs 进程自己追 published Snapshot |
| A1 vs CW3 | 发权 vs 组配方；组配方不发权 |
