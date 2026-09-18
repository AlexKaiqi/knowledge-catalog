# 产品 CLI

日期：2026-09-16

本文解释产品 CLI 为什么按人的任务组织 argv 和 help，而不是按 HTTP 资源树或协议平面抄一份命令。公开路径闭集、每条命令的操作语义、typed HTTP 路由和授权动作名不在本文复制。

落地时的迁移对照见 [`cli/REFACTOR.md`](../cli/REFACTOR.md)；那份记录不是本文的替代，也不进入文档图。

---

## Goal

给真人一条能完成四条路径的命令面：进入一间 Catalog、读一份知识、往一个仓写、把源挂上并给人权。命令名说「做什么」；`--repo` / `--dataset` / `--object` / `--command-id` 只指出这次动作碰到的对象。回执带 `commit`，消费者不管理 pin 文件。Client 已经知道的 Server、身份和当前 Catalog，不再当日常 flag。

知识面动作在 argv 上不带 `knowledge` 前缀。help 按使用频率披露，不按岗位或协议平面建树。CLI 与 HTTP 独立登记、调用同一组 typed executor（`API-01`）。

## Non-Goals

- 不按岗位建命令树；`help consume|write|compose` 是旅程，不是身份。
- 不把供给、Catalog 登记、知识发布、发权焊成一次隐式初始化。
- 不把当前 Catalog 写进 login token，不做成知识集 pin，Server 不猜默认 Catalog。
- 不为 CLI 变短删除 HTTP 集合路由，也不把 HTTP 集合 GET 镜像成 CLI。
- 不在 KC 里托管申请/审批队列。
- 不做旧 argv 兼容别名。
- 不把 README 扩成领域分类、owner、质量线或投影热度，也不把 README 展成 Catalog title/summary。
- 不取消 Client 内部保存入口，不取消凭证按 Server 隔离，也不把入口地址塞进 login token。

## 硬性约束 / Invariants

- `API-01`：CLI 与 HTTP 调同一应用 executor，但 transport 注册相互独立。禁止 HTTP 调 CLI parser，禁止两个入口实现不同业务规则。
- 知识面协议动作仍是 `knowledge.*`，HTTP 仍在 `/knowledge/v1`。argv 去掉 `knowledge` 前缀，不把协议平面改名，也不把 HTTP 路径抄进命令。
- 入口、身份、当前 Catalog 是 Client 上下文。单仓动作用 `--repo`，已定义 Dataset 用 `--dataset`。知识命令拒绝 `--catalog` / `--source` / `--pin`。Writer、`diff` 与 `schema list` 拒绝 `--dataset`。
- 单仓消费不要求先解析 Dataset。`schema list` 只接受 `--repo`。它只回答「这个仓有哪些实体」；合同正文走 READ，检索字段走 `schema describe`。
- 一条命令内 resolve 一次（`V-01`）。跨命令精确重放抄回执里的 `--repo --commit`。Dataset 发布冻 commit（`KS-01`）；pin 不冻结未来权限（`KS-02`）。产品 argv 不出现 `kc pin` / `--pin`。
- `access` 与 `invoke` 分开：能看见受保护操作的说明，不等于能在外部系统执行它。
- 退役 argv 必须失败关闭，不留别名分支。
- 产品 CLI 的 `USAGE_INVALID` 必须写出失败原因和该叶子用法。每条公开命令都有叶子用法。HTTP 仍是 FaultJSON。成功回执仍是 JSON。
- 一条产品命令是否成立，只认问、刀、对同时成立。形、侧、路是诊断，不能把前三维不过的命令判成成立。刀必须写出对手（其它 `kc`、Linux/`git`/`rg`、HTTP GET）；远程不是刀。

## 选定方案 / 被否决方案

