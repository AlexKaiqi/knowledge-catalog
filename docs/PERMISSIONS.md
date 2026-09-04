# 权限模型：按仓隔离、发现与读分层

日期：2026-09-03
范围：谁能对哪份知识执行哪类 `kc` 动作。公开动作名与默认边界由本文拥有；规则字段一旦选定，由 allow 策略合同描述，本文不重贴。

本文回答：为什么安全边界默认是 Repository，为什么 Workspace 组合不能扩大授权，为什么登记进 Catalog 等于可被发现但不等于可读正文，以及知识仓中的外部授权快照为什么不能替代外部系统实时强制。

---

## Goal

回答谁能对哪份知识执行哪类 `kc` 动作：默认安全边界是 Repository；登记进本 Catalog 的仓对已认证且持有 `catalog.read` 的主体可发现；正文、历史、关系与 VFS 仍要仓级 `knowledge.read`；Workspace 组合不扩大授权；外部授权快照不能替代源系统实时强制。

## Non-Goals

- 不把 Catalog 权限做成文件 ACL，不按 Ranger/Unity 表 GRANT 拆知识仓。
- `permissions` Aspect 不是 `kc knowledge read` 闸门，也不能放行 SELECT。
- 不做 GitHub 式文件 ACL/CODEOWNERS 解释器。
- 不发明与 `kc` 动作平行的授权枚举（下文「默认粒度」）。
- 不复制 allow 规则字段全集（形状由 allow 策略合同拥有，不在本文重贴）。
- 不按 path 授权；不以对象级读 ACL 作为主模型。
- `repo-add` / define-workspace 不隐式发权；不在协议里建角色/组继承树。
- 不为访客或每个 principal 重建检索索引；发现过滤只用已命名的固定元信息，不是第二份投影。
- 不把仓级「访客/成员可见性」或其它业务分类做成第二套过滤键。源说明不承载分类或授权（`KNOWLEDGE_PRODUCT_AND_SCHEMA.md`）。
- 不选定字段级隐私化 / 脱敏声明语言；交付链选定且仅选定首段「无 `knowledge.read` 则屏蔽正文」。后续段在拥有该主题的文档选定并写入 `ARCHITECTURE_INVARIANTS.md` 之前，不得实现。
- 不把 `LIVE_MATERIALIZATION.md` 的 continuation / replay token 当成交付链上的正文裁剪规则。
- 交付链不是出站 Hook，也不在 READ/SEARCH 上挂用户脚本（`HOOKS.md`）。
- 不把 Gitea collaborator / Dolt SQL GRANT 当成 `kc` 动作授权。
- 不把 `onBehalfOf` 与用户权限求交（除非另开 ADR）。
- 不提供匿名访客读。

## 硬性约束 / Invariants

已固化（`ARCHITECTURE_INVARIANTS.md`）：

- `WS-02` 成为 Workspace 成员不获得 READ；旧 pin 不能绕过撤权。
- `C-01` Canonical 只从固定 authority basis 解释；hydrate 义务不因交付链屏蔽正文而取消。本文不重定义 hydrate。
- `AUTH-01` 命名知识集与 `--repo` SEARCH 不按 `knowledge.read` 裁候选；无读权屏蔽正文且不是 `partial`。
- `AUTH-02` `workspace.consume` 不放行 `knowledge.*`；命名知识集 SEARCH 另要 `knowledge.search`。
- `AUTH-03` 交付链输入是已 hydrate 的知识 ID；按序改写可见正文；无读权只清空正文；不得改 ID/Address。

其它已选定、尚未进入不变量索引：

- Catalog 范围 SEARCH 语法糖（`search --catalog` / discovery Workspace）的准入是 `catalog.read`，不另要 consume，也不用按仓 `knowledge.search` 裁候选。禁止观察：该糖因缺少 consume 或某仓 `knowledge.search` 而 `FORBIDDEN` / 省略成员。参考实现尚未提供该表面。
- 调用方看见 Canonical 正文要求仓级 `knowledge.read`。禁止观察：精确 READ 用屏蔽正文的 200 代替 `FORBIDDEN`。
- 授权按 `principal` 求值；`onBehalfOf` 只是审计事实（`OBSERVABILITY.md`）。
- 空 Home 只能用一次性 `kc local grant bootstrap` 建立首个管理主体；业务命令无 owner bypass。

