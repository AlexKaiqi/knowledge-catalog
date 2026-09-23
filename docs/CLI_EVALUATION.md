# 产品 CLI 六维评价

日期：2026-09-16

方法的应然在 [`CLI.md`](CLI.md) §6。本文按协议场景判定**全部**公开产品命令：`cliSurface` 58 条，外加不在闭集里的 `serve`、`kcfs plan`、`kcfs mount`。不复制 argv 闭集，也不把 HTTP DTO 写成产品 stdout。数仓夹具不是本表的场景。

检查：`TestCLIEvaluationDailyJourneyProductStdout`（`read` / `grant list` / `schema describe` / `resolve` 产品 stdout）、`TestShapeCLIProductDailyCommands`（含 `search` / `relations` / `hitmap` 编码）。HTTP 仍是协议 DTO（`API-01`）。

证据来自 `.data/scenes/` 的 construct/probe、宿主上点名的独立 Go 用例，以及叶子 help 第一句。Go 用例使用自己的 setup，不表示它消费了宿主的场景 fixture。没有三人组，对不能判过。

## 读表

成立只认问、刀、对全过。形、侧、路是诊断。日常误放 = 最短旅程里的命令前三维已经不过。

刀必须写出对手。对必须有成功 stdout、失败 stdout、叶子 help 第一句。远程不是刀。

场景入口是 `.data/scenes/` 的真实可复用前态及其独立用例，不另维护命令树。下表区分 construct、具名 probe 与独立 Go 证据；用例的一次性后态不作为目录状态。最新关系由 `python3 .data/scenes/tree.py` 生成，产品条目定位用 `--family product`。

## 编码前失败（最短旅程）

编码前，日常消费路径里这三条问、刀过了，对不过：

| 命令 | 问 | 当时 stdout | 为什么对不过 |
|---|---|---|---|
| `read` | 打开这一份知识的正文 | `KnowledgeValue`：`knowledgeRef` + `address` + `units` + `declarations` | 人要的是正文；拼装记录是协议信封 |
| `search` | 这仓里哪些知识匹配这句话 | `hits[].knowledge` 嵌套整份 `KnowledgeValue`，另有 `searchView` | 定位和打开混成一次；无 `read` 时还在答投递链 |
| `grant list` | 这仓现在有哪些授权规则 | 无 principal+action 时整份 `allow.json`（`version`、`initialGrants`） | 列表不是配置文件转储；`--repo` 不过滤 |

这三条的产品编码见 [`CLI.md`](CLI.md) 选定方案。HTTP SEARCH/READ 与 AllowFile 仍是协议形状（`API-01`）。

编码前 canvas 还把 `create`、`pack` 标成日常误放。独立复核后：`create` 双模式是 CLI.md 已选定的供给/连接。`pack` 已从产品面拿掉。目录写入走 `writer commit --dir`；可选 `kc diff` 看同一对照，不是提交前必经步骤。

## 本轮优化后再评

进阶六条原先不成立。产品 CLI 打印路径与 help 收口后重评，问刀对全过。HTTP 仍是协议形状。

| 命令 | 原先不过 | 现在 stdout / help |
|---|---|---|
| `schema describe` | 刀：看起来像 list | `{repository, commit, schemas[].objectId, fields[{path,type,access}]}`，可选 `origin`，无 `entity`。help「列出字段能否 filter、text、sort，以及 Bound State 访问原点」 |
| `resolve` | 刀/对：像阉割的 `read` | `{repository, objectId, commit, status}`，无 Address 信封。缺对象是 `UNRESOLVED` 成功回执（与 HTTP 对齐） |
| `relations` | 对：`searchView` 信封 | hits `{repository, objectId, commit, relationType, matchedRoles}`。无投影 → `CAPABILITY_UNSATISFIED` |
| `writer put` | 刀：像跳过目录提交 | help「直接发布一个 Address，不经过草稿目录」；对手是 `writer commit --dir` |
| `governance preview create` | 对：只有 previewId | `{previewId, candidate.repositoryId, candidate.commitId, repositories}`；`read --repo` 仍旧值是侧正确 |
| `operations audit hitmap` | 对：`source:access` | `{source:hitmap, hits}`。访问账仍是 `source:access` |
| `governance proposal create` | 形：help 只有 `--candidate`，construct 还带 `--object/--value` | 问就是「把这次变更写到 candidate」。help 与 `USAGE_INVALID` 都要求 `--value` / `--file` / `--changeset`。不拆成两条命令 |

形、侧收口：`dataset define` / `dataset retire` 的 `--dataset`；`dataset overlay` 的 `--file`+`--overlay`；`relations --direction DIRECTED\|UNDIRECTED`；`projection notice` 第一句改为观察变化；`resolve` 缺对象用 `UNRESOLVED` 成功回执（与 HTTP 对齐，写入 CLI.md 选定）。

---

## 1. 进房 — consume 前半

场景：`catalog-initialized` 提供初始 Catalog 和空授权。`http-served`、`catalog-read-granted`、`grants-bootstrapped` 是其不同前态分支，分别供身份配对、读权边界和管理者发现用例复用。