- 选定：四条真人路径决定根 help 出现什么；resolve、治理、运维、`catalog audit` / `archive` 进分组，不进最短旅程。
- 选定：`catalog use` 保存当前 Catalog；只有一间可见时 `catalog list` 自动选择。日常知识/写命令不再收 Catalog 操作数。
- 选定：`create` 只供给或连接 Repository；`attach` 只把已连接仓登记进当前 Catalog；`grant add` 才发权。三者不互相隐含。
- 选定：从 Catalog 拿下源用 `detach --repo`，不把「归档历史」听成拿下成员。
- 选定：知识面日常动词扁平为 `search` / `read` / `schema list` 等；内部 handler 仍是 `knowledge-search` 这类操作名。
- 选定：`schema list` 只说明有哪些实体（`entity`、可选 `description`、打开合同用的 `objectId`）。② 返回 Schema 正文的是 READ；DESCRIBE_SCHEMA 是检索编译，不是 list。
- 选定：全局唯一的短动词保持扁平（`show` / `create` / `attach`）；同族多条命令保留家族前缀（`writer`、`grant`、`operations`、`catalog …`）。
- 选定：help 三层——根上分组与最短旅程，组内列命令，叶子才给 flag。未写完的家族前缀（`kc grant`）输出该组索引并失败关闭，不把前缀登记成命令。
- 选定：叶子 help 三段——这条命令干什么、argv 骨架、值有写法时再加怎么写和能抄的例子。复杂操作数闭集由 `complexLeafOperands` 拥有。`--query` / `--value` / `--action` 含空格或 `*` 必须加单引号；`*` 不是 SEARCH 浏览，也不能当 `grant add` 的动作。
- 选定：产品 CLI 的 `USAGE_INVALID` 短路打出原因和同一份叶子 help；`kc grant` 这类未写完前缀已经是组索引。FORBIDDEN 等协议失败仍是 FaultJSON，不假装成缺 flag。
- 选定：用六维评价产品命令（问、刀、对、形、侧、路）。成立 = 问 ∧ 刀 ∧ 对。最短旅程先于进阶；进阶成立不能稀释日常不成立。程序见本文 §6，判定表见 [`CLI_EVALUATION.md`](CLI_EVALUATION.md)。
- 选定：产品 CLI 的 `read` 回答「这一份知识在固定 commit 上的正文」：`repository`、`objectId`、`commit`、`value`，有 Aspect 时加 `aspectName`，有 Schema 合同时加 `schemaRef`。`--dataset` 为同一形状的数组。不是 `KnowledgeValue` 拼装记录。
- 选定：产品 CLI 的 `search` 回答「哪些对象匹配」：`hits` 里每条是 `{repository, objectId, commit}`，不嵌套 `KnowledgeValue` / `body`。正文走 `read`。HTTP SEARCH 仍是 `KnowledgeHit` 与投递链。
- 选定：产品 CLI 的 `schema describe` 回答「哪些字段能搜、能滤」：`repository`、`commit`、`schemas[].objectId`、`schemas[].fields`（`path` / `type` / `access`）。有 Bound State 时加 `schemas[].origin`。不是实体目录，也不是合同正文。
- 选定：产品 CLI 的 `access` 回答「这个实体这个 Aspect 此刻的墙外观察」：`objectId`、`aspectName`、`schemaRef`、`value`、`basis`。协议是 Schema frontmatter `origin` + 实体 ID。不是 `{bindings, observations}`，也不另存空 Aspect 文件。
- 选定：产品 CLI 的 `resolve` 回答「这一份在不在、钉在哪一版」：`repository`、`objectId`、`commit`、`status`，有 Aspect 时加 `aspectName`，有 member 时加 `memberKey`。不打开正文，不打印 Address 信封。
- 选定：产品 CLI 的 `relations` 回答「直接连着谁」：`hits[]` 为 `{repository, objectId, commit, relationType, matchedRoles}`。不打印 `searchView` 或关系信封。HTTP RELATIONS 仍是 `RelationPage`。
- 选定：产品 CLI 的 `operations audit hitmap` 回答「哪些对象被命中」：`{source:hitmap, hits}`。访问账仍是 `{source:access, entries}`。
- 选定：`writer put` 的刀是「直接发布一个 Address，不经过草稿目录」；最短旅程是 `writer commit --dir`。
- 选定：目录写入是 `writer commit --command-id --repo --dir`。对照当前版本求差是 commit 的内部动作；`kc diff --repo --dir` 用同一对照，只列出会改的对象，不写仓。HTTP Writer 仍收 ChangeSet。
- 选定：产品 CLI 的 `diff` 回答「相对当前发布版本，这个目录会改哪些对象」：`repository`、`commit`、`changes[]` 为 `{objectId, change}`，有 Aspect 时加 `aspectName`。`change` 是 `add` / `update` / `remove`。不打印 ChangeSet，不打开正文。空 diff 是 `changes: []` 的成功回执。
- 选定：`governance proposal create` 把一次变更写到 `--candidate` 并打开 Proposal，不推进 `--target`（默认 published）。变更用 `--object`+`--value`/`--file`，或 `--changeset`。空 Proposal（没有变更）失败关闭。
- 选定：产品 CLI 的 `resolve` 缺对象是 `status=UNRESOLVED` 的成功回执（单仓）或空名单（Dataset），不是 `read` 的 `KNOWLEDGE_REF_UNRESOLVED`。与 HTTP RESOLVE 同一失败代数。
- 否决：`kc repo …` 分组；「每次传 catalog」与 `use` 两套并存；`create` 顺带登记；`create --url` 当成 attach。
- 否决：attach 要求 published HEAD / 非空仓；attach 或 create 隐含 `knowledge.read`；产品 `create` 要求 `--command-id`；Client 替 Writer 默默生成 `--command-id`。
- 否决：`workspace use`；单源消费先造 pin 文件；Catalog 当搜索范围；知识命令继续拒绝 `--dataset`；Writer 收 `--dataset` / `--source`；`schema list --dataset` / `--pin`。
- 否决：产品 CLI 保留 `kc pin` / `kc pin check` / argv `--pin` 作为消费者操作数；把知识集讲成「用户管理 commit」。
- 否决：`schema list` 返回 Schema 正文、YAML 文件或完整 `SchemaDescription`。
- 否决：把 argv 写成带 `knowledge` 前缀的 search 只为对齐 HTTP `/knowledge/v1`；把 HTTP 集合 GET 镜像成 CLI；兼容别名恢复旧 argv。
- 否决：把「这是远程的」当成刀；用形、侧、路的分数把问、刀、对不过的命令判成成立；把进阶命令的成立当成最短旅程已过关。
- 否决：把 HTTP / 协议 DTO（`KnowledgeValue`、`KnowledgeHit`、整份 AllowFile、`access` 的 `{bindings, observations}`）当作产品 CLI 的打印形状。`API-01`：编码只发生在 CLI 打印路径，不改 HTTP、不改 executor。
- 否决：产品 CLI 的 `USAGE_INVALID` 只吐一份 error JSON；用法失败时倾倒整本根 help。
- 否决：叶子用法只给 flag 骨架、不说明值怎么写；把检索代数或错误码表抄进 help。
- 否决：产品 CLI 另设 `pack` / `status` / `plan`；否决最短旅程必须「先 diff 再提交」。
- 否决：把 ChangeSet 文件交给接入方当日常 CLI 操作数；`pack --out` 写出 changeset。
- 否决：`--server` 写成 `login` 专属参数；登录回执带 `server`；知识集讲成默认 search scope；KC 内建申请队列。