## 选定方案 / 被否决方案

- 选定：按治理边界拆 `--repo`；发权是 `kc admin grant add`；外部 GRANT 快照作为 SOURCE 知识。
- 选定：发现与读分层——一份 AccessHints 索引；查询过滤只用固定元信息（`repository`、`object_id`、`basis`、`schema_ref`）；hydrate 之后走交付链，当前只挂首段仓读权屏蔽。
- 选定：动作按阶段分责（见接口表）。`catalog.read` 覆盖该 Catalog 库存与 Catalog 范围 SEARCH；命名知识集走 `workspace.consume` + `knowledge.search`；正文只认仓级 `knowledge.read`。
- 否决（本文边界）：父级授权自动继承；把知识仓 ACL 做成 Ranger 镜像；按表 GRANT / 单个 Agent / 单个 Workspace 拆仓；按人复制索引；先省略无权仓再假装 Catalog 不可发现；把未命名的仓级可见性或隐私化当成已选定链段；用 `workspace.consume` 或按仓 `knowledge.search` 裁 discovery 候选。Workspace union 当目录优先级见系统设计 [R-05](KNOWLEDGE_CATALOG_DESIGN.md#r-05)。成员仓 clone 后不再声称对象级只读。

## 接口契约 / 状态机

三套不能混合的权限见下文。公开动作名由本文拥有；allow 规则字段由 allow 策略合同拥有，不在本文重贴。Client↔Server 配对由本文与 `SERVICE_ARCHITECTURE.md` 分责。参考实现把 SEARCH 登记成同一内部动词，**求值仍按本表**，不得用当前 `cli/allow.go` 的隐含关系收窄设计。

| 动作 | 典型范围 | 放行 | 不放行 |
|---|---|---|---|
| `catalog.read` | Catalog | 该 Catalog 库存（含源说明标题/摘要信封）；以 discovery Workspace 为 pin 的 Catalog 范围 SEARCH，不另要 `workspace.consume` | 命名知识集 consume；成员正文；`knowledge.schema.read`；VFS 字节 |
| `knowledge.search` | 命名 Workspace 或 Repository | 对该范围调用 SEARCH | 正文；Catalog 库存；按仓裁 discovery 候选 |
| `knowledge.read` | Repository | 交付链放行正文；精确 READ / RESOLVE / LOG / GET_PROVENANCE；该仓进入 VFS plan | 调用 SEARCH；从发现候选抹仓 |
| `knowledge.schema.read` | Repository | schema describe / browse | 实例正文；不被 `catalog.read` 隐含 |
| `workspace.consume` | 命名 Workspace | 进入该知识集组合面 | 任何 `knowledge.*` |
| `workspace.resolve` | Workspace | 解析 pin | 发权或读正文 |

交付链挂在 `SERVICE_ARCHITECTURE.md` §4.4：hydrate 得到 `SearchResult` 之后、transport 编码之前。公开类型是 `delivery.Envelope`（知识 ID 为 `PinnedKnowledgeRef`）、`delivery.Chain`、`delivery.Stage` 与首段 `delivery.RepositoryRead`；屏蔽命中沿用 KnowledgeHit，只去掉 Aspect 正文、保留固定元信息；不另造访客 DTO。公开命中字段由知识命中合同拥有。命名知识集与 `--repo` SEARCH 的证据是 `AUTH-01` / `AUTH-02`；链本身的证据是 `AUTH-03`。发现/过滤/交付链见 §7.2。


## 1. 默认粒度

Repository 是默认安全和治理边界；Workspace 只组合成员，不授予读权；协议授权使用已有 `kc` 动作；外部受保护操作仍由外部系统当场强制；外部授权快照可以是知识，不能反向成为知识仓 ACL。

| 边界 | 选择 |
|---|---|
| 发现边界 | Catalog 库存 = 已登记仓；Catalog 范围 SEARCH 候选 = discovery 成员（门槛均为 `catalog.read`） |
| 正文读边界 | 整个 Repository（`knowledge.read`） |
| 写约束 | 可进一步限制 ref 或 Address |
| 敏感度差异 | 真正构成安全边界时拆 Repository |
| Workspace | 每次读逐成员求值，不发权 |
| 外部业务授权 | 外部系统实时强制 |
| 外部授权快照 | `permissions` Aspect，属于 SOURCE 知识 |

---

## 2. 三套不能混合的权限

| 层 | 回答 | 权威 |
|---|---|---|
| Store 门禁 | 谁能碰 remote、目录、clone/push | Git 托管、文件系统、部署凭证；Gitea 的 private/collaborator；Dolt SQL user/GRANT（本仓库里是进程级库表权限，唯一客户端是 KC Server） |
| Knowledge Catalog 授权 | 谁能对 Repository/Catalog 执行某个 `kc` 动作 | 部署侧 allow policy |
| 外部操作强制 | 谁能在业务系统执行 SELECT、发布、运行任务等动作 | 外部系统当场决策 |

```text
Agent ── kc knowledge read ──→ Knowledge Repository
  │
  └── protected action ──→ External System
```

Catalog 不在外部操作路径上。能浏览关于某资源的知识，不等于能使用该资源。

Bound State READ 先通过 Workspace 的 `workspace.consume`（旧名 `read-workspace`）授权，才允许进入
`knowledge/serving.StateLookup`；lookup 请求继续携带已经建立的 `principal/onBehalfOf`，由墙外
runtime 对外部数据访问再次强制。KC 的仓读取授权不能替代源系统授权，runtime 拒绝时不得回退
到 Repository 中的 `null` 占位或旧缓存。consume 仍不授予 `knowledge.read`。

### 2.1 外部授权快照是知识

`permissions` Aspect 描述“某次观测时外部系统对谁开放了什么”。它与 facts、relations 等 SOURCE Aspect 同构：有 Address、commit、producedAt 和 provenance，也允许落后。

三条边界：

1. 可以进入 Canonical，供说明、候选过滤和审计。
2. 不要求与外部系统实时一致；必须暴露观测时间和来源。
3. 真正执行动作时仍问外部系统。副本说允许、外部系统说拒绝，最终必须拒绝。

谁能看见这份快照，只由知识仓授权决定；不能读取 Aspect 内容后再决定“你是否有权读取它”。检索面继续使用 Schema AccessHints，通常不把 GRANT 正文当全文文档。

---

## 3. 推导

### 3.1 为什么 ACL 边界等于 Repository

Repository 不只是文件目录，而是一张完整的 Snapshot 图：clone、log、diff、backup、projection 和 retention 都围绕它工作。

如果同一版本图内的两部分具有不同读者集合或历史可见性，那么“能打开这张图”本身已经扩大授权。对象/file ACL 会迫使每次历史读取、索引、备份和迁移都做正确裁剪；Git clone 更无法隐藏部分历史。

按 Repository 隔离后，问题退化为：

```text
principal × action × repository → allow | deny
```

Workspace 有 N 个成员时逐仓求值，复杂度与治理边界数量相关，而不是与对象数量相关。

### 3.2 Pin 不冻结授权

ResolvedWorkspace 固定本次数据坐标，不赋予未来访问权。每次命令按当前规则重新求值；否则一次旧 pin 会变成永久 capability，无法撤权。

### 3.3 配方不发权

`repo-add` 表示本机能够打开 Store；WorkspaceDefinition 表示配方希望组合成员；allow policy 才表示 principal 当前能执行动作。三者不能合并。

本 Catalog 已登记仓对持有 `catalog.read` 的主体可发现（§7.2）；发现不等于 `knowledge.read`。主动分享的便携配方还可能把 Repository identity 交给尚未持有 `catalog.read` 的接收者，那也不是读权。

---

## 4. 拆仓谓词

只有以下四个维度一致时，知识才适合放进同一 Repository：

| 维度 | 强行同仓的后果 |
|---|---|
| 读 ACL | clone、历史或索引泄漏 |
| 所有权/写权威 | 独立断言被当成覆盖冲突 |
| published ref 节奏 | 不同发布周期绑在一起 |
| 历史可见性 | 无法只隐藏旧 revision |

不应仅因源系统、文件类型、微服务、消费者数量、某个 Agent 或外部系统单条 GRANT 而拆仓。

经验判断：

- 两仓经常必须在同一次变更中一起修改，治理边界可能划错，应考虑合并。
- 两仓只是对同一主题有独立断言，应保持独立并在读侧并存。
- 同一 ACL 下仅写范围不同，用 Address 级写约束，不新建仓。
- 同一对象存在真正的敏感信息差，优先在受限仓写另一条知识并引用公共对象，不做 Aspect 级读 ACL。

---

## 5. Git 能解决什么

Git 擅长整仓访问、commit/ref、expected-old CAS、candidate branch 和评审路由；不擅长请求时身份、跨仓 Workspace、部分历史隐藏和对象级读授权。

因此：

- Git remote ACL 是 Store 门禁，可以与 KC 授权同向使用（例如仓默认 private，Server 用服务账号拉对象）；
- Gitea collaborator / org 不能自动变成 `knowledge.read`；Dolt SQL GRANT 只回答 KC 进程能不能打开那张库；
- Git author 或可伪造的环境字段不能作为可信 principal；
- CODEOWNERS 是评审路由，不是读隔离；
- Agent 若持有成员仓 clone，对象级只读已经失去意义；
- Store 替换时，授权语义不能依赖 GitHub 特有概念。

---

## 6. 业界调研与取舍

| 系统 | 借鉴 | 取舍 |
|---|---|---|
| Microsoft Purview | Collection 作为 metadata security boundary，Catalog 负责发现 | Repository 对应安全边界，Workspace 对应组合面 |
| Unity Catalog | 隔离单元与表级数据特权分开 | 外部 SELECT GRANT 不进入 KC allow |
| Dataplex | attach 外部资产而不复制 | Workspace 引用成员，不搬运正文 |
| dbt Mesh | project 是所有权边界，跨项目引用 | 经常协同修改说明边界可能过细 |
| GitHub/GitLab | Repository ACL、branch protection | 不把 CODEOWNERS 当文件读 ACL |
| DataHub | Policy 可对 Domain/instance 做细过滤 | 灵活但查询时授权和继承成本更高 |
| Atlas/Ranger | 元数据与业务特权由不同系统负责 | `permissions` Aspect 与实时强制分开 |
| Solid | 数据保留在原权威 | 资源级 ACL 复杂度不适合作为默认模型 |

不采用父级授权自动继承。Scope 不是目录优先级；Workspace union 对每个成员独立求值。

---

## 7. 授权面

### 7.1 身份与动作

principal 来自 Client 的显式本地身份或可信认证 facade 注入；所有业务请求都跨过 Server 认证/授权边界，不存在直接打开 Home 的 owner bypass。协议动作使用稳定 semantic action；组和角色属于 IdP，不在知识协议里再造对象树。

Catalog 范围发现与 SEARCH 按 `catalog.read` 求值，不先要 discovery Workspace 的 `workspace.consume`。命名知识集先判断 `workspace.consume`，再按仓求值 `knowledge.search` / `knowledge.read`；配方本身不发权。

Catalog 改动和 Repository 写入沿各自权威历史记录；成功读通常不写 Canonical。request/trace 只作为审计指针，不变成身份真相。

### 7.2 发现、过滤与交付链

消费请求分三段，不要混成一次授权：

```text
过滤：catalog.read + discovery Workspace 成员 + 固定元信息（如 repository）
  → ③ SEARCH：CandidateRef → 同一 basis hydrate Canonical
  → 交付链首段：无 knowledge.read 则屏蔽正文
  → 调用方
```

```mermaid
flowchart LR
  Filter[过滤]
  Search[SEARCH_hydrate]
  Chain[交付链首段]
  Caller[调用方]
  Filter --> Search
  Search --> Chain
  Chain --> Caller
```

检索仍是一份 AccessHints 投影（`RETRIEVAL.md`）。不为访客、不为每个 principal 重建索引。动作分责见文首接口表。

#### 发现

登记进本 Catalog 的仓，对已认证且持有该 Catalog `catalog.read` 的主体可出现在 Catalog 库存里（含源说明标题/摘要信封；缺说明是 `profile: missing`，不是 `FORBIDDEN`）。进入 discovery Workspace 的成员，可进入 Catalog 范围 SEARCH 的候选。这不是匿名读，也不授予 `knowledge.read`，也不另要 discovery 的 `workspace.consume`。不同意被发现就不登记，或换一间私有 Catalog。缺少 `knowledge.read` 或按仓 `knowledge.search` 都不把该仓从候选中抹掉。

System Repository 对已认证主体可发现、可读，由 `KNOWLEDGE_PRODUCT_AND_SCHEMA.md` 规定，不是业务仓的默认 grant 模式。

#### 固定元信息

知识对象带有协议坐标，不是业务正文：`repository`、`object_id`、`basis`、`schema_ref`（`TERMINOLOGY.md`）。索引携带它们，查询用 typed filter 缩小范围（例如只搜关心的仓）。Workspace、Pin、allow 规则和当前 principal 不是固定元信息，不编进索引文档。不在这四个坐标之外另挂「仓级可见性」过滤键。

#### 交付链

hydrate 之后、编码返回之前：按固定顺序挂接平台规则，逐条处理已 hydrate 的 hit。输入是知识 ID（加已 hydrate Canonical），输出是该主体可见的内容。这不是 ④ 协议层，也不是出站 Hook（`HOOKS.md`）。当前选定且仅选定第一段。公开类型见 `delivery/README.md`。

```text
hydrate Canonical
  → 1. 仓读权（已选定）：无 knowledge.read 则屏蔽 Aspect 正文，保留固定元信息
  → 调用方
```

屏蔽命中仍是 KnowledgeHit，不另造访客 DTO。每段可以原样放过、改写交付信封，或 fail closed。不得改 Candidate 身份、SearchView/basis，不得写回 Canonical，不得按人重建索引。新段只在拥有该主题的文档选定并出题之后往链上挂，不改检索代数。秘密字段不要标 `text`，否则 MATCH 仍能撞到它们；交付链不能消除关键词神谕。

#### 各消费面

- Catalog 范围 SEARCH：准入是该 Catalog 的 `catalog.read`；候选是 discovery 成员；调用方可用固定元信息过滤；命中走交付链首段。无 `knowledge.read` 不是 `partial` / `CAPABILITY_UNSATISFIED` 的理由。能力不足、投影缺失、预算耗尽仍报 `partial` 或 `CAPABILITY_UNSATISFIED`，不得伪装成零命中。SearchView 含本次实际检索到的 basis，包括调用方不能读正文的仓。
- 命名知识集 SEARCH：准入是该 Workspace 的 `workspace.consume` 与 `knowledge.search`；候选是该知识集成员；交付仍按仓 `knowledge.read` 屏蔽。
- `--repo` SEARCH：准入是该仓 `knowledge.search`；交付仍按 `knowledge.read`。
- `READ` / `RESOLVE` / `RELATIONS` / `LOG` / `GET_PROVENANCE`：无 completeness 信封，成员读权不齐时 fail closed，不能把拒绝伪装成空结果，也不能用「屏蔽正文仍 200」代替 `FORBIDDEN`。`RELATIONS` 要求成员仓级读权。对象 RESOLVE 授权复用 `knowledge.read`，不经 Catalog pin。
- 命名 `workspace resolve` / `describe-access` 若向调用方交出完整成员读侧元数据，要求全部成员的 `knowledge.read`。Catalog 范围 SEARCH 解析 discovery pin 不走这条，不要求成员读权。
- Workspace File Gateway / kcfs 交付字节正文；无权成员不进入 plan。不得把其输出当完整知识 SEARCH。
- 交付正文只认 Repository 级 `knowledge.read`；object 级规则不能授权未知对象的正文，也不能当成「看不见这个仓」。

### 7.3 认证与授权分开

认证回答“是谁”，allow policy 回答“能做什么”。每个业务请求都携带凭证；Server
不创建会话资源，Pin 也不绑定身份。Taihu 部署参数见 [`DEPLOY_AUTH.md`](DEPLOY_AUTH.md)；
传输头见 [`SERVICE_ARCHITECTURE.md`](SERVICE_ARCHITECTURE.md) §8.1。

#### 配对

Client 与 Server 只有两种合法配对，错配失败关闭：

| 配对 | Server | Client 只发送 |
|---|---|---|
| 测试 / 本机夹具 | `kc serve --auth local` | `X-Kc-As` |
| 产品 | `--auth taihu` 或 `--auth gitea` | `Authorization` |

`kc serve` 必须带 `--auth`。省略不得静默变成 local。进程内
`HTTPHandler(home)` 仍是测试接缝，语义等于 local，不是产品默认。

local 不是匿名：空 `X-Kc-As` 仍是 `UNAUTHENTICATED`。local 拒绝 `Authorization`
和客户端自报的 `X-Kc-On-Behalf-Of`（未验证委托等于冒充）。产品配对拒绝
`X-Kc-As` 和客户端自报的 `onBehalfOf`。两种凭证同时出现，产品侧为 `FORBIDDEN`。

`--as` / `KC_AS` / `kc login --mode local --as` 只是 local 配对的测试捷径，不能
写成生产登录。产品身份只来自已验证认证器。

#### 三种主体，两种产品登录

复用已有 `principal` 与可选 `onBehalfOf`，不新增第三种协议对象。KC 按
`principal × action × repository` 授权；`onBehalfOf` 只进证据，不参与授权交集。
Agent 不得把用户写成 principal。

Taihu 用户名在 IdP 内唯一且不可变，因此也是 KC 用户 principal 的稳定标识。
allow.json 发权给 `taihu:<username>`。工号（introspection `sub` / 网关 `staff_id`）
只作为 `subject` 相关，不进入授权键。用户或委托 token 缺少 username 时认证失败关闭，
不得回退到工号。

| 种类 | principal | onBehalfOf | 怎样证明 |
|---|---|---|---|
| 用户 | `taihu:<username>`（Gitea 为 `gitea:<id>`） | 空 | 用户本人登录 |
| Agent 代理用户 | `agent:<id>` | `taihu:<username>` | 用户同意后由 token 携带 actor+subject；禁止 `kc login --as <user>` |
| 服务账号 | `service:<id>` | 空 | 已授权的机器主体；产品侧 Taihu `client_credentials`，测试侧 `--as service:bootstrap` |

`KC_SERVICE_CLIENT_SECRET` 与 `KC_TAIHU_HMAC_SECRET` 是 KC 资源方凭证，不是调用方
身份，只从部署环境注入。需要委托的测试不得用 local 自报 header，应注入 fake authenticator。

新的本机 Home 尚无 allow rule，因此提供唯一一个宿主级引导动作：

```bash
kc local grant bootstrap --home .kc --principal user:local-admin
```

它只在 allow 为空时创建首个全局管理 rule；一旦已有任何 rule 就失败关闭，不能覆盖治理状态。它与 `kc local init` / `repository attach` 一样属于宿主 bootstrap，不是第二套业务 API。首个 principal 建立后，后续 `kc admin grant ...` 也必须作为 Client 经 Server 执行。

启动命令、Taihu claim 名和当前覆盖状态由 CLI/HTTP 代码、`DEPLOY_AUTH.md` 与
`TEST_CATALOG.md` 维护，不在本文复制。

### 7.4 调用可观测性

调用身份冻结为两个字段：`principal` 是实际执行主体，`onBehalfOf` 是可选的被代理用户。Agent 代理用户时不能把用户冒充成 principal；KC 按 principal 授权，并完整记录两者。

成功、失败和拒绝的消费访问追加到访问证据库，每个命中都绑定固定 Repository、commit、object/Address。反馈按 trace 关联，hitmap 从访问账派生；这些都是过程证据，不写回 Canonical。查询走 `audit.read`；store 不按审计员身份再裁剪「谁访问过」。完整契约见 [`OBSERVABILITY.md`](OBSERVABILITY.md)。

---

## 8. 代码是具体协议说明

- 公开动作名与阶段分责：本文接口表
- allow 求值与认证装配：`cli/` 参考实现（不得用其隐含关系收窄本文）
- Workspace 逐成员读取：`knowledge/reader/serving.go`、CLI consume tests
- 交付链缝：`delivery.Chain` / `delivery.Envelope`；政策见本文 §7.2；不得写入 `retrieval/` / `index/`
- Catalog 库存与源说明拼装：`KNOWLEDGE_PRODUCT_AND_SCHEMA.md`；参考实现 `catalog/`、CLI catalog tests
- `permissions` Aspect：普通 Writer/Reader/Schema 路径
- Hook/Gate/外部资源边界：`HOOKS.md`、`GATES.md`、`CONNECTORS.md`
- 访问身份、trace/feedback 与 hitmap：`OBSERVABILITY.md`、`observability/`

规则字段、文件布局、启动命令和当前覆盖状态不再复制到设计文档；它们应只在代码、帮助文本、包 README 和测试目录中维护。