| 命令 | 场景 | 一句语义 | 问 | 刀（对手） | 对 | 形 | 侧 | 路 | 成立 |
|---|---|---|---|---|---|---|---|---|---|
| `login` | `http-served` | 验证身份并保存当前 Server 的本机会话 | 过 | 过：`whoami`（只看）、HTTP `Authorization` | 过：local `{status:authenticated, principal, mode:local}`；Taihu 未完成 `{auth_required, auth_url, expires_in, next_step:kc login --wait}`。help「验证身份并保存当前 Server 的本机会话」 | 过：两种 stdout 对应两种模式，与 `create` 双模式同类 | 过：回执不带 Server 地址 | 日常 | 成立 |
| `logout` | `http-served` | 清除当前 Server 的本机会话 | 过 | 过：`login` | 过：`{status:logged out}`。help「清除当前 Server 的本机会话」 | 过 | 过 | 日常 | 成立 |
| `whoami` | `catalog-initialized` / `http-served` | 当前认证主体是谁 | 过 | 过：`admission show`（权与入口）、`login`（建立会话） | 过：`{principal}`，`grants` 缺席。help「显示当前认证主体」 | 过 | 过：不列 grant | 日常 | 成立 |
| `admission show` | `http-served` / `grants-bootstrapped` | 本人已有哪些权、去哪申请、谁能发权 | 过 | 过：`whoami`、`grant list`（管理者视图） | 过：`{principal, grants, request.url, request.administrators}`；无 `status`/`eligible`/`currentActions`。未 bootstrap 时 administrators 空。help「显示本人已有权限和外部申请入口」 | 过：名字有申请味，stdout 已否决队列 | 过：KC 不托管申请队列 | 日常 | 成立 |
| `catalog list` | `catalog-initialized` / `catalog-read-granted` | 我能看见哪几间 Catalog | 过 | 过：`show`（一间库存）、HTTP 集合 GET | 过：`{catalogs:[{id}]}`。help「列出可见 Catalog」 | 过 | 过：只有一间可见时自动 `use` 是 CLI.md 选定，不是对失败 | 日常 | 成立 |
| `catalog use` | `catalog-read-granted` | 选择当前 Catalog | 过 | 过：`catalog list`、每次命令再传 catalog | 过：`{catalogId}`。缺 id → `USAGE_INVALID`。help「选择当前 Catalog」 | 过 | 过 | 日常 | 成立 |
| `show` | `catalog-initialized` / `repository-attached` 的库存隔离用例 | 当前 Catalog 挂了哪些仓和知识集 | 过 | 过：`catalog list`、`schema list`（实体不是库存）、对象 LIST（已否决） | 过：`{catalogId, repositories[].id, datasets}`；`home`/`title`/`summary` 缺席。help「显示当前 Catalog 的 Repository 与知识集」 | 过 | 过：`catalog.read` 不放行正文 | 日常 | 成立 |

`catalog-initialized` 上的退役 checkout probe 与 `TestRemovedCommandsAreRejected` / `TestAppendAndStreamSurfacesStayAbsent` 钉住：没有对象 LIST、checkout/export、MCP 正路径。缺的不是本表漏判。

---

## 2. 挂源给权 — write / compose 供给

场景：`catalog-initialized` 挂平台建仓的具名 Go 旅程与独立撤权 probe；`repository-attached` 挂库存隔离、写权隔离和 detach 用例。建仓、撤权、detach 的验证结果没有另建状态。

| 命令 | 场景 | 一句语义 | 问 | 刀（对手） | 对 | 形 | 侧 | 路 | 成立 |
|---|---|---|---|---|---|---|---|---|---|
| `create --name` | `catalog-initialized` 的平台建仓 Go 证据 | 向平台要一个空仓 | 过 | 过：`git init`、直接改 Catalog 文件、HTTP POST 集合 | 过：`{catalog, repositoryId, status:APPLIED, head}`；无 `--name`/`--url` → `USAGE_INVALID`。help「创建托管 Repository 或连接自有 Repository」 | 过 | 过：不 attach、不发 `knowledge.read` | 日常 | 成立 |
| `create --url` | `catalog-initialized` 的连接入口 Go 证据 | 把已有仓接到 KC 能打开 | 过 | 过：`attach`（登记）、裸 clone | 过：与 `--name` 互斥；缺 `--credential-file` → `USAGE_INVALID` | 过 | 过：连接 ≠ 挂进 Catalog | 日常 | 成立 |
| `attach` | `repository-attached` | 把已打开的仓挂进当前 Catalog | 过 | 过：`create`、改登记表文件 | 过：`{catalog, repositoryId}`，随后 `show` 见成员。help「把已连接 Repository 挂入当前 Catalog」 | 过 | 过：不要求已发布，不发读权 | 日常 | 成立 |
| `detach` | `repository-attached` / `probe-detach-keeps-head-readable.feature` | 从当前 Catalog 拿下这个仓 | 过 | 过：`catalog archive`（整间房）、删 Snapshot | 过：`{repositoryId, detached:true}`；`writer head` 仍有 commit。help「从当前 Catalog 拿下 Repository」 | 过 | 过：不删对象 | 日常 | 成立 |
| `grant add` | 复用授权前态的 construct；一次性发权在各独立 probe | 允许某人在这仓、这间 Catalog 或这个 Dataset 做一类动作 | 过 | 过：改 `allow.json`、HTTP POST shares | 过：`{id, principal, catalog\|repository, actions}`。help「发放一条仓、Catalog 或 Dataset 范围的授权规则」 | 过 | 过：`--repo` 与 `--catalog` 二选一；Dataset 消费权加 `--dataset`；不能发 `*` | 日常 | 成立 |
| `grant list` | `catalog-initialized` / `*-granted` | 这范围现在有哪些授权规则 | 过 | 过：`admission show`（本人）、AllowFile 原文 | 过：`{rules:[…]}`；空登记 `{rules:[]}`。help「列出授权规则」 | 过 | 过：`--repo`/`--catalog` 过滤 | 日常 | 成立 |
| `grant remove` | `catalog-initialized` / `probe-revoke-catalog-read-denied.feature` | 撤销这一条授权规则 | 过 | 过：`grant add`、`detach` | 过：`{revoked:alw_1}`，随后 `rules:[]`。help「撤销一条授权规则」 | 过 | 过：立即生效，旧 pin 不能绕过（KS-02） | 日常 | 成立 |