## 接口契约 / 状态机

公开 argv 闭集：[`cli/surface.go`](../cli/surface.go)。产品操作语义：[`cli/SURFACE.md`](../cli/SURFACE.md)。验收分母：`TestProductCLIRefactorDefinesTheExactPublicSurface`、`TestRemovedCommandsAreRejected`。日常消费路径产品 stdout：`TestCLIEvaluationDailyJourneyProductStdout`。进阶产品 stdout：`TestShapeCLIProductDailyCommands`。判定表与缺命令清单：[`CLI_EVALUATION.md`](CLI_EVALUATION.md)。

typed HTTP 与 Client：[`httpsurface/`](../httpsurface/README.md)、[`client/`](../client/README.md)、[`SERVICE_ARCHITECTURE.md`](SERVICE_ARCHITECTURE.md) `API-01`。授权动作名：[`PERMISSIONS.md`](PERMISSIONS.md)。组合与 pin：[`COMPOSITION.md`](COMPOSITION.md)。公开名词：[`TERMINOLOGY.md`](TERMINOLOGY.md)。

产品命令经 Server 与显式 principal。默认 Snapshot ref 是 `snapshot.DefaultRef`。Writer 提交必须由用户提供 `--command-id`。

---

## 1. CLI 不是 HTTP 的 argv 版

HTTP 按资源分层：人看见 `METHOD /{plane}/v1/{resource}` 就能预期副作用落在哪一类对象上。CLI 的读者不是资源树，是要完成一件事的人。把集合 GET 一条条镜像成命令，会让日常路径淹没在库存读取里；把 argv 写成协议平面前缀，会把「读一份知识」说成「进入知识子系统」。

因此两面独立登记、对齐操作与 action，不对齐字符串。CLI 变短不删除 HTTP 集合路由；HTTP 保留资源层级，不要求 CLI 再套一层 `catalog repo …`。

## 2. 四条路径决定根上出现什么

人要做的只有这些：

1. 进入一间 Catalog（看见自己能进哪些房，选当前这一间）。
2. 读一份知识（先知道该仓有哪些实体，再 search / read）。
3. 往一个仓写（显式 command-id，不让 Client 伪造一次维护）。
4. 把源挂上并给人权（供给或连接仓、登记进 Catalog、再 grant）。

