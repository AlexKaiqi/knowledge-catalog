# 权限模型：按仓隔离、发现与读分层

日期：2026-09-03
范围：谁能对哪份知识执行哪类 `kc` 动作。公开动作名与默认边界由本文拥有；规则字段一旦选定，由 allow 策略合同描述，本文不重贴。

本文回答：为什么安全边界默认是 Repository，为什么知识集组合不能扩大授权，为什么登记进 Catalog 等于可被发现但不等于可读正文，以及知识仓中的外部授权快照为什么不能替代外部系统实时强制。

---

## Goal

回答谁能对哪份知识执行哪类 `kc` 动作：可授权资源是 Catalog、Repository 与 Dataset。维护通道的安全边界是 Repository；消费通道的读权是 Dataset 上的 `file.read`。登记进本 Catalog 的仓对已认证主体默认可发现（Catalog 可声明 private，此时仍要 `catalog.read` grant）；`--repo` 正文、历史、关系仍要仓级 `knowledge.read`；Dataset 组合不扩大仓权。外部授权快照不能替代源系统实时强制。

## Non-Goals

- 不把 Catalog 权限做成文件 ACL，不按 Ranger/Unity 表 GRANT 拆知识仓。
- `permissions` Aspect 不是 `kc read` 闸门，也不能放行 SELECT。
- 不做 GitHub 式文件 ACL/CODEOWNERS 解释器。
- 不发明与 `kc` 动作平行的授权枚举（下文「默认粒度」）。
- 不复制 allow 规则字段全集（形状由 allow 策略合同拥有，不在本文重贴）。
- 不按 path 授权；不以对象级读 ACL 作为主模型。
- Repository 接入 / 知识集定义不隐式发权；不在协议里建角色/组继承树。
- 不为访客或每个 principal 重建检索索引；发现过滤只用已命名的固定元信息，不是第二份投影。
- 不把仓级「访客/成员可见性」或其它业务分类做成第二套过滤键。README 不承载分类或授权（`KNOWLEDGE_PRODUCT_AND_SCHEMA.md`）。
- 不选定字段级隐私化 / 脱敏声明语言；交付链选定且仅选定首段「无 `knowledge.read` 则屏蔽正文」。后续段在拥有该主题的文档选定并写入 `ARCHITECTURE_INVARIANTS.md` 之前，不得实现。
- 不把 `LIVE_MATERIALIZATION.md` 的 continuation / replay token 当成交付链上的正文裁剪规则。
- 交付链不是出站 Hook，也不在 READ/SEARCH 上挂用户脚本（`HOOKS.md`）。
- 不把 Gitea collaborator / Dolt SQL GRANT 当成 `kc` 动作授权。
- 不把 `onBehalfOf` 与用户权限求交（除非另开 ADR）。
- 不提供匿名访客读。

## 硬性约束 / Invariants

已固化（`ARCHITECTURE_INVARIANTS.md`）：

- `KS-02` 成为 Dataset 成员不获得仓级 READ；`file.read`×Dataset 与仓 `knowledge.read` 互不蕴含；旧 pin 不能绕过撤权。
- `C-01` Canonical 只从固定 authority basis 解释；hydrate 义务不因交付链屏蔽正文而取消。本文不重定义 hydrate。
- `AUTH-01` `--repo` SEARCH 不按 `knowledge.read` 裁候选；无仓读权屏蔽正文且不是 `partial`。Dataset SEARCH 候选必须落在当前服务版文件范围内。
- `AUTH-02` Dataset 上的 `file.read` 足以读清单内文件并在这些文件上解释知识；不放行任何仓上的 `knowledge.*` / `writer.*`。`catalog.read` 不能跳过 Dataset `file.read`。
- `AUTH-03` 交付链输入是已 hydrate 的知识 ID；按序改写可见正文；无读权只清空正文；不得改 ID/Address。

其它已选定、尚未进入不变量索引：

- Catalog 只提供有界库存发现，不是知识 SEARCH 范围。消费方按一个 Repository 搜索，或先
  固定多 Repository pin。
- 调用方看见 Canonical 正文要求仓级 `knowledge.read`。禁止观察：精确 READ 用屏蔽正文的 200 代替 `FORBIDDEN`。
- 授权按 `principal` 求值；`onBehalfOf` 只是审计事实（`OBSERVABILITY.md`）。
- 首次部署初始化只在空耐久授权状态中建立配置声明的首个管理主体；重新部署恢复既有 grants，业务命令无 owner bypass。

## 选定方案 / 被否决方案