---

## 3. 读知识 — consume 后半

场景：System 可读与不可写用例挂在 `catalog-initialized`；`schema-read-granted` 供两个 Schema 边界用例复用。`projection-synced` 提供索引，`knowledge-search-granted` 上分别验证搜索权限和临时增加读权后的交付；对象史在 `knowledge-published`。这些分支不是一条权限继承链。

| 命令 | 场景 | 一句语义 | 问 | 刀（对手） | 对 | 形 | 侧 | 路 | 成立 |
|---|---|---|---|---|---|---|---|---|---|
| `schema list` | `catalog-initialized` 的 System 浏览用例与 Go 分页证据；`schema-read-granted` | 这仓发布了哪些实体类型 | 过 | 过：`show`、空 `search`、对象 LIST、`read` 合同正文 | 过：`{repository, commit?, schemas[].objectId, entity, description}`。无 `--repo` → `USAGE_INVALID`。help「列出一个 Repository 已发布的实体，不是对象目录或合同正文」 | 过 | 过：`catalog.read` 不隐含 | 日常 | 成立 |
| `schema describe` | `schema-read-granted` | 这种实体哪些字段能搜、能滤 | 过 | 过：`schema list`（实体目录）、`read` 合同正文 | 过：`{repository, schemas[].objectId, fields[{path,access}]}`，可选 `origin`，`entity` 缺席。help「列出字段能否 filter、text、sort，以及 Bound State 访问原点」 | 过 | 过：不返回实例正文 | 进阶 | 成立 |
| `search` | `knowledge-search-granted` | 这仓里哪些知识匹配这句话 | 过 | 过：`read`、Linux `rg`、空查询当浏览 | 过：hits `{repository, objectId, commit}`；无 query → `USAGE_INVALID`。help「按 Schema AccessHints 检索知识，列出匹配对象，不是文件 contains」 | 过 | 过：无 `knowledge.read` 仍定位、不带正文 | 日常 | 成立 |
| `read` | `knowledge-search-granted` / `read-grant-returns-canonical.feature`；`knowledge-published` | 打开这一份知识的正文 | 过 | 过：`search`、`log`/`provenance`、Linux `cat`、`writer head` | 过：`{repository, objectId, commit, value}`。help「读取一个知识对象在固定 commit 上的正文」 | 过 | 过 | 日常 | 成立 |
| `resolve` | `knowledge-published`；`dataset-query-principals-granted` 的消费用例 | 这一份知识在不在、钉在哪一版 | 过 | 过：`read`（打开正文）、`writer head`（仓 HEAD） | 过：`{repository, objectId, commit, status}`，`address` 缺席。help「检查对象是否存在并给出解析到的 commit，不打开正文」 | 过 | 过：缺对象是 `UNRESOLVED` 成功回执（与 HTTP RESOLVE 对齐，不是 `read` 的错误码） | 进阶 | 成立 |
| `relations` | `knowledge-published` | 这一份知识直接连着谁 | 过 | 过：`search`（匹配）、`read`（正文）、`provenance` | 过：hits 邻居身份；无投影 → `CAPABILITY_UNSATISFIED`。help「列出对象的一跳邻居身份，不是检索信封」 | 过：`--direction DIRECTED\|UNDIRECTED` | 过：不扫描权威 | 进阶 | 成立 |
| `provenance` | `knowledge-published` | 这一份知识声称从哪来 | 过 | 过：`log`（修订）、`read`（正文） | 过：`{objectId, repository, …}`。help「读取对象来源信封」 | 过 | 过 | 进阶 | 成立 |
| `log` | `knowledge-published` | 这一份知识经历过哪些版本 | 过 | 过：`catalog audit`（登记表史）、`git log`（路径） | 过：`{logs, continuation?}`。help「分页读取对象修订历史」 | 过 | 过：不要 `--aspect` | 进阶 | 成立 |
| `binding show` | `repository-attached` / `access-missing-runtime-unavailable.feature` 的 Binding 观测 | 这个字段声明怎么连墙外 | 过 | 过：`read`（句柄正文）、`access`（观察值） | 过：Binding 声明。help「查看 Binding 声明」 | 过 | 过：只看声明 | 进阶 | 成立 |
| `access` | `repository-attached` 的访问边界 probe 与资源解析 Go 证据；根节点的参数拒绝 probe | 读墙外此刻的观察值 | 过 | 过：`binding show`、`read`、`invoke` | 过：Schema 无 origin → `CAPABILITY_UNSATISFIED`；成功 `{objectId, aspectName, schemaRef, value, basis}`。help「按 Schema origin 与实体 ID 读取该 Aspect 的当前观察」 | 过 | 过：`--operation` 拒，指向 `invoke` | 进阶 | 成立 |
| `invoke` | `repository-attached` / `invoke-missing-capability-denied.feature` | 对墙外执行一次已声明的操作 | 过 | 过：`access` | 过：描述无 origin → `CAPABILITY_UNSATISFIED`。help「调用 ResourceDescriptor 操作」 | 过 | 过：仓读权不能替代源系统强制 | 进阶 | 成立 |

