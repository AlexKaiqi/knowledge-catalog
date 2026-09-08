# 公开 CLI 操作语义（审查稿）

给人逐条批用。**路径权威**是 `surface.go`；**HTTP 面权威**是 `service_routes.go` / `service_management_routes.go`；协议动词仍在各包 README。本文不是第二份命令清单，也不是设计文档：批完要么改 `surface.go` / `help.go`，要么删掉与合同重复的段落。

分组按进程和 Client 打到的 Server namespace，不按岗位。`kc help consume|write|compose` 只在文末当最短路径。

批评时直接在对应节下加 `批：`。

---

## 0. 进程

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| P0 | `kc help [topic]` | 打印公开 CLI 或一条最短 Client 路径 | 不是协议动词 |
| P1 | `kc serve` | 读取 `--config`，恢复既有 Catalog Git 与独立耐久状态；长寿命进程追踪 published HEAD | 不是 Client；产品命令不经这条 argv 读知识 |
| P2 | `kcfs plan` / `mount` | 把已经固定的 Workspace pin 投影成只读目录（`/workspace-files/v1`） | 不是 `kc` 动词；不写知识；不另解对象 |

---

## 1. 部署 `kc deployment`（读取声明式配置）

配置与状态独立于实例缓存。配置类型是 `home.DeploymentConfig`；Catalog Git、Snapshot、耐久控制状态、秘密和派生缓存分别有自己的恢复来源。

| ID | 操作 | 语义 | 边界 |
|---|---|---|---|
| D1 | `deployment init --config <file>` | 显式初始化配置声明的 Catalog Git 与空耐久控制状态，建立首个管理主体 | 不创建业务 Snapshot；已有部署不可被覆盖成空态 |
| D2 | `deployment status --config <file>` | 只读检查配置、Catalog authority、binding 与耐久状态 | 缺必需状态失败关闭，不能回退本机目录发现 |
| D3 | `deployment system publish --config <file>` | 向配置中 `kr://kc/system` 的 binding 显式发布内置信任根 | 只允许此信任根发布例外；业务 Writer 仍不可写 System 仓 |
| D4 | `deployment identity migrate --config <file> --file <request.json>` | 停服维护时由运营者显式核实用户名、provider、issuer、subject 并迁移旧主体尚存授权 | 原子保存一次迁移回执；不恢复已撤销规则，不自动接管同名主体；已有不同绑定拒绝 |

`serve --config` 只恢复；没有 `local` 命令或独立 `catalog repo register`。新增 Catalog 与静态存储 binding 由配置管理；托管仓的分配和连接由 Server 持久保存，不通过本机目录命令修改。初次初始化不承诺 Git 与状态卷的跨介质事务；已有 Catalog 而缺状态时要求恢复或核对未完成初始化。

---

## 2. 身份 `/identity/v1`

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| I1 | `login` | 先读 Server 接受哪种凭证，再把配对存本机 | Server 不建 session |
| I2 | `logout` | 清本机配对 | 不通知 Server |
| I3 | `whoami` | 当前请求被认证成哪个 principal（及可选 onBehalfOf） | 不列 grant |
| I4 | `admission show` | 本人能否显式申请部署的首次准入、原决定及当前尚存动作 | 不在登录时发权，不向客户端交付发权参数 |
| I5 | `admission request` | 按部署配置为当前可信用户一次性应用准入策略 | 不接受指定其他主体；重放不恢复已撤销权限 |

---

## 3. Catalog 读 `/catalog/v1`（发现）

门槛：`catalog.read`。都不返回对象正文。消费发现：`list` → `show`（选知识集）→ `pin`（钉版本）→ 才进知识读面。

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| CR1 | `catalog list` | 哪些 Catalog 对当前主体可见。只出 `{id}` | 不含仓、知识集、对象 |
| CR2 | `catalog show` | 这一间的**当前组合态**：承认哪些源 + 有哪些命名知识集。源上可附 title/summary（应用层读该仓 published HEAD 的保留源说明对象拼出；缺说明是 `profile: missing`） | 不是对象目录；知识集只有成员仓 id，没有 commit / selector；不是 git 历史；不含宿主路径 |
| CR3 | `catalog repo list` | CR2 的「承认哪些源」切片（同样带源说明对象） | 只列此 Catalog 已完成接入的成员 |
| CR4 | `workspace list` | 命名知识集名单：id + revision + 成员仓 id | 不是 pin |
| CR5 | `workspace show` | 一条配方的当前定义 | 不解 selector，不读对象 |
| CR6 | `workspace pin` | 把命名配方（或临时 `--source` / `--file`）解成这次任务的 `{仓 → commit}`。固定后不追随新 HEAD。`--out` 时 pin 进文件，stdout 只含 `workspaceId` / `pinId` / `out` | 不是对象 RESOLVE；不解 `object_id`；不发读权 |
| CR7 | `workspace check` | 对**已经 resolve 的 pin**：成员仓是否仍 attach、commit 是否仍在 | 不检查配方写得对不对；不读对象 |
| CR9 | `workspace overlay --file <recipe> --overlay <overlay> [--out <file>]` | 客户端把个人 overlay 合成为临时 WorkspaceDefinition，后续 `workspace pin --file` 固定它 | 不打开 Server Home、不修改共享 Catalog 配方 |
| CR8 | `catalog audit` | 登记表自己怎么变过来的（define / register / retire 那些 yaml 的 git） | 不是对象历史（`knowledge log`）；不是谁搜过/读过（`operations audit`）。CLI `--layer` 选择耐久过程账，HTTP 能力以正式注册表为准 |

