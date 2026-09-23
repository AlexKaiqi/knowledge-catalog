# 动态知识物化与统一检索

日期：2026-09-21
定位：Binding/Observation 与统一检索的语义设计。实现状态只在 `MVP_ACCEPTANCE.md` /
`TEST_CATALOG.md` 维护；State 控制算法见 `PROJECTION_CONTROLLER.md`。

本文解释高频变化的当前态和事件流为什么不属于 Knowledge Catalog 的权威 Store，以及怎样通过版本化 Aspect 句柄进入统一检索。字段形状选定后由 Conformance 钉死；未冻结的 Stream 问题列在 §8.3，不是「不做」。

---

## Goal

说明高频当前态和事件流为什么不属于 Knowledge Catalog 的权威 Store，以及稳定 Aspect 如何通过版本化 Binding 句柄被观察。下一阶段先完成 State 的可信消费闭环：按声明取值、后台恢复、同依据交付、有界保留观察，以及当前授权。统一检索代数见 `RETRIEVAL.md`。

## Non-Goals

- 不把外部 runtime 的 checkpoint/WAL 登记成 Knowledge Repository（下文「为什么」）。
- 实时 State/Stream 的运行与存储不属于 Store Adapter（`STORE_ADAPTERS.md`）。
- 本文不维护实现完成度（`MVP_ACCEPTANCE.md` / `TEST_CATALOG.md`）；控制算法不在本文（`PROJECTION_CONTROLLER.md`）。
- 不拥有 SEARCH 查询代数（`RETRIEVAL.md`）。
- 不拥有发现/读授权与交付屏蔽（`PERMISSIONS.md`）。

## 硬性约束 / Invariants