`read` 的刀：对手是 `kc search`（只定位）、`kc log` / `kc provenance`、Linux `cat`（路径不是 Address）。`writer head` 回答发布坐标。若已 `kcfs` 挂上，`cat` 能读文件树；`kc read` 仍按对象身份取 Canonical。远程只说明有 Client。

`search` 编码后 hits 是身份名单。投递链只由 HTTP SEARCH 证明。`schema describe` 编码后不再像 list；`resolve` 不打开正文；`relations` 不打印 `searchView`。

---

## 4. 写仓 — write

场景：`domain-schema-published`；普通知识 `knowledge-published` 与语义实例 `semantic-knowledge-published`；写权隔离在 `repository-attached` 上的 `probe-writer-grant-isolates-principals.feature`。

| 命令 | 场景 | 一句语义 | 问 | 刀（对手） | 对 | 形 | 侧 | 路 | 成立 |
|---|---|---|---|---|---|---|---|---|---|
| `writer commit` | `domain-schema-published` | 把期望正文发布成仓的新版本 | 过 | 过：`writer put`、`kc diff`（只看不写） | 过：`{disposition:APPLIED, result.repositoryId, result.newCommit}`。help「把目录里的最终样子发布成仓的新版本」 | 过 | 过：`--command-id` 由用户提供 | 日常 | 成立 |
| `diff` | `domain-schema-published` | 相对当前发布版本，这个目录会改哪些对象 | 过 | 过：`git diff`（路径/工作树）、`writer commit`（写仓）、`writer head`（只给 commit） | 过：`{repository, commit, changes:[{objectId, change}]}`；已对齐时 `changes: []`。help「对照当前发布版本，列出这个目录会改哪些对象」 | 过 | 过：不写仓；`writer.preview` 足够；无 `--command-id` | 进阶 | 成立 |
| `writer put` | `knowledge-published` | 直接发布一个 Address | 过 | 过：`writer commit`（草稿目录） | 过：CommitReceipt；随后 `read` 见正文。help「直接发布一个 Address，不经过草稿目录」 | 过 | 过 | 进阶 | 成立 |
| `writer remove` | `knowledge-published` probe | 删掉一个知识单元 | 过 | 过：`detach`、`writer put` 覆盖 | 过：remove 后 `read` 该对象失败、邻居仍在。help「移除一个 Address」 | 过 | 过 | 进阶 | 成立 |
| `writer head` | `knowledge-published` | 这仓当前发布在哪一版 | 过 | 过：`read`、`deployment status`、`git rev-parse` | 过：`{repository, commit}`。help「查看 Repository 当前发布版本」 | 过 | 过：`detach` 后 HEAD 仍在 | 进阶 | 成立 |
| `writer receipt` | `domain-schema-published` | 那次写命令做成了没有 | 过 | 过：`writer head`、`read` | 过：`{commandId, digest}`。help「查询 Writer 命令回执」 | 过 | 过 | 进阶 | 成立 |

最短旅程不靠 `writer put`。进阶成立不能把它塞进 `help write`。

---

## 5. 多仓任务 — compose

场景：`knowledge-set-defined` 上分别验证退役、解析授权、overlay 和固定版本；`dataset-query-principals-granted` 供同一消费者的发现、检索与读取用例复用。文件计划由 `knowledge-set-defined` 上的独立 Go 证据证明。

| 命令 | 场景 | 一句语义 | 问 | 刀（对手） | 对 | 形 | 侧 | 路 | 成立 |
|---|---|---|---|---|---|---|---|---|---|
| `dataset define` | `knowledge-set-defined` | 给多仓组合起一个可复用的名 | 过 | 过：每次 `--repo`、`attach` | 过：`{setId, revision}`，`show` 见 datasets。help「定义可复用的多 Repository 配方（目录映射与逐文件交付）」 | 过：位置参数与 `--dataset` 相同 | 过 | 进阶 | 成立 |
| `dataset retire` | `knowledge-set-defined` / `dataset-retire-prevents-read.feature` | 退役这个配方 | 过 | 过：`detach`、删仓 | 过：`{dataset, retired:true}`，`show` 仍见 id 且 `retired:true`。help「退役命名配方」 | 过 | 过：不删成员仓 | 进阶 | 成立 |
| `dataset overlay` | `knowledge-set-defined` / `dataset-overlay-preserves-published-recipe.feature` 与 overlay Go 证据 | 只在本机合成一份不发表的配方 | 过 | 过：`dataset define`（发表） | 过：`{setId, sources[]}`；`show` 的共享 Dataset 不变；用 overlay 名去 `read --dataset` → `KNOWLEDGE_SET_INVALID`。缺操作数 → `USAGE_INVALID`。help「只在本机合成一份不发表的配方」 | 过：`--file` 底稿 + `--overlay` 补丁 | 过：不改共享定义 | 进阶 | 成立 |
| `dataset clone` | `dataset-defined` / `TestDatasetCloneJourney` 与 `TestDatasetCloneMaterializesDeliveredTree` | 把已发布 Dataset 的交付目录树物化成普通本地目录 | 过 | 过：File Gateway 逐文件拉取自建目录、`kcfs`（FUSE） | 过：`{catalog, dataset, dir, files, bytes, mounts, fileEntries, pinId, ref, revision}`；非空目标 → `PRECONDITION_FAILED`，不覆盖任何文件。产品 argv 不收 `--pin`，clone 只随当前服务版演进。help「把已发布 Dataset 的交付目录树物化成普通本地目录」 | 过：`<dataset> <dir>` | 过：只写本地目标目录 | 进阶 | 成立 |