能走 HTTP 或进阶分组完成的，根 help 和最短旅程都不出现。`help consume|write|compose` 只是把上述路径说成旅程，不是角色，也不另开一套命令树。

## 3. 上下文与操作数

Server 入口、登录身份、当前 Catalog 是会话里已经成立的事实。每次读知识再问一遍「在哪间房」会把 Catalog 操作数训练成搜索范围，也会让 `catalog use` 失去意义。

本次动作真正要指向的是：

- 单仓：`--repo`
- 已定义 Dataset：`--dataset`
- 知识对象：`--object`
- Writer 提交：`--command-id`

`--catalog` / `--source` / `--pin` 曾把组合配方、Catalog 库存、内部冻结文档和知识来源混进同一条消费命令。命名 Dataset 已经回答「这次读哪几个仓」；命令开始时内部 resolve 一次（`V-01`），回执带 `commit`。知识命令再收 `--pin`，等于让消费者管理一份协议冻结文件。

单仓消费直接 `--repo`。要求先 pin 会把「读一个仓」说成「先开一个组合任务」，也把 Schema 列表误当成任务目录。

## 4. 知识面：argv 扁平，协议仍在知识面

`knowledge.search`、`POST /knowledge/v1/search`、内部 handler `knowledge-search` 回答的是协议平面：这条动作改变或读取的是知识，不是 Catalog 成员表，也不是外部系统。

人敲命令时已经站在「读知识」这条路上。再写带 `knowledge` 前缀的 search 是把平面名当成分组约束，而不是消解碰撞——`search` / `read` / `show` 并不互相抢名字。组合面已经扁平了 `show` / `create` / `attach`；知识面日常动词同样扁平。

help 仍可把这些命令收在 `kc help knowledge` 一组里，方便进阶发现。组名不是 argv 前缀，也不构成旧路径别名。

## 5. 短动词何时扁平

扁平的条件是全局唯一、且名字已经在说这件事：`show` 是当前 Catalog 的成员与知识集，不是对象目录；`create` 是供给或连接仓；`attach` / `detach` 是 Catalog 成员登记。

同族多条、或名字单独拿出来会歧义的，保留家族前缀：`writer *`、`grant *`、`operations *`、`catalog audit`。这不是按 HTTP 集合再嵌一套资源树，而是避免 `list` / `add` / `remove` 在根上互撞。

## 6. 怎样判断一条命令成立

评价一条产品 CLI，按这个顺序，失败关闭。分数不能互相抵消。

1. **问**：写成一句人会问的话。一句里只能有一个问号。写不出、或必须用「并且」才能说完，问不过。
2. **刀**：相对三个对手多出来的那一刀，且必须点名对手——其它 `kc`、Linux/`git`/`rg`、HTTP GET。远程访问解释的是 Client，不是这把刀。写不出对手，刀不过。
3. **对**：成功 stdout、失败 stdout、叶子 help 第一句，三者都在回答那一句。stdout 答另一问（拼装记录、投递链、整份配置文件），对不过。
4. **形**：是否符合本产品 CLI 合同（扁平短动词、操作数、help 三层、`USAGE_INVALID` 短路）。形不过是病，救不了问刀对。
5. **侧**：副作用、授权、失败代数是否跟动词一致。`create` 不得隐含 `knowledge.read`；`search` 不得假装已授权 `read`。
6. **路**：最短旅程还是进阶。日常误放 = 最短旅程里的命令问刀对已经不过。

**成立 = 问 ∧ 刀 ∧ 对。** 形、侧、路是诊断。

程序：

- 证据三人组：成功 stdout、失败 stdout、叶子 help 第一句。没有三人组，对不能判过。
- 先评最短旅程（consume / write / compose），再评进阶。进阶成立不能让日常不成立看起来没那么严重。
- 列「缺的命令」和「只该留在 HTTP 的能力」，避免把缺口说成命令面已经完备。
- 反事实：若已经 `kcfs` 挂上，Linux `cat` / `rg` / `git log` 会不会把这把刀吃掉。会，就不是 `kc` 的刀。
- 校准：`writer head` 回答「发布在哪一版」，远程 `cat` 回答「文件内容」。`read` 的刀是 Address 身份上的 Canonical，不是远程文件。

按场景覆盖全部公开命令（`cliSurface`、`serve`、`kcfs`）的判定表、编码前失败证据和缺命令清单在 [`CLI_EVALUATION.md`](CLI_EVALUATION.md)。