- `D-01` Bound State 必须同时标识声明 basis 与 observation basis；Stream 不得隐式数组化（`K-28`）。
- `P-01` 投影失败不得回滚 Canonical commit。
- `V-01` 消费 SEARCH 使用本次解开的 commit，不回绕 live HEAD。
- Catalog 不固定动态 cut（[ADR-022](KNOWLEDGE_CATALOG_DESIGN.md#adr-022)）。

## 选定方案 / 被否决方案

- 选定：[ADR-017](KNOWLEDGE_CATALOG_DESIGN.md#adr-017) / [ADR-022](KNOWLEDGE_CATALOG_DESIGN.md#adr-022) / [ADR-027](KNOWLEDGE_CATALOG_DESIGN.md#adr-027)：② 只保存 Binding/ResourceDescriptor；Serving 经窄端口 hydrate；③ 按 capabilities 编 RetrievalPlan。
- 选定：invalidate-and-pull 的通知角色规范名称是 Observer（`TERMINOLOGY.md` §6）；Collector 对账后发 Writer；Resource Access 提供 origin 访问地址。
- 选定：Bound State 的 `resource-access/v1` 原点写在 Domain Schema Canonical frontmatter 的 `origin`（http(s) 原点，不含 `/v1/access`）。`kc access` 用 origin + 实体 `object_id` 取回该 Aspect；不另存 `null` 实例文件。多接入方各写自己的 Schema。ResourceDescriptor 操作同样在描述里声明 `origin`。
- 选定：出站身份必须与受信任目标、凭证适用范围和委托目的匹配；投影 refresh 使用独立服务身份，不把用户 token 写入 Schema。源侧逐调用方授权与显式共享观察的边界由 `PERMISSIONS.md` §2 拥有。
- 选定：分别解释查询覆盖、新鲜度、可重读性与授权，不能用其中一项代替其它项的证明。默认时效与响应政策仍待选定，不因此放宽现有 complete 条件。
- 选定：State 阶段纳入有界保留的观察记录；它提供已观察值的重读依据，不提供未观察时刻的源历史。Stream 窗口与持续订阅后续分别设计。
- 否决（本文边界）：APPEND Surface；访问默认沉淀为知识；整台 Knowledge Server 一个 `KC_RESOURCE_ACCESS_URL` / `--resource-access-url`；为瞬时值再 PUT 一份空 Aspect 句柄。系统级拒绝见 [R-03](KNOWLEDGE_CATALOG_DESIGN.md#r-03)。

## 接口契约 / 状态机

Binding/Observation 语义以本文为准。消费侧需要可注入的 State 读取端口。SEARCH 代数与 RetrievalPlan 见 [`RETRIEVAL.md`](RETRIEVAL.md)。参考实现：`knowledge/serving.StateLookup`、`index.ChangeNotice`。


## 1. 为什么 Snapshot 不能当运行时

ETL 任务展示了一个常见分裂：

- `job_id`、Aspect、Schema 和访问方式长期稳定；
- running/failed/progress 等当前值高频变化；
- 运行记录是不断增长的事件流；
- 用户既要搜“失败的任务”，也要读取某个任务的最新状态或历史窗口。

Git Snapshot 适合保存稳定知识，却不适合作为实时状态和事件流运行时。反过来，只有一个可调用句柄能读取已知资源，却不能让用户发现候选。

这里的“不进入 Snapshot”只指**不进入 Knowledge Catalog 的 Canonical Repository**。流处理器、数据库和时序系统仍会为故障恢复制作 checkpoint、WAL 或内部 snapshot；这些是运行时拥有、按 retention 回收的恢复产物，不是用户可治理的知识版本，不能因为也叫 snapshot 就注册成 Knowledge Repository。

问题因此是：

> Repository 保存什么稳定声明，外部产品承担什么运行语义，Retrieval 又怎样把两者编译成统一发现与回源路径？

---

## 2. 推导

“非稳定”不是一种数据类型，变化频率也不是唯一落位依据。决定载体前先问：谁是权威、读取的是
当前态还是历史、是否要求回放、能冻结什么 basis、允许怎样的 retention/过期。低频的运行状态
仍可能只是一种 observation；高频结果若经过确认并需要治理，也可以显式捕获为 Snapshot 知识。

### 2.1 知识声明与运行值分开

Aspect 回答“这是什么知识”；Binding 回答“怎样观察它”。前者和后者都可以作为稳定声明随 Repository commit 版本化，但 Binding 指向的当前值、事件、cursor 和 watermark 不进入 Snapshot。

```text
② Repository knowledge
   Aspect identity / schema / access hints / binding handle
                              ↓
M  external materialization runtime
   state / events / cursor / watermark / health
                              ↓
③ retrieval projection and routing
```

M 是上层产品能力，不是 Knowledge Catalog 新增的编号协议层，也不进入底座 import DAG。

### 2.2 State 与 Stream 是两种访问形态，不是完整介质分类

- State：每个 Address 在一次观察上有 0..1 个当前值。
- Stream：每个 Address 或 ResourceRef 有 0..N 条有序记录。

这是 Binding 对调用方暴露的最小逻辑形态。实际运行系统通常还会按责任继续拆分：当前态 KV/源查询、可回放事件日志、指标/日志/trace 时序存储、流处理 checkpoint，以及面向查询的 materialized view/index。时序观测可以通过 Stream Binding 暴露，checkpoint 不属于 ValueSource，materialized view/index 则属于派生 serving state。

State 与 Stream 可以互相派生：事件 Fold 成当前态，当前态变化可产生 change stream。但这只说明物化代数，不表示它们具有相同 retention、顺序、恢复和查询语义，更不表示底座需要拥有 Stream Store。

### 2.3 每次动态观察仍必须有 basis

把 Stream 搬出底座不会消除 cursor、watermark、source revision、observedAt 和 late-data 语义。它只改变这些语义的责任人：

- Snapshot basis 由 Catalog/Repository 的 commit 保证；
- State/Stream observation basis 由 Materialization Runtime 保证；
- Retrieval 结果必须把两类 basis 分开返回。

没有 observation basis 的“实时”无法复核，也无法解释索引与回源不一致。

不同源能提供的一致性强度不同，上层不能统一伪装成 repeatable read：

- repeatable：源能在可定位的版本或冻结依据上重读原值；仅有 generation 名称不构成重读证明；
- bounded：在来源明确承诺的界限内解释观察；单调 revision/watermark 本身既不证明有界 freshness，也不证明旧值可重读；
- latest-only：源只能读取调用时最新值，本身不保证跨页或多次读取一致；平台可靠保留实际观察后，可在保留期内重读该观察，不能据此提升源能力声明。

多个 Binding 通常也没有一个全局原子 cut。除非外部运行时另有协调协议，动态 projection revision 只能标识本次收集到的一组 observations，不能伪装成源系统的“全局实时快照”。SearchView 保存该紧凑 revision；每个命中的 KnowledgeVersion 保存其实际 observation bases，而不是把全库 observations 内联进响应。

### 2.4 索引始终是派生状态

Snapshot、State 和 Stream 的索引都只定位候选。CandidateRef 不携带知识正文；命中后必须通过 typed reference 回到 Snapshot 或固定 Binding 读取完整知识及版本。物理引擎的 stored fields、summary、doc values 或 `_source` 只可作为内部优化，不能成为协议结果。候选已变化、消失或不可按原 basis 重读时必须拒绝该结果；partial 只描述已声明的 approximate coverage 或预算耗尽，不能掩盖投影与权威不一致。

### 2.5 Invalidation 不证明完整

通知可能丢失、合并或乱序。动态投影若要声明完整，Materialization Runtime 必须至少提供 delta、enumerate/checkpoint、周期 reconcile 或有界 TTL 中的一组恢复机制。

TTL 只能让超期观察失去“足够新”的资格，不能发现漏掉的变化；周期任务存在也不等于已在期限内
完成对账。必须以恢复实际完成的范围与依据解释承诺，不能以调度配置或最近一次收到通知代替。

### 2.6 一次动态消费有四个独立问题

| 问题 | 必须说明什么 | 不能据此推出什么 |
|---|---|---|
| 查询覆盖 | 在固定声明范围与选定观察集合上，必需条件是否查全 | 外部源没有发生未观察的变化 |
| 新鲜度 | 观察年龄、来源进度与恢复证据是否满足此次用途 | 单凭观察时间就知道源系统进度 |
| 可重读性 | 来源或平台能否在承诺保留期内重读同一观察 | 可以回放从未观察到的源历史 |
| 当前授权 | 本次调用能否访问该来源或获准共享的观察 | 曾经成功访问、持有旧 pin 或命中旧缓存便永久可读 |

这些是设计维度，不在本文增加另一套响应字段。用途要求“当前”时，时效证明不足便不能把结果
当作当前事实交付；允许读取已声明的旧观察，也必须由消费合同明确选择，不能自动降级。
查询范围级的依据必须覆盖零命中和分页场景，仅在命中上附带观察时间不够。
在公开合同能表达这些差异之前，保持 `RETRIEVAL.md` 的 complete 与失败关闭要求。

---

## 3. 绑定与运行时边界

### 3.1 Bound State 声明在 Schema 上

目标概念模型：

```text
Snapshot Aspect = Identity × Schema × AccessHints × Snapshot value
Bound State     = Domain Schema(origin, entity, aspect, fields)
Access          = origin × object_id  →  Aspect value
```

- Snapshot：值就在固定 Repository commit 中。
- Bound State：Schema frontmatter 的 `origin` 是访问路径；实体 `object_id` 是约定坐标；返回该 Schema 命名的 Aspect。不另存 `null` 实例文件。
- Stream Binding：句柄返回按 cursor/window 组织的记录（声明仍在 Schema；普通 READ 不隐式数组化）。

`schema_ref` 描述解析后的业务值。业务字段仍不含 cursor、凭证或源库地址。`origin` 不是 `rowCount` 这类业务字段，Canonical 文件写在 schema/* 的 frontmatter。

访问声明需要让运行方知道“观察哪个实体、按什么业务结构解释”。协议坐标就是实体 ID。
它不携带实际连接秘密，也不把当前值混入声明。可执行形状由
[Binding 类型](../knowledge/binding.go)、[观察类型](../knowledge/observation.go)及
[Serving 合同](../knowledge/serving/README.md)拥有。新增形状必须先满足这些语义，再由 Conformance
验证，不能把概念示意复制成另一套协议。

### 3.2 ResourceDescriptor 是操作包装

`kc access` 不需要实例 Binding 文件。需要带输入的操作时，ResourceDescriptor 是独立知识对象，给 `kc invoke`。运行方必须能从固定 Repository commit 得到 Schema origin 或 Descriptor 声明。

### 3.3 Materialization Runtime 在底座之外

实时状态和 Stream 由更上层产品实现，包括：

- lookup、search、window、delta、subscribe；
- cursor/watermark、retention、late data、回放和当前态 Fold；
- projection controller、调度、checkpoint、reconcile；
- 源侧认证、凭证、限流、健康状态和调用 trace。

因此底座不再定义 `repository.Stream`、Writer `APPEND`、Workspace `AppendCuts` 或 Stream Adapter。Catalog 只固定 Repository commit；动态 observation cut 由上层 Retrieval 请求创建和持有。

### 3.4 Dynamic lane 是 Repository-bound、非 Repository-owned

动态 lane 与 Repository 的关系是：

```text
belongs-to       repository identity
compiled-from    declaration commit + binding digest
keyed-by         Knowledge Address or ResourceRef
observed-through fixed binding generation
```

它借 Repository 获得身份、Schema、声明版本和 Workspace 可见范围，但运行值不是 Repository 内容，也不是 Workspace 成员。

完全无 Repository 关联的资源搜索可以由上层产品提供，但它不自动获得 Knowledge Workspace 的身份、授权和完整性语义。

### 3.5 Stream 是记录集合，不是 Aspect 数组

Stream Binding 的 Schema 描述单条记录。上层产品可以维护两种不同投影：

```text
external stream
  ├── Event Projection          回答历史/window 问题
  └── Current-State Projection  通过 Fold 回答当前状态
```

候选必须保留 event identity、order、event time、observation basis 和 continuation。不能用一个无界 JSON 数组或一份含糊的“列表索引”替代这些语义。

### 3.6 Observer 通知变化，平台拉取

默认采用 invalidate-and-pull：

```text
Observer 发出 source changed
  → upper-layer controller 合并与调度
  → 按固定 Binding lookup/delta/enumerate
  → 更新或重建可丢投影
```

Observer 不写 OpenSearch 等物理索引。它只报告 Binding、Address、source identity 或 scope 的变化；访问能力由接入方写在 Domain Schema / ResourceDescriptor。公开名称见 `TERMINOLOGY.md`。

source key 到 Address 的映射仍属于 integration/scene。新实体需要先经 Collector 用 COMMIT 建立知识身份；否则只能作为外部 ResourceRef 返回。

### 3.7 Agent 看语义元数据，不看秘密

Agent 与 Planner 应看到 Aspect、Schema、ValueSource、逻辑访问面、freshness、retention、coverage 和 hydrate 语义。Schema 的访问面只表达 `text/filter/sort`；provider 和物理索引参数属于运行时。

watermark、lag、availability、last error 和 active generation 是运行可观测状态，不应高频 COMMIT。凭证、源库内部拓扑和未脱敏 payload 不暴露。`resource-access` 原点随 Schema 版本化，换机房就发新 Schema commit，不改 Server 配置。

---

## 4. 物化与检索代数

### 4.1 载体与 basis

```text
ValueSource ::=
    Snapshot(repository, commit)
  | Binding(State, bindingGeneration, observationBasis)
  | Binding(Stream, bindingGeneration, cursorOrWindow)

Basis ::= Commit | ObservationBasis | StreamCut
```

| 形态 | 基数 | 权威读取 |
|---|---:|---|
| Snapshot | 每 Address 0..1 | READ @ commit |
| Bound State | 每 Address 0..1 | Binding lookup @ observation basis |
| Bound Stream | 每 Address/Resource 0..N | Binding window/lookup @ stream cut |

概念上：

```text
Stream ── Integrate/Fold ──→ State
State  ── Differentiate  ──→ Change Stream
Observe(value, basis)       → 可解释的一次动态读取
```

代数统一的是规划和增量维护，不是权威归属。只有 Snapshot 属于 Knowledge Catalog 的 Canonical Store。

### 4.2 能力

```text
Source access  Lookup | Scan | Search | Delta | Subscribe | Window
Transform      Project | Filter | Fold | Window | Union
Retrieval      Locate | Rank | Hydrate
```

只有 lookup 的 Binding 只能刷新已知对象，不能证明全局发现完整；search 可以下推；delta/enumerate 使 managed projection 能追赶和重建。

clause 级 Probe（Exact / Superset / Approximate / Unsupported）与 completeness 由 [`RETRIEVAL.md`](RETRIEVAL.md) 拥有。coverage 与 freshness 不能从 guarantee、索引存在或 invalidation 成功推断。

### 4.3 等价关系

上层产品的 Conformance 应优先验证语义而不是物理引擎：

```text
Build(X ⊕ ΔX) = Apply(Build(X), IndexDelta(X, ΔX))

Events(≤k₂) = Events(≤k₁) ⊕ Events(k₁,k₂]

Fold(Events(≤k₂)) = Fold(Fold(Events(≤k₁)), Events(k₁,k₂])

Refresh(binding, key, sourceRevision) 是幂等的
```

这些不进入 Repository Conformance；它们属于 Materialization/Retrieval 产品契约。

---

## 5. State exact READ 与托管物化

SEARCH 代数、AccessSpec 与 RetrievalPlan 见 [`RETRIEVAL.md`](RETRIEVAL.md)。本文只拥有 Binding / Observation 与 Serving State。

消费侧精确 READ 必须先在固定 Workspace commit 解析 Binding，再由注入的窄 `StateLookup`
端口返回 value + `ObservationBasis`。没有 runtime、observation basis 不合法或遇到不支持的
Stream 访问时失败关闭；具体 Binding adapter、Serving State 和源 runtime 不属于仓库根核心。

Snapshot SEARCH 命中后的正文回读必须复用同一 State hydrate，不能让 `READ` 与 `SEARCH`
对同一 basis 分别返回 live 值和占位。动态字段的候选发现、`SearchView`、continuation 见 [`RETRIEVAL.md`](RETRIEVAL.md)；同 revision hydrate 的控制算法由 `PROJECTION_CONTROLLER.md` 拥有；当前支持范围只查
`MVP_ACCEPTANCE.md` / `TEST_CATALOG.md`。

动态 Binding 的源访问放在下层 Materialization Runtime；Controller/Index 只看已经解析、校验和规范化的
文档，不 import Binding adapter：

```text
change notice(binding/key/sourceRevision)
  → controller 合并、去重、按固定 binding generation lookup
  → Schema 校验并写 Serving State
  → Apply(upsert/delete) 到动态投影
  → 发布新的 active observation basis
```

Serving State 保存完整观察及其可重读依据和有效性；索引只保存 AccessSpec 声明的检索字段。SEARCH 从索引取得 CandidateRef 后，在同一 observation basis 从
Serving State hydrate，不需要为每个 hit 再调用外部源，也不能直接把索引载荷当权威正文。

Serving State 是某个 generation/basis 上可重读的完整观察物化，不自动成为业务源权威，也不是
Knowledge Catalog Canonical。它可以由源查询、事件 Fold 或 CDC 构建；能否重建必须按来源能力
判断。最新当前态能够重新取值，不代表旧观察能够重建；不可再生部分按 §6.3 的观察记录治理。

单个任务或 Address 变化只刷新一个 key。以下情况才全量重建动态投影：Binding generation、
Schema/AccessSpec、解析算法或 physical revision 改变；checkpoint 断档；reconcile 发现无法安全
增量修复；首次建立投影。

invalidation 只是低延迟提示，不能证明完整。查询覆盖与新鲜度按 §2.6 分别证明，再由消费要求
决定是否可以交付。仅有成功 observation 和同 basis hydrate 不能推出外部源没有更新。
默认时效、可接受滞后与过期响应仍需选定；不得把 complete 宣传为实时保证，也不得借此解除
下列恢复与时效要求：

```text
invalidate → lookup 的实时路径
+ delta(since checkpoint) 或 enumerate/checkpoint 的恢复路径
+ 周期 reconcile 或有界 TTL 的兜底
```

上层首版只应冻结 State 当前态，不冻结 Stream event/window 查询。跨 Serving State 与物理索引无法
原子提交时，controller 必须先写 basis-addressable Serving State 和投影，再切换 active observation basis；查询
发现候选依据与观察依据不一致时必须失败关闭，不能拼接两个版本或降级为 partial。


## 6. 取值路径、观察记录与恢复/时效要求

### 6.1 新观察按声明向运行时取值，同依据重读使用观察记录

新的动态观察只能经**声明上的取值入口**取得。变更信号（消息、回调、轮询）只承担**发现**，不承担取值：

- 信号可以丢失、重复或乱序；平台收到信号后仍必须按固定 Binding 重新取值。
- 因此不存在"把消息里的值直接当知识"的路径：它既没有可核对的声明依据，也无法在信号丢失后恢复。
- 恢复与时效由三条共同保证，缺一条都不得声明完整：

```text
invalidate → 取值（实时路径）
+ delta(since checkpoint) 或 enumerate/checkpoint（恢复路径）
+ 周期 reconcile 或有界 TTL（兜底）
```

只有取值入口、没有恢复路径的来源，只能证明实际完成的那次观察，不能自动获得有界新鲜度。
只有变更信号而没有取值入口的来源，应当由接入方运行时先物化出可取值形态；不在 State 接入中
隐式承担通用消息折叠。业务源没有原生按 key 接口并非禁止接入，但其 runtime 必须履行取值合同。

查询命中、分页和历史复核要求重读已选观察时，使用该依据下保留的完整观察，或来源可证明的
同依据重读。此时不能重新取 latest 来替代旧值。统一的是声明、依据与失败语义，不是要求所有
消费请求都访问源系统；授权检查仍按 `PERMISSIONS.md` 执行。

### 6.2 重读能力由接入方声明，平台只如实转述

声明 commit 不固定动态值。来源能力和平台保留能力必须分别说明，平台不得替来源作出更强承诺：

| 来源声明的能力 | 仅凭来源能力可以承诺 | 仅凭来源能力不得宣称 |
|---|---|---|
| `repeatable`：来源支持 as-of / MVCC / 可冻结水位 | 同一 basis 可重读 | —— |
| `bounded`：来源明确声明能力界限 | 仅承诺来源实际支持的界限；有历史读取与保留保证时才承诺窗口内重读 | 从单调 revision/watermark 推导历史重读或实时保证 |
| `latest-only`：来源只有调用时的当前值 | 只保证这一次观察当时的取值 | 未来还能重读该值 |

观察时刻不等于来源位置：前者是"我什么时候看的"，后者是"我看到的是来源里的哪一点"。
缺少 `sourceRevision`/`watermark` 的来源不能靠观察时刻补位。

### 6.3 观察记录由平台提供，属于证据而不是事实

统一接入平台应当提供**统一的观察记录**：在统一坐标下保存"何时、按哪个声明、以什么一致性观察到了什么"。
它使只有 `latest-only` 来源的接入方也获得有界重读，并让下游的 API 与 basis 语义不因接入方能力不同而分叉。

边界与性质：

- 它记录的是**观察**，不是事实：来源当时错误或观察撞上竞态时，记录忠实反映平台看到了什么。
  它不成为第二份权威，也不得对外宣称持有来源侧的真相。
- 它**不是可丢派生**。介质角色与丢失后果见 [`STORE_ADAPTERS.md`](STORE_ADAPTERS.md)：
  来源是 `latest-only` 时，历史观察一旦丢失即永久丢失，"投影可删除、可重建"对它不适用。
- 它由上层 Materialization 产品承担，**不进入 Repository、Writer 或 Catalog pin**（§8.2 已定边界），
  也不因持久化而成为 `snapshot.Store`。
- 保留期由接入方声明的业务需要决定，由平台设上限并强制；超限应显式拒绝，而不是静默截断。
- 观察失败、成功确认空值与成功取值是三种不同结果，不得合并（判定规则见
  [`PROJECTION_CONTROLLER.md`](PROJECTION_CONTROLLER.md) §4.4）。

活动 Serving State 解决本次查询如何一致交付；观察记录解决更新或重启以后如何复核旧观察。
保留一个 active revision、计算其摘要或把值写入临时目录，都不等于履行历史保留承诺。
需要保留的完整观察与依据应先可靠保存，再发布依赖它的结果。索引重建可以重用保留记录，
不能要求 latest-only 来源重新提供已经消失的值。

观察记录不等于连续源日志：两次观察之间的变化可能从未被看到，不能据此推导任意时刻的状态。
保留期内丢失记录是恢复故障；按合同到期是生命周期结束；两者都不能返回 latest、空值或成功的
空历史来掩盖。清理还须遵守已承诺的查询/重读生命周期；超过该生命周期的请求明确不可重读。
发布、恢复、配额和清理的具体接口由上层运行时合同选定，不新增 Catalog 事务。

### 6.4 固定声明的服务生命周期

Dataset 服务版固定声明 commit，不会因 Repository HEAD 前进而自动改用新 Schema 或 origin。
只要服务版仍承诺动态消费，其固定声明就必须作为维护需求被跟踪：独立刷新、恢复、解释时效，
不能只有 HEAD 对应的声明得到维护。服务版发布成功也不等于外部状态已经就绪。

固定旧声明不意味着旧 runtime 永久在线。接入方退役 generation 时，需要明确仍在服务的消费
依赖，以及继续服务、显式迁移或停止该动态能力的处理。不能悄悄切换到 HEAD 的新 Binding；
旧声明或同依据观察不可用时明确失败。声明保留期、runtime 服务期和观察保留期是三种生命周期。

---

## 7. 调研结论

这套方向既能在成熟基础设施和数据目录产品中找到对应模式，也有流关系、增量视图、联邦查询与
分布式检索的理论基础。业界没有把所有“非稳定信息”收进一种通用 Store；共同做法是按权威、
查询、恢复和 retention 责任拆开，再用稳定 identity、schema 和 basis 连接。

### 7.1 产品与基础设施模式

| 系统 | 处理方式 | 对本项目的启示 |
|---|---|---|
| [Kubernetes Objects](https://kubernetes.io/docs/concepts/overview/working-with-objects/) / [API watch](https://kubernetes.io/docs/reference/using-api/api-concepts/) | `spec` 表达声明态，`status` 表达控制器观察的当前态；客户端 list 后从 `resourceVersion` watch，旧 revision 被压缩后必须重新 list | 声明与观察分开；通知历史有限，完整恢复依赖 enumerate/reconcile |
| [Kubernetes Events](https://kubernetes.io/docs/reference/kubernetes-api/events/) | Event 有限保留，官方定义为 informative、best-effort、supplemental | 运行事件不能自动充当审计或知识真相 |
| [Apache Kafka Log Compaction](https://kafka.apache.org/43/design/design/) | 事件按 partition/offset 排序并按时间/大小保留；compaction 保留每个 key 的最后状态，但旧记录和 tombstone 仍会清理 | Event Log、Current State 与永久审计是三种不同承诺 |
| [Apache Flink Checkpointing](https://nightlies.apache.org/flink/flink-docs-stable/docs/dev/datastream/fault-tolerance/checkpointing/) / [Checkpoint vs Savepoint](https://nightlies.apache.org/flink/flink-docs-stable/docs/ops/state/checkpoints_vs_savepoints/) | checkpoint 将 operator state 与输入位置一起保存用于故障恢复；savepoint 才是用户管理、面向迁移的运行状态镜像 | 动态运行时可以做 snapshot，但其所有权、生命周期和语义不等于 Knowledge Snapshot |
| [Materialize Views](https://materialize.com/docs/concepts/views/) | materialized view 持久并增量更新结果，index 在集群内存中服务查询 | Serving State 与查询索引分层；索引可丢，物化状态按自己的恢复模型管理 |
| [Prometheus Storage](https://prometheus.io/docs/prometheus/latest/storage/) | 指标按时间块、WAL 和 retention 管理，可转发到远端时序存储 | metric/health/lag 属于时序可观测数据，不应高频 COMMIT 成知识 |
| [DataHub Metadata Model](https://github.com/datahub-project/datahub/blob/master/docs/modeling/metadata-model.md) | Versioned Aspect 存关系库；高频 Timeseries Aspect 直接进 Kafka/Elasticsearch，且文档明确提示其灾备恢复更困难 | 数据目录也拆稳定与时序 lane；但搜索索引不应因此被提升为 Canonical |

横向结论：

1. durable 不等于 Canonical。Kafka log、Flink checkpoint、TSDB block 都可以持久，但不因此成为知识仓；
2. 当前态、事件历史、运行恢复和可观测数据应分别声明 retention、顺序和恢复承诺；
3. subscribe/watch/invalidation 只降低延迟，正确性仍依赖 revision、relist/delta、checkpoint 和 reconcile；
4. 每次动态读取必须如实解释观察依据与来源能力；不能用 bounded/latest-only 标签代替时效或重读证明，partial 的适用边界仍由检索合同决定；
5. 动态观察只有经明确选择、汇总和 provenance 捕获后，才由 Collector COMMIT 晋升为知识。

### 7.2 数据集成与能力驱动改写

- Halevy 的 [Theory of Answering Queries Using Views](https://homepages.inf.ed.ac.uk/libkin/dbtheory/alon.pdf) 说明怎样用已声明的 view/source 回答统一查询。
- IBM Garlic 的 [Capabilities-Based Query Rewriting](https://research.ibm.com/publications/capabilities-based-query-rewriting-in-mediator-systems) 让源声明能力，由 mediator 生成可执行子查询和组合计划。
- Garlic/DB2 的 [Federated Query Processing](https://research.ibm.com/publications/garlic-a-new-flavor-of-federated-query-processing-for-db2) 证明 Catalog 元数据、源模块与联邦优化器可以协同。
- Li 的 [Limited Access Patterns](https://ics.uci.edu/~chenli/pub/jvldb03.pdf) 说明只有受限 lookup 时，并非所有查询都能得到完整答案。

结论：capability checking、pushdown 和 cost-based routing 很成熟；任意外部协议与任意查询的完全自动改写不成立。

### 7.3 Stream–Relation 与增量视图

- [CQL](https://web.stanford.edu/class/cs245/readings/cql.pdf) 区分 stream、relation、window 和 relation-to-stream 输出。
- [DBToaster](https://vldb.org/pvldb/vol5/p968_yanifahmad_vldb2012.pdf) 研究高频动态视图的高阶 delta 维护。
- [Differential Dataflow](https://www.cidrdb.org/cidr2013/Papers/CIDR13_Paper111.pdf) 用逻辑时间维护变化中的计算。
- [DBSP](https://www.vldb.org/pvldb/vol16/p1601-budiu.pdf) 用 differentiation/integration 给出通用 IVM 代数。

结论：Stream、当前态、window/cut 和增量维护有成熟理论；这支持上层产品实现 Materialization Runtime，不构成把 Stream 塞进 Repository 的理由。

### 7.4 联邦检索

[Searching Distributed Collections with Inference Networks](https://sigir.org/wp-content/uploads/2017/06/p160.pdf) 已把分布式检索拆成 collection representation、selection 和 result merging。

结论：自动选源与候选合并成熟；异构相关性分数仍不天然可比。

### 7.5 成熟度判断

| 能力 | 判断 |
|---|---|
| Binding capabilities → 合法 pushdown | 成熟，可借鉴 |
| State/Stream/window 时间语义 | 成熟，可交给专门运行时 |
| 增量维护 filter/join/aggregate/fold | 成熟，有理论与系统实现 |
| passthrough 与 managed projection 自动选路 | 可行，需要 stats/SLA/成本策略 |
| invalidation-only 保证完整 | 不成立 |
| 任意外部源完全自动改写 | 不成立 |
| 异构检索 score 统一标尺 | 不成熟，不应伪造 |
| Knowledge Address + Binding basis + Agent 元数据 | 没有现成标准，是本项目设计空间 |

---

## 8. 已定边界与待冻结问题

### 8.1 声明、观察与检索的交接

一次消费必须先确定知识声明，再确定外部观察的依据。仅解析 Binding 不应产生源访问；精确
读取和检索命中交付则通过消费侧窄端口得到完整值及其观察依据。Repository、Writer 与 Catalog
仍不拥有外部运行时。

State 精确读取与动态检索是同一声明语义的两种使用方式：前者按已知身份观察，后者先定位候选
再按该次检索固定的观察依据交付。独立的后续读取可能获得更新观察，不能倒过来替换原查询证据。
动态检索的规划归 [检索设计](RETRIEVAL.md)，索引发布与同版本回读归
[投影控制设计](PROJECTION_CONTROLLER.md)。

观察若需要晋升为知识，必须由接入方显式选择内容、保留来源与当时的观察依据，再经 Writer
形成可治理版本。仅能取得调用时最新值时，应如实保留这一次观察，不能许诺未来还能重读源值。

Stream 的窗口、分区进度、历史重放和保留策略需要额外设计；不能借 State 读取路径默认获得。
具体 Binding、观察与结果结构由公开类型及包文档维护；参考实现与未验收范围由
[MVP 验收](MVP_ACCEPTANCE.md)与[验证体系](TEST_CATALOG.md)记录。

### 8.2 已定边界

已定：

- 底座的权威 Store 只有 Snapshot；
- “底座不存动态值”不禁止外部运行时制作 checkpoint/WAL；运行恢复 snapshot 不是 Knowledge Snapshot；
- Aspect 可声明 Snapshot、State Binding 或 Stream Binding；
- State/Stream 是 Binding 的逻辑访问形态，不是物理介质分类；当前态、事件日志、时序观测、checkpoint 与查询投影仍分别治理；
- 值来源随知识单元声明版本化，运行观察还需独立依据；磁盘位置与序列化形状由公开知识协议拥有；
- inline Binding 与同 commit 的 ResourceDescriptor reference 同时支持，且二者互斥；
- Binding 声明属于 ②，运行值属于墙外 Materialization Runtime；
- 消费侧 `knowledge/serving` 可以通过注入端口看到 State 运行值，但不拥有 runtime/provider；
- Catalog 只固定 Repository commit，不固定动态 cursor/watermark；
- Writer 不提供 APPEND；动态值若要沉淀，Collector 显式 COMMIT Snapshot；
- Stream Schema 描述单条记录，不是数组；
- Observer 通知变化，平台按 Binding 拉取；
- Projection 非权威，命中后按 typed reference hydrate；
- Schema 只声明 `text/filter/sort`；SEARCH 代数、Probe、RetrievalPlan 见 `RETRIEVAL.md`；
- State 动态首版采用 invalidate-and-pull + basis-addressable Serving State + 动态 State 投影，并补齐后台恢复与有界观察保留；控制语义与验收见 `PROJECTION_CONTROLLER.md`，不进入 Repository/Writer/Catalog；
- 调用方信封是否含全文见 `PERMISSIONS.md` 交付链首段。

### 8.3 后续需选定的合同

State 下一阶段需要选定默认时效、过期响应、观察保留与容量上限、活动消费依赖的登记/退出，以及
源授权或显式共享观察的装配合同。方向已在上文确定，公开形状与可执行证据不能由本文代写。
以下问题留给后续 Stream、容错或规模设计：

1. 旧 runtime generation 的长期托管、迁移与服务期限；固定声明不可偷换的边界已在 §6.4 选定；
2. ObservationCut 对分区水位、cursor/window 和部分序的具体表达；
3. Stream retention、tombstone、compaction 与 gap 后 relist/reset 的责任协议；
4. Stream Event Projection 与 Current-State Fold 的声明位置；
5. passthrough 与 managed projection 的成本、authority role 和 retention 模型；
6. stale/removed 在流式事件和 retryable error 之间的具体编码；
7. 多副本 controller、worker lease、durable queue 与大规模历史查询；State 的有界观察保留不等这些能力才设计。

## 9. 用例

本节描述使用者要完成的任务与可观察边界，不是实现完成清单，也不选定命令、字段或错误码。
U1–U8 属于 State 下一阶段；U9 描述后续 Stream 方向。具体测试、局部证据与缺口由验证体系和
场景视图维护；有方向性用例不代表已有可执行入口。

### U1：查询当前失败的服务，包括零命中

运维人员在已授权 Dataset 中查询“当前失败的服务”。即使命中为零，也应知道本次检查的声明
范围、观察覆盖和时效是否足以支持“当前没有失败”的判断。源值已经变化但通知丢失时，后台
恢复应在承诺范围内追上；不能证明足够新时，不能把旧观察上的零命中当作当前结论。消费查询
不触发同步重建，分页也不能随后台更新混入另一观察依据。

### U2：区分业务空值、取值失败与没有取值能力

接入方发布状态声明后，消费方读取一个已知任务。成功确认没有状态、runtime 暂时故障、以及
部署根本没有取值能力，应能被区分。只有成功观察才能支持业务缺失判断；失败不能制造空值或
清空检索字段，缺能力不能转读通知正文或扫描补齐。精确读取同时保留声明与观察依据。

### U3：通知丢失、重复或乱序后仍能恢复

接入方持续改变任务状态，部分通知丢失、重复、乱序，随后 KC 重启。后台应按固定声明重新取值，
从实际可用的对账或来源进度恢复，并如实暴露尚未恢复的范围；不能以“曾经就绪”或队列已清空
证明追平。通知不直接携带权威值，恢复不改变 Repository commit 或文件视图。
来源提供顺序依据时应防止旧依据覆盖新依据；没有顺序证明时如实说明能力，不能虚构来源的全局单调进度。

### U4：来源只能给最新值，仍能复核已交付的观察

分析人员保存一次失败状态的观察依据；源状态随后恢复正常，服务也发生重启。在约定保留期内，
复核应取得当时实际观察的失败值，并能解释当时的声明；它不保证两次观察之间的完整历史。
保留期内丢失记录应报告恢复故障，到期则明确不可重读，两者都不能返回当前正常值或空历史。

### U5：HEAD 前进后，已发布 Dataset 的状态仍按原声明服务

接入方修改 Schema 或 origin 并推进 HEAD，消费方继续使用尚未退役的 Dataset 服务版。动态维护
仍跟踪该服务版的固定声明，不因 HEAD 已更新而停止对账，也不混用新声明。旧 runtime 下线时，
使用者应看到明确的迁移或不可用结果；不能把“固定知识版本”误解为冻结了源当前值或永久运行保障。

### U6：后台能看到的状态，不自动成为所有消费者可见的状态

后台服务身份可以观察某资源，但消费方未获源访问权。若该接入要求逐调用方授权，精确读取、
搜索、分页与历史观察都不能借后台记录绕过当前授权；撤权后旧 pin 与旧记录不能放行。若接入方
有权且明确发布共享观察，则按该共享范围交付。两种授权来源必须显式区分，不能默认互相替代。

### U7：访问地址变化不扩大凭证的使用范围

接入方修改声明中的 origin，消费方再次取值。只有受信任且符合凭证适用范围的目标才能接收该
证明；声明可写不等于有权索取消费方凭证。后台对账使用按来源信任边界配置的服务身份，不能
借用最近一次通知或用户请求的身份。拒绝访问时不把凭证写入知识、索引或诊断正文。

### U8：大量状态中只变化一个，维护成本与故障影响有界

接入方维护大量任务，其中一个发生变化。正常增量应只重取、重算并写入受影响部分，不能虽然
只调用一次 runtime，却复制全部观察并重建整库索引。若某个任务持续取值失败，后台仍能推进
其它独立工作；查询按其实际依赖判断可用性，依赖故障部分的结果不得伪装为完整或业务缺失。

### U9：从当前状态走向有界事件窗口，再到持续订阅

排障人员先查询任务当前状态，再请求某个有限时间窗口内的事件。后续 Stream 能力应明确窗口、
顺序、来源进度、迟到数据与保留缺口；发生断档时不能把剩余记录当完整历史。持续订阅还需独立
处理恢复、背压和取消，不由窗口读取默认获得。普通 State READ 不返回无限数组，事件也不经
Writer APPEND 进入 Snapshot；若要沉淀排障结论，显式选取内容后经 Writer 形成知识。