---

## 6. 合入治理

场景：`proposal-opened` → `proposal-preview-created`。Preview 直接使用祖先发布的 scene-notes v1；发布 v2 是 `dataset-defined` 上 `probe-publish-next-dataset-revision.feature` 的独立用例。Preview 供两个独立 probe 复用：结构检查保持 main 不变；记录外部 PASSED 报告后合并 candidate 并读回。后一个用例自己产生报告，不继承前一个用例的结果。进阶。

| 命令 | 场景 | 一句语义 | 问 | 刀（对手） | 对 | 形 | 侧 | 路 | 成立 |
|---|---|---|---|---|---|---|---|---|---|
| `governance proposal create` | `proposal-opened` | 把这次变更写到 candidate，main 不动 | 过 | 过：`writer commit`（直接上 main） | 过：`{proposalId, candidateCommit}`；`read --repo` 仍旧值，`--ref` candidate 才是新值。无变更 → `USAGE_INVALID`。help「把变更写到 candidate 并打开 Proposal，不推进 target」 | 过：`--object/--value` 就是这次变更 | 过：main 不前进 | 进阶 | 成立 |
| `governance preview create` | `proposal-preview-created` construct | 把 proposal 叠到本次解析的 Dataset 上，得到可校验的 Preview 坐标 | 过 | 过：`dataset define` | 过：`{previewId, candidate.commitId, candidate.repositoryId}`；随后 `read --repo` 仍旧值。help「给出 candidate 坐标，不改 main」 | 过：必须 `--dataset` | 过：main 仍不动；正文走 `read --commit` | 进阶 | 成立 |
| `governance preview validate` | `proposal-preview-created` / `probe-structure-validation-keeps-main.feature` | 这份 Preview 协议结构过不过 | 过 | 过：`validation record`（外部套件）、业务测试 | 过：`{reportId, outcome:PASSED}`。help「校验 Preview 的协议结构」 | 过 | 过：不跑业务套件 | 进阶 | 成立 |
| `governance validation record` | `proposal-preview-created` / `probe-record-passed-validation-merges-candidate.feature` | 把外部套件已给出的结果绑上去 | 过 | 过：`preview validate`（KC 自己跑） | 过：`{reportId, outcome:PASSED}`。help「记录外部套件结果」 | 过 | 过：不执行检查 | 进阶 | 成立 |
| `governance proposal merge` | `proposal-preview-created` / `probe-record-passed-validation-merges-candidate.feature` | 清单齐则快进仓 Ref | 过 | 过：`writer commit`、`git merge` | 过：`{proposalId, commitId}`；随后 `read` 见 proposed。help「合入 Proposal」 | 过 | 过：同一用例记录外部报告后 merge | 进阶 | 成立 |

---

## 7. 投影、出站、墙外观察

场景：`projection-synced` 的构建同步投影；动态观察刷新是 `repository-attached` 上的具名 Go 证据；hooks 与 gates 都是 `repository-attached` 上的独立 probe。