---

## 4. Catalog 写 `/catalog/v1`（组合）

成员登记和配方管理不写知识正文，也不发权。平台托管仓创建是应用服务流程，另按显式部署策略完成初始授权。

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| CW1 | `catalog repo attach --repo <id>` | 使用预配置 binding，只读验证既有 Snapshot，再原子提交 Catalog 成员登记 | 不创建仓、不改 Snapshot ref、不发权；失败无半登记 |
| CW2 | `catalog repo archive` | 该仓在本 Catalog 生命周期结束（System 仓禁止） | 不删 Snapshot 里的对象 |
| CW3 | `workspace define` | 发布/改一条命名知识集：哪些仓、跟哪根已发布 selector | 不是写入前置条件；不解成 commit；不发权 |
| CW4 | `workspace retire` | 这条配方不能再被 Open / 消费 | 不归档整本 Catalog |
| CW5 | `catalog archive` | 整间 Catalog 只读历史 | 没有 DELETE |
| CW6 | `catalog repo create --catalog <id> --repo <id> --command-id <id>` | Client 申请平台托管仓；服务供给、持久保存连接、登记成员，并按显式策略给认证主体新仓能力 | 不接管已有仓；不接收目录、DSN、凭证或任意 grant；不扩大到他人仓 |
| CW7 | `catalog repo share add\|list\|remove --repo <id>` | 有本仓分享权的主体将显式策略允许且自己当前拥有的消费动作授予其他用户名；按本仓 share ID 查看和撤销 | 不授予再转授或管理动作，不把局部对象/ref授权扩大，不删除管理员规则或别仓分享 |
| CW8 | `catalog repo connect --catalog <id> --repo <id> --url <url> --credential-file <path>` | 只读验证部署批准 provider 上的已有 Gitea Snapshot，保存私有连接并登记成员，按显式策略一次应用初始授权 | 不初始化或写外部仓，不接受任意 Server 目录，不隐式发权 |
| CW9 | `catalog repo connection show\|check\|rotate --repo <id>` | owner 在当前仓级连接管理权下查询管理 URL、只读复查、或验证同 authority 后轮换 credential | 不变更 endpoint/知识身份，失败不替换旧 credential，正文读权独立 |

CW6 的动作是 `catalog.repositories.create`，Catalog 必须显式指定，不通过 `catalog.read` 发现默认值。请求重试保留 command-id；成功响应的 `APPLIED` / `REPLAYED` 与原仓身份保持一致。供给、Catalog Git 和权限存储不构成跨介质事务，失败不得报告创建成功，也不能丢掉已开始分配的恢复证据。普通 attach 的只读合同不变。

---

## 5. Writer `/writer/v1`

一次只写一个 Repository。代数只有 PUT / REMOVE。

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| W1 | `pack` | **Client 预处理**：目录 → ChangeSet（+ diagnostics）；显式 `--base` 固定写入基点。`--out` 时 stdout 不含 ChangeSet | 不连接 Server、不解析身份、不发布 |
| W2 | `writer commit` | 把已有 ChangeSet 提交进权威（CAS / command-id 幂等） | 不经 Workspace |
| W3 | `writer put` | 单条 PUT 的 commit 糖 | Server 仍是 `POST …/commits` |
| W4 | `writer remove` | 单条 REMOVE 的 commit 糖 | 同上 |
| W5 | `writer head` | 该仓某 ref 当前 commit | 不是对象内容 |
| W6 | `writer receipt` | 按 `command-id` 查那次写的回执 | 不是知识 READ |