- 选定：按治理边界拆 `--repo`；发权是 `kc grant add`；外部 GRANT 快照作为 SOURCE 知识。
- 选定：发现与读分层——一份 AccessHints 索引；查询过滤只用固定元信息（`repository`、`object_id`、`basis`、`schema_ref`）；hydrate 之后走交付链，当前只挂首段仓读权屏蔽。
- 选定：已认证主体默认可发现公开 Catalog；Catalog 可声明 `private`，此时仍要该 Catalog 的 `catalog.read` grant。发现不等于 `knowledge.read`。
- 选定：仓可声明已认证默认可读动作（与 Catalog 公开发现同一模式）。未声明则 fail closed，认 grant。System Repository 选用该声明，授权器不按保留 ID 短路。
- 否决：仓级 public/private 类型；把可见性编进检索索引；已认证默认放行写、发权或管理；按 `kr://kc/system` 在授权器或交付链特例放行。
- 选定：动作按阶段分责（见接口表）。`catalog.read` 只覆盖该 Catalog 库存；`--repo` SEARCH 要仓 `knowledge.search`，正文只认仓级 `knowledge.read`。Dataset 消费只认该 Dataset 的 `file.read`。
- 选定：Bound State / `resource-access` 出站调用携带已经建立的调用方认证证明（Taihu 为 `Authorization` 与/或 `X-Tai-Identity`，以及 `X-Resource-Principal`）。KC 仓授权不能替代源侧强制；源侧可以忽略这些头，KC 不得省略。
- 否决（本文边界）：父级授权自动继承；把知识仓 ACL 做成 Ranger 镜像；按表 GRANT / 单个 Agent / 单个知识集拆仓；按人复制索引；先省略无权仓再假装 Catalog 不可发现；把未命名的仓级可见性或隐私化当成已选定链段；用 `file.read` 或按仓 `knowledge.search` 裁 discovery 候选。知识集 union 当目录优先级见系统设计 [R-05](KNOWLEDGE_CATALOG_DESIGN.md#r-05)。成员仓 clone 后不再声称对象级只读。

## 接口契约 / 状态机

三套不能混合的权限见下文。公开动作名由本文拥有；allow 规则字段由 allow 策略合同拥有，不在本文重贴。Client↔Server 配对由本文与 `SERVICE_ARCHITECTURE.md` 分责。参考实现把 SEARCH 登记成同一内部动词，**求值仍按本表**，不得用当前 `cli/allow.go` 的隐含关系收窄设计。

| 动作 | 典型范围 | 放行 | 不放行 |
|---|---|---|---|
| `catalog.repositories.create` | Catalog | 按平台显式供给策略申请新仓并完成准入 | 管理其它成员、选择存储地址、任意发权、Catalog 库存读取 |
| `catalog.read` | Catalog | 该 Catalog 库存（仓库 id 与 `schemaCount`）；公开 Catalog 对已认证主体默认放行，`private` 仍要 grant | SEARCH；成员正文；README；`knowledge.schema.read`；VFS 字节 |
| `knowledge.search` | `--repo` 或 Dataset | 对该范围调用 SEARCH | `--repo` 正文；Catalog 库存；按仓裁 discovery 候选 |
| `knowledge.read` | Repository | 交付链放行正文；精确 READ / RESOLVE / LOG / GET_PROVENANCE；该仓 `--repo` VFS。仓可声明已认证默认，此时已认证主体无需逐人 grant | 调用 SEARCH；从发现候选抹仓；写；未声明仓的默认读；Dataset 消费 |
| `knowledge.schema.read` | Repository | schema describe / browse | 实例正文；不被 `catalog.read` 隐含 |
| `file.read` | Dataset 或 Repository | Dataset：打开该服务版清单内 blob，并在这些文件上解释知识（含 pin SEARCH/READ）。仓：`--repo` VFS 字节 | 对另一资源的 `knowledge.*` / `writer.*`；清单外 path |
| `dataset.resolve` | Dataset | 解析 `latest` / `vN` pin | 发权或写 |
| `dataset.manage` | Dataset | 发布或退役命名 Dataset | 发权、读清单内文件 |

开放决策（REVIEW-02）：本表把历史与来源读取归入仓级读权，公开动作登记却有独立的历史、
来源及关系准入动作，单仓和组合入口的求值也不完全相同。需统一「仓读权是否足以调用这些
动作」及各入口的一致性；决定会影响最小 grant、既有授权兼容和 Conformance。未决前不能
把某个入口的实现行为当成所有入口的授权保证。

交付链挂在 `SERVICE_ARCHITECTURE.md` §4.4 的 hydrate 之后、transport 编码之前：保留知识身份和固定来源，只按已选政策改变正文可见性。信封、链和首段过滤器的公开类型由 [`delivery/README.md`](../delivery/README.md) 与 `delivery/` 拥有，不另造访客 DTO。命名知识集与单仓 SEARCH 的证据是 `AUTH-01` / `AUTH-02`；链本身的证据是 `AUTH-03`。发现/过滤/交付链见 §7.2。

上表是应然动作合同。已暴露入口由 [`cli/SURFACE.md`](../cli/SURFACE.md) 维护，产品 argv 分组见 [`CLI.md`](CLI.md)；未满足项只在 [`MVP_ACCEPTANCE.md`](MVP_ACCEPTANCE.md) 记录；不能以当前 CLI 缺少入口收窄本表。交付链后续隐私化未选定，禁止实现。


## 1. 默认粒度

Repository 是默认安全和治理边界；知识集只组合成员，不授予读权；协议授权使用已有 `kc` 动作；外部受保护操作仍由外部系统当场强制；外部授权快照可以是知识，不能反向成为知识仓 ACL。

| 边界 | 选择 |
|---|---|
| 发现边界 | Catalog 库存 = 已登记仓；知识候选由显式 Repository 或固定 pin 决定 |
| 正文读边界 | 整个 Repository（`knowledge.read`） |
| 写约束 | 可进一步限制 ref 或 Address |
| 敏感度差异 | 真正构成安全边界时拆 Repository |
| 知识集 | 每次读逐成员求值，不发权 |
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
Agent ── kc read ──→ Knowledge Repository
  │
  └── protected action ──→ External System
```

Catalog 不在外部操作路径上。能浏览关于某资源的知识，不等于能使用该资源。

Bound State READ 先通过 Dataset 的 `file.read` 授权，才允许进入
`knowledge/serving.StateLookup`；lookup 请求继续携带已经建立的调用方认证证明
（Taihu：`Authorization` 与/或已验证 `X-Tai-Identity`）和 `principal/onBehalfOf`，由墙外
runtime 对外部数据访问再次强制。用不用由接入方决定。KC 的仓读取授权不能替代源系统授权，runtime 拒绝时不得回退
到 Repository 中的 `null` 占位或旧缓存。Dataset `file.read` 仍不授予仓上的 `knowledge.read`。

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

知识集有 N 个成员时逐仓求值，复杂度与治理边界数量相关，而不是与对象数量相关。

### 3.2 Pin 不冻结授权

ResolvedKnowledgeSet 固定本次数据坐标，不赋予未来访问权。每次命令按当前规则重新求值；否则一次旧 pin 会变成永久 capability，无法撤权。

### 3.3 配方不发权

Repository 接入表示服务验证了既有 authority 并完成 Catalog 成员登记；KnowledgeSet 表示配方希望组合成员；allow policy 才表示 principal 当前能执行动作。三者不能合并。平台仓创建需要独立 Catalog 创建准入；创建者的仓级能力只来自部署明确配置的窄动作策略，落实为普通、可审计且可撤销的 allow 规则。没有默认读写权限，不允许请求方自选授权。创建的幂等重放和服务重启不会补回已撤销规则，也不把创建者变成全局管理员。

本 Catalog 已登记仓对已认证主体默认可发现（§7.2）；声明 private 后仍要 `catalog.read` grant。发现不等于 `knowledge.read`。主动分享的便携配方还可能把 Repository identity 交给尚未持有发现权的接收者，那也不是读权。

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

Git 擅长整仓访问、commit/ref、expected-old CAS、candidate branch 和评审路由；不擅长请求时身份、跨仓知识集、部分历史隐藏和对象级读授权。

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
| Microsoft Purview | Collection 作为 metadata security boundary，Catalog 负责发现 | Repository 对应安全边界，知识集对应组合面 |
| Unity Catalog | 隔离单元与表级数据特权分开 | 外部 SELECT GRANT 不进入 KC allow |
| Dataplex | attach 外部资产而不复制 | 知识集引用成员，不搬运正文 |
| dbt Mesh | project 是所有权边界，跨项目引用 | 经常协同修改说明边界可能过细 |
| GitHub/GitLab | Repository ACL、branch protection | 不把 CODEOWNERS 当文件读 ACL |
| DataHub | Policy 可对 Domain/instance 做细过滤 | 灵活但查询时授权和继承成本更高 |
| Atlas/Ranger | 元数据与业务特权由不同系统负责 | `permissions` Aspect 与实时强制分开 |
| Solid | 数据保留在原权威 | 资源级 ACL 复杂度不适合作为默认模型 |

不采用父级授权自动继承。Scope 不是目录优先级；知识集 union 对每个成员独立求值。

---

## 7. 授权面

### 7.1 身份与动作

principal 来自 Client 的显式本地身份或可信认证 facade 注入；所有业务请求都跨过 Server 认证/授权边界，不存在直接打开 Home 的 owner bypass。协议动作使用稳定 semantic action；组和角色属于 IdP，不在知识协议里再造对象树。

公开 Catalog 库存对已认证主体默认可发现；声明 private 后按该 Catalog 的 `catalog.read` grant 求值。知识 SEARCH 只接受显式 Repository 或固定 pin，并按仓
求值 `knowledge.search` / `knowledge.read`；配方本身不发权。

Catalog 改动和 Repository 写入沿各自权威历史记录；成功读通常不写 Canonical。request/trace 只作为审计指针，不变成身份真相。

### 7.2 发现、过滤与交付链

消费请求分三段，不要混成一次授权：

```text
过滤：显式 Repository / pin 成员 + knowledge.search + 固定元信息
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

登记进本 Catalog 的仓，对已认证主体默认可出现在 Catalog 库存里（身份列表；缺 README 不从库存抹仓，也不是 `FORBIDDEN`）。Catalog 可声明 private，此时仍要该 Catalog 的 `catalog.read` grant。这不是匿名读，也不授予 `knowledge.read` 或 `knowledge.search`，更不把 README 展成库存 title/summary。不同意被默认发现就声明 private，或换一间私有 Catalog。

仓默认无读权。部署可按仓声明已认证默认可读动作（consume 侧闭集：`knowledge.read` / `knowledge.schema.read` / `knowledge.search` / 历史与来源 / `file.read` / `projection.read`）。未声明则仍要 grant。这不是仓类型，不是 SEARCH 过滤键，也不放行写或发权。无 Deployment 的本地 Home 没有 Catalog 公开默认；本地 init 把同一份声明写入耐久文件。System Repository 是该声明的第一个使用者，见 `KNOWLEDGE_PRODUCT_AND_SCHEMA.md`。

#### 固定元信息

知识对象带有协议坐标，不是业务正文：`repository`、`object_id`、`basis`、`schema_ref`（`TERMINOLOGY.md`）。索引携带它们，查询用 typed filter 缩小范围（例如只搜关心的仓）。知识集、Pin、allow 规则和当前 principal 不是固定元信息，不编进索引文档。不在这四个坐标之外另挂「仓级可见性」过滤键。

#### 交付链

hydrate 之后、编码返回之前：按固定顺序挂接平台规则，逐条处理已 hydrate 的 hit。输入是知识 ID（加已 hydrate Canonical），输出是该主体可见的内容。这不是 ④ 协议层，也不是出站 Hook（`HOOKS.md`）。当前选定且仅选定第一段。公开类型见 `delivery/README.md`。

```text
hydrate Canonical
  → 1. 仓读权（已选定）：无 knowledge.read 则屏蔽 Aspect 正文，保留固定元信息
  → 调用方
```

屏蔽命中仍是 KnowledgeHit，不另造访客 DTO。每段可以原样放过、改写交付信封，或 fail closed。不得改 Candidate 身份、SearchView/basis，不得写回 Canonical，不得按人重建索引。新段只在拥有该主题的文档选定并出题之后往链上挂，不改检索代数。秘密字段不要标 `text`，否则 MATCH 仍能撞到它们；交付链不能消除关键词神谕。

#### 各消费面

- 命名 Dataset SEARCH：准入是该 Dataset 的 `file.read`；候选是当前服务版清单内文件；不得搜出清单外对象再剥正文。
- `--repo` SEARCH：准入是该仓 `knowledge.search`；交付仍按 `knowledge.read`。
- `READ` / `RESOLVE` / `RELATIONS` / `LOG` / `GET_PROVENANCE`：`--repo` 无仓读权则 fail closed。Dataset 通道有 `file.read` 即读清单内文件。`RELATIONS` 在 Dataset 通道上同样只解释清单内文件。
- 命名 `pin` / `operations access-spec describe` 若向调用方交出完整成员读侧元数据，要求全部成员的 `knowledge.read`。
- Knowledge Set File Gateway / kcfs 交付字节正文；无权成员不进入 plan。不得把其输出当完整知识 SEARCH。
- 交付正文只认 Repository 级 `knowledge.read`；object 级规则不能授权未知对象的正文，也不能当成「看不见这个仓」。

### 7.3 认证与授权分开

认证回答“是谁”，allow policy 回答“能做什么”。每个业务请求都携带凭证；Server
不创建会话资源，Pin 也不绑定身份。Taihu 部署参数见 [`DEPLOY_AUTH.md`](DEPLOY_AUTH.md)；
传输头见 [`SERVICE_ARCHITECTURE.md`](SERVICE_ARCHITECTURE.md) §8.1。

#### 配对

Client 与 Server 的认证模式必须显式配对，错配失败关闭。本地断言只用于受控测试与夹具，
也必须有明确身份；产品身份来自已验证的认证器。未声明模式不能静默变为 local，任何模式
都不能接受未经验证的委托。具体 header、flag、错误码和进程内测试接缝统一由
[`cli/README.md`](../cli/README.md) 与 [`client/README.md`](../client/README.md) 维护。

#### 三种主体，两种产品登录

复用已有 `principal` 与可选 `onBehalfOf`，不新增第三种协议对象。KC 按
`principal × action × repository` 授权；`onBehalfOf` 只进证据，不参与授权交集。
Agent 不得把用户写成 principal。

用户公开 principal 使用经过验证的 KC 用户名。认证 adapter 同时保留 IdP 的可信 issuer 和稳定、不可重新指派的 subject，Server 将它们与用户名耐久绑定；不同 subject 不能因复用同名而取得旧用户权限。改名或迁移必须显式处理，不能在登录时临时换字段、自动合并提供方或恢复已撤销规则。claim 映射由 [`cli/README.md`](../cli/README.md) 与认证器公开合同维护。

| 种类 | 实际执行主体 | 代理关系 | 怎样证明 |
|---|---|---|---|
| 用户 | 已验证用户 | 无 | 用户本人登录 |
| Agent 代理用户 | 已验证 Agent | 已验证的被代理用户 | 用户同意且认证器验证委托声明 |
| 服务账号 | 已授权机器主体 | 无 | 身份系统验证机器凭证 |

`KC_SERVICE_CLIENT_SECRET` 与 `KC_TAIHU_HMAC_SECRET` 是 KC 资源方凭证，不是调用方
身份，只从部署环境注入。需要委托的测试不得用 local 自报 header，应注入 fake authenticator。

首次部署初始化在明确的空耐久状态中建立配置声明的首个管理主体。初始化必须区分新部署与恢复：已有规则不可被新的 bootstrap principal 覆盖；缺失授权状态不可被当成一次全新初始化。授权与 Gate 配置必须随部署独立持久保存，不能因替换实例而清空。

首个 principal 建立后，后续授权管理由 Client 经 Server 执行。部署配置与首次初始化的公开形状由 `home/` 和 `cli/` 拥有，不另保留本机发权命令。

启动命令、Taihu claim 名和当前覆盖状态由 CLI/HTTP 代码、`DEPLOY_AUTH.md` 与
`TEST_CATALOG.md` 维护，不在本文复制。

### 7.4 调用可观测性

调用身份冻结为两个字段：`principal` 是实际执行主体，`onBehalfOf` 是可选的被代理用户。Agent 代理用户时不能把用户冒充成 principal；KC 按 principal 授权，并完整记录两者。

成功、失败和拒绝的消费访问追加到访问证据库，每个命中都绑定固定 Repository、commit、object/Address。反馈按 trace 关联，hitmap 从访问账派生；这些都是过程证据，不写回 Canonical。查询走 `audit.read`；store 不按审计员身份再裁剪「谁访问过」。完整契约见 [`OBSERVABILITY.md`](OBSERVABILITY.md)。

---

## 8. 代码是具体协议说明

- 公开动作名与阶段分责：本文接口表
- allow 求值与认证装配：`cli/` 参考实现（不得用其隐含关系收窄本文）
- 知识集逐成员读取：`knowledge/reader/serving.go`、CLI consume tests
- 交付链缝：`delivery.Chain` / `delivery.Envelope`；政策见本文 §7.2；不得写入 `retrieval/` / `index/`
- Catalog 库存身份列表：`KNOWLEDGE_PRODUCT_AND_SCHEMA.md`；参考实现 `catalog/`、CLI catalog tests
- 仓已认证默认可读：部署 `repositoryAccess` 与 `home.RepositoryAccess`；不得按 System Repository ID 短路
- `permissions` Aspect：普通 Writer/Reader/Schema 路径
- Hook/Gate/外部资源边界：`HOOKS.md`、`GATES.md`、`CONNECTORS.md`
- 访问身份、trace/feedback 与 hitmap：`OBSERVABILITY.md`、`observability/`

规则字段、文件布局、启动命令和当前覆盖状态不再复制到设计文档；它们应只在代码、帮助文本、包 README 和测试目录中维护。