| 命令 | 场景 | 一句语义 | 问 | 刀（对手） | 对 | 形 | 侧 | 路 | 成立 |
|---|---|---|---|---|---|---|---|---|---|
| `operations projection sync` | `projection-synced` | 让检索投影追上 published HEAD | 过 | 过：`projection notice`（墙外观察）、`search`（消费） | 过：`{repository, basisCommit, objectCount}`。help「同步历史或排障投影」 | 过 | 过：不是 Binding 动态车道 | 进阶 | 成立 |
| `operations projection describe` | `projection-synced` | 投影现在落后 HEAD 吗 | 过 | 过：`sync`、`writer head` | 过：`{basisRepository, basisCommit, objectCount, lagBehindHead}`。help「查看投影状态」 | 过 | 过 | 进阶 | 成立 |
| `operations projection notice` | `repository-attached` 的动态观察刷新 Go 证据 | 告诉投影墙外观察变了 | 过 | 过：`sync`（索引追 HEAD）、`writer head`（仓版本） | 过：`{repository, basisCommit, revision}`；随后 HEAD commit 仍在。缺 `--repo` → `USAGE_INVALID`。help「通知 Bound State 观察变化，不推进仓 HEAD」 | 过：句柄用 `--object`/`--aspect` 写在用法第二行 | 过：HEAD 不变 | 进阶 | 成立 |
| `operations access-spec describe` | `knowledge-set-defined` | 这个 Dataset 上各仓声明怎么连墙外 | 过 | 过：`schema describe`（检索字段）、`binding show`（单个声明） | 过：`{setId, specs}`。help「查看 AccessSpec」 | 过：`--repo` 或 `--dataset` | 过 | 进阶 | 成立 |
| `operations hook add` | `repository-attached` | 登记一个出站 hook | 过 | 过：`gate add`（合入证据）、改配置文件 | 过：`{id, on, phase}`。help「登记 Hook」 | 过 | 过：不发权、不改 Snapshot | 进阶 | 成立 |
| `operations hook list` | 同上 | 现在有哪些 hook | 过 | 过：`gate list` | 过：`{bindings}`（空为 `[]`）。help「列出 Hook」 | 过 | 过 | 进阶 | 成立 |
| `operations hook remove` | 同上 | 去掉这个 hook | 过 | 过：`grant remove` | 过：`{revoked}`。help「移除 Hook」 | 过 | 过 | 进阶 | 成立 |
| `operations gate add` | `repository-attached` / `probe-gates.feature` | 合入必须备齐哪些证据 | 过 | 过：`hook add`、`preview validate` | 过：`{id, on:merge}`。help「登记 Gate」 | 过 | 过 | 进阶 | 成立 |
| `operations gate list` | `repository-attached` / `probe-gates.feature` | 现在有哪些 gate | 过 | 过：`hook list`（`bindings` vs `rules`） | 过：`{rules}`。help「列出 Gate」 | 过 | 过 | 进阶 | 成立 |
| `operations gate remove` | `repository-attached` / `probe-gates.feature` | 去掉这个 gate | 过 | 过：`hook remove` | 过：`{revoked}`。help「移除 Gate」 | 过 | 过 | 进阶 | 成立 |

`projection notice` 回答「观察变了、仓版本没动」。

---

## 8. 访问账与反馈

场景：`catalog-initialized` 的 access/hitmap 来源 probe；`http-served` 的访问账与 trace Go 证据；`catalog-initialized` 的反馈输入 probe；`knowledge-search-granted` 的重排、反馈 Go 证据。

| 命令 | 场景 | 一句语义 | 问 | 刀（对手） | 对 | 形 | 侧 | 路 | 成立 |
|---|---|---|---|---|---|---|---|---|---|
| `operations audit access` | `catalog-initialized` / `probe-access-log.feature`；`http-served` 的审计 Go 证据 | 最近发生了哪些访问 | 过 | 过：`catalog audit`（登记表）、`log`（对象史） | 过：`{source:access, entries?}`；空窗不是错误。help「查看访问账」 | 过 | 过 | 进阶 | 成立 |
| `operations audit hitmap` | `catalog-initialized` / `probe-access-log.feature`；`http-served` 的审计 Go 证据 | 检索命中了哪些对象 | 过 | 过：`audit access`（事件 vs 对象） | 过：`{source:hitmap, hits}`。help「按对象汇总检索命中，不是访问事件账」 | 过 | 过 | 进阶 | 成立 |
| `operations audit trace` | `http-served` 的 trace Go 证据；根节点缺操作数 probe | 这一次请求的证据链 | 过 | 过：`audit access`（分页账）、`hitmap` | 过：缺 `--trace-id` → `USAGE_INVALID`；成功 `{traceId, entries[]}`（kind=access/feedback/refine/retrieval）。help「查看请求追踪」 | 过 | 过 | 进阶 | 成立 |
| `operations feedback record` | `catalog-initialized` 的非法 outcome probe；`knowledge-search-granted` 的反馈 Go 证据 | 给这次检索记一条人的对错 | 过 | 过：`preview validate` 的 PASSED/FAILED | 过：非法 outcome → `USAGE_INVALID`；成功 `{traceId, outcome, recorded:true, …}`。help「记录检索反馈」 | 过 | 过：晚于请求的独立调用 | 进阶 | 成立 |

---

## 9. Catalog 进阶与部署

场景：`catalog-initialized` 上的审计发权 probe、部署恢复与 System 发布 Go 证据；`repository-attached` 的归档 probe；`http-served` 提供身份用例复用的测试 Server。