HTTP 提案只走 `/governance/v1/proposals`；CLI 的 proposal 走 §7，不走 `kc writer`。

---

## 6. Knowledge `/knowledge/v1`

知识命令可使用命名 `--workspace`、临时 `--source` / `--workspace-file`、已保存的任务 `--pin`，或 `--repo` 单仓维护基点。临时 pin 保存定义与 Catalog 身份，后续命令可直接消费；不能混入另一套仓或配方坐标。没有公开 LIST。SEARCH 命中后 hydrate，交付链按仓 `knowledge.read` 屏蔽正文。

| ID | 操作 | 语义 | 不是 |
|---|---|---|---|
| K1 | `knowledge schema list` | 一仓 Schema 目录分页（选知识集之前就能用） | 不是实例目录；不是 CR2 |
| K2 | `knowledge schema describe` | 某对象/范围字段的 `text/filter/sort` 逻辑访问语义 | 不返回实例正文 |
| K3 | `knowledge search` | 在命名、临时任务或单仓的固定版本上定位候选 | 零命中 ≠ 面不可用；不枚举仓；不代替 READ；不是 `search --catalog`（未提供） |
| K4 | `knowledge resolve` | 此 basis 上该对象在不在（缺 = unresolved） | 不是 Workspace pin（CR6）；不是空 READ |
| K5 | `knowledge read` | 此 basis 上的 Canonical 正文。成员读权不齐 fail closed | 不追随 live ref |
| K6 | `knowledge relations` | 从某对象查出关系边 | 不扫全仓；返回对象也要成员读权 |
| K7 | `knowledge provenance` | 单元上的来源信封 | 不是 git log |
| K8 | `knowledge log` | 该对象各 digest 是哪些 commit 引入的 | 不是登记表历史（CR8） |
| K9 | `knowledge binding show` | 取出 Aspect Binding **声明** | 不调 live、不取数 |
| K10 | `knowledge access` | 按 Binding `--aspect` 调用墙外 StateLookup | 不是 K9；不是 OP3；不是 K11 |
| K11 | `knowledge invoke` | 调 ResourceDescriptor 上声明的一次 `--operation --input` | 不是 Operations 动词；不是 K10 |

HTTP 还有 `POST /knowledge/v1/search:rerank`、`/rerank`，无对应 `kc` 命令。`knowledge access` 与 `knowledge invoke` 共用 `POST /knowledge/v1/resources:access`。

未提供（不得写入当前入口）：`knowledge search --catalog` / Catalog `discoveryWorkspaceId`；交付链首段之后的隐私化（`PERMISSIONS.md` Non-Goal，未选定）。缺口台账 [`docs/MVP_ACCEPTANCE.md`](../docs/MVP_ACCEPTANCE.md)。

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
| OP3 | `operations projection notice` | 观察方报告 Bound State/Stream 变了；控制器按固定 Binding 拉，不与 Snapshot HEAD 合成一个 key | 不是 Snapshot commit；不带正文 |
| OP4 | `operations access-spec describe` | 此 pin 上每成员一份 AccessSpec（能按什么字段搜/滤） | 不是授权诊断；不是 K10 |

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

HTTP 还有 retrieval-log / retrieval-training / refine-log / rerank-training，无对应 `kc` 命令。help 总表「HTTP-only」是这组闭集。

---

## 11. help 主题（不是 Server 面）

`help.go` 把 Client 最短路径收成三个旅程 topic。协议没有这些主体；授权键仍是 `principal × action × repository|catalog`。

| topic | 覆盖 |
|---|---|
| `consume` | login → catalog list/show → schema list → workspace pin --out → search/read |
| `write` | 申请平台托管仓（已有仓可省略）→ pack → commit/put → 用 `--repo` 回读 |
| `compose` | catalog repo attach → workspace define → grant |

`kc help governor|consumer|provider` 非零退出。总表节名按 Catalog / Workspace / Pack / Writer / Knowledge / Admin / Governance / Operations。操作数：Catalog/Workspace 位置参数（`--catalog`/`--workspace` 仍可用），对象 `--object`，仓 `--repo`。`--workspace` 与 `--repo` 混用是 `USAGE_INVALID`。

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
| K9 vs K10 vs K11 vs OP4 | 声明 vs Binding 墙外观察 vs Descriptor 操作 vs 检索能力说明书 |
| OP2 vs OP3 vs P1 | 排障重建 vs 动态观察通知 vs 进程自己追 published Snapshot |
| A1 vs CW3 | 发权 vs 组配方；组配方不发权 |