| 命令 | 场景 | 一句语义 | 问 | 刀（对手） | 对 | 形 | 侧 | 路 | 成立 |
|---|---|---|---|---|---|---|---|---|---|
| `catalog audit` | `catalog-initialized` 的审计读取与审计发权 probe | 这间 Catalog 的登记表怎么变过来的 | 过 | 过：`log`（对象修订）、`audit access` | 过：`{source:catalog, catalogId, entries}`。无 `catalog.audit.read` → 拒绝。help「查看 Catalog 登记表历史」 | 过 | 过：不放行库存发现 | 进阶 | 成立 |
| `catalog archive` | `repository-attached` / `probe-archive-rejects-dataset-definition.feature` | 结束这间 Catalog 的生命周期 | 过 | 过：`detach`（一个仓）、删部署 | 过：`{catalog, archived:true}`，随后 `show.archived=true`。无管理权 → 拒绝。help「归档当前 Catalog」 | 过 | 过 | 进阶 | 成立 |
| `deployment init` | `catalog-initialized` 的初始化 Go 证据与缺配置 probe | 按配置文件把部署建起来 | 过 | 过：手建目录、`serve` | 过：成功 `{initialized:true, catalogs}`；无 `--config` → `USAGE_INVALID`。help「初始化部署」 | 过 | 过：不猜本机 Home | 部署 | 成立 |
| `deployment status` | `catalog-initialized` 的恢复 Go 证据与缺配置 probe | 既有部署恢复好了没有 | 过 | 过：`show`、`writer head` | 过：`{status:ready, catalogs:[{id,head,archived,repositories}]}`；无 `--config` → `USAGE_INVALID`。help「查看部署恢复状态」 | 过 | 过：缺耐久状态失败关闭 | 部署 | 成立 |
| `deployment system publish` | `catalog-initialized` 的 System 发布 Go 证据与缺配置 probe | 把 System Schema 写入绑定仓 | 过 | 过：`writer put` 到 `kr://kc/system`（禁止） | 过：`{repositoryId, commit, metaSchema, metaSchemaDigest, seeded}`；无 `--config` → `USAGE_INVALID`。help「把 System Schema 写入绑定仓」 | 过 | 过：不登记业务 Snapshot | 部署 | 成立 |
| `deployment identity migrate` | `catalog-initialized` 的身份迁移 Go 证据 | 把历史身份绑定迁到核验过的主体 | 过 | 过：`grant add` 重发、改用户名 | 过：`{status:APPLIED\|REPLAYED, principal, migratedRules}`。help「迁移历史身份绑定」 | 过 | 过：重放不重建已撤销规则 | 部署 | 成立 |
| `serve` | `http-served` | 启动 typed API | 过 | 过：ttyd（人敲 `kc`）、把 `/` 当页面 | 过：help「启动 typed API Server；浏览器打开根路径会 404」。无配置 → `USAGE_INVALID`。成功是进程在听，不是 JSON 页 | 过 | 过：不是 CLI 入口 | 部署 | 成立 |

`serve` 的刀是「给 Client 一个 typed API」，不是给人一个终端。走查入口仍是 ttyd。

---

## 10. 文件视图 — `kcfs`

场景定位：`knowledge-set-defined` 上的固定 pin、Gateway 和 kcfs 命令边界 Go 证据；这些测试独立准备环境，不产生可被 scene 继承的挂载状态。真实 Linux FUSE 由专门套件验收。不在 `cliSurface`；`TestSceneCatalogCoversPublicProductSurfaces` 另钉。含逐文件条目的 Dataset 不能被 kcfs 投影：投影会静默缩小交付树，所以直接失败关闭；FUSE 只服务纯目录映射的交付。

| 命令 | 场景 | 一句语义 | 问 | 刀（对手） | 对 | 形 | 侧 | 路 | 成立 |
|---|---|---|---|---|---|---|---|---|---|
| `kcfs plan` | `knowledge-set-defined` 的固定 pin 文件计划 Go 证据 | 这份 Dataset 会挂成哪些只读路径 | 过 | 过：`dataset define`（坐标）、`kcfs mount`（FUSE）、扫描仓伪造工作树 | 过：打印 mount manifest。help 第一句「kcfs mounts a Knowledge Catalog Dataset」 | 过：必须 `--dataset` + `--root` | 过 | 进阶 | 成立 |
| `kcfs mount` | `knowledge-set-defined` 的 kcfs 命令边界 Go 证据；真实 Linux FUSE 另验收 | 把这份 Dataset 挂进现有项目当只读目录 | 过 | 过：`plan`、Linux `mount`、checkout | 过：同一份 manifest，然后服务到信号。Linux FUSE 才是真实挂载 | 过 | 过：无 `--dataset` 不挂；知识仓无 mount 不得扫描 | 进阶 | 成立 |

---

## 闭集核对

`cliSurface` 58 + `serve` + `kcfs plan` + `kcfs mount` = 61。每条恰好一行。

| 命令 | 场景锚点 | 成立 |
|---|---|---|
| `login` | `http-served` | 是 |
| `logout` | `http-served` | 是 |
| `whoami` | `catalog-initialized` | 是 |
| `admission show` | `http-served` / `grants-bootstrapped` | 是 |
| `catalog list` | `catalog-read-granted` | 是 |
| `catalog use` | `catalog-read-granted` | 是 |
| `show` | `catalog-initialized` | 是 |
| `create` | `catalog-initialized` 的平台建仓与连接 Go 证据 | 是（两种模式） |
| `attach` | `repository-attached` | 是 |
| `detach` | `repository-attached` / `probe-detach-keeps-head-readable.feature` | 是 |
| `grant add` | 复用授权前态的 construct；一次性发权在各独立 probe | 是 |
| `grant list` | `catalog-initialized` | 是 |
| `grant remove` | `catalog-initialized` / `probe-revoke-catalog-read-denied.feature` | 是 |
| `schema list` | `catalog-initialized` 的 System 浏览用例与 Go 分页证据；`schema-read-granted` | 是 |
| `schema describe` | `schema-read-granted` | 是 |
| `search` | `knowledge-search-granted` | 是 |
| `read` | `knowledge-search-granted` / `read-grant-returns-canonical.feature`；`knowledge-published` | 是 |
| `resolve` | `knowledge-published`；`dataset-query-principals-granted` 的消费用例 | 是 |
| `relations` | `knowledge-published` | 是 |
| `provenance` | `knowledge-published` | 是 |
| `log` | `knowledge-published` | 是 |
| `binding show` | `repository-attached` / `access-missing-runtime-unavailable.feature` 的 Binding 观测 | 是 |
| `access` | `repository-attached` 的访问边界 probe 与资源解析 Go 证据；根节点的参数拒绝 probe | 是 |
| `invoke` | `repository-attached` / `invoke-missing-capability-denied.feature` | 是 |
| `writer commit` | `domain-schema-published` | 是 |
| `diff` | `domain-schema-published` | 是 |
| `writer put` | `knowledge-published` | 是 |
| `writer remove` | `knowledge-published` | 是 |
| `writer head` | `knowledge-published` | 是 |
| `writer receipt` | `domain-schema-published` | 是 |
| `dataset define` | `knowledge-set-defined` | 是 |
| `dataset retire` | `knowledge-set-defined` / `dataset-retire-prevents-read.feature` | 是 |
| `dataset overlay` | `knowledge-set-defined` / `dataset-overlay-preserves-published-recipe.feature` 与 overlay Go 证据 | 是 |
| `dataset clone` | `dataset-defined` / `TestDatasetCloneJourney` 与 `TestDatasetCloneMaterializesDeliveredTree` | 是 |
| `governance proposal create` | `proposal-opened` | 是 |
| `governance preview create` | `proposal-preview-created` construct | 是 |
| `governance preview validate` | `proposal-preview-created` / `probe-structure-validation-keeps-main.feature` | 是 |
| `governance validation record` | `proposal-preview-created` / `probe-record-passed-validation-merges-candidate.feature` | 是 |
| `governance proposal merge` | `proposal-preview-created` / `probe-record-passed-validation-merges-candidate.feature` | 是 |
| `operations projection sync` | `projection-synced` | 是 |
| `operations projection describe` | `projection-synced` | 是 |
| `operations projection notice` | `repository-attached` 的动态观察刷新 Go 证据 | 是 |
| `operations access-spec describe` | `knowledge-set-defined` | 是 |
| `operations hook add` | `repository-attached` | 是 |
| `operations hook list` | `repository-attached` | 是 |
| `operations hook remove` | `repository-attached` | 是 |
| `operations gate add` | `repository-attached` / `probe-gates.feature` | 是 |
| `operations gate list` | `repository-attached` / `probe-gates.feature` | 是 |
| `operations gate remove` | `repository-attached` / `probe-gates.feature` | 是 |
| `operations audit access` | `catalog-initialized` / `probe-access-log.feature`；`http-served` 的审计 Go 证据 | 是 |
| `operations audit hitmap` | `catalog-initialized` / `probe-access-log.feature`；`http-served` 的审计 Go 证据 | 是 |
| `operations audit trace` | `http-served` 的 trace Go 证据；根节点缺操作数 probe | 是 |
| `operations feedback record` | `catalog-initialized` 的非法 outcome probe；`knowledge-search-granted` 的反馈 Go 证据 | 是 |
| `catalog audit` | `catalog-initialized` 的审计读取与审计发权 probe | 是 |
| `catalog archive` | `repository-attached` / `probe-archive-rejects-dataset-definition.feature` | 是 |
| `deployment init` | `catalog-initialized` | 是 |
| `deployment status` | `catalog-initialized` 的恢复 Go 证据与缺配置 probe | 是 |
| `deployment system publish` | `catalog-initialized` 的 System 发布 Go 证据与缺配置 probe | 是 |
| `deployment identity migrate` | `catalog-initialized` 的身份迁移 Go 证据 | 是 |
| `serve` | `http-served` | 是 |
| `kcfs plan` | `knowledge-set-defined` 的固定 pin 文件计划 Go 证据 | 是 |
| `kcfs mount` | `knowledge-set-defined` 的 kcfs 命令边界 Go 证据；真实 Linux FUSE 另验收 | 是 |

最短旅程（`help consume|write|compose`）里问刀对不过的，编码前是 `read` / `search` / `grant list`，编码后这三条成立。旅程内没有新的日常误放。进阶六条本轮收口后也成立。

形、侧无剩余诊断项。`resolve` 缺对象用 `UNRESOLVED` 成功回执是 CLI.md 选定，与 HTTP 同一代数。`governance proposal create` 的 `--object/--value` 就是写到 candidate 的那次变更，不是顺带的第二问。

---

## 缺的命令 / 只该留在 HTTP

缺的（不是本轮要加的命令）：按实体浏览实例目录（BROWSE ≠ object LIST ≠ 空 SEARCH）；把一次维护收成「申请单」在 KC 内审批（已否决）。

只该留在 HTTP：FaultJSON 全量、SEARCH 投递链与 `KnowledgeHit`、集合 GET 库存、AllowFile 原文。产品 CLI 变短不删除这些路由。

`catalog-initialized` 的退役入口 probe 与具名 Go 反例已钉：MCP、对象 LIST、checkout/export、connector-run、APPEND/Stream 正路径都不存在。

---

## 反事实 `kcfs`

挂上之后：路径可读的用 `cat` / `rg` / `git log`。`kc read` / `kc search` / `kc schema list` 仍然按 Address 与 Schema AccessHints 工作。不能因为远程 Client 存在，就把协议信封留给人看。`kcfs` 自己的刀是 Dataset 挂载时内部冻结的只读文件树，不是把 Canonical 单元信封摊成默认界面。
