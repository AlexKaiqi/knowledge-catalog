# 服务化 Store

协议分层 ⓪–③ 见 [`LAYERS.md`](LAYERS.md)。本文只钉 **介质**：同一层语义用什么引擎。不要和 ⓪–③ 混成另一套「四层」。

① Catalog 成员是 **Snapshot**（git 形）。APPEND 是 **Stream**（有序段），不是仓。不要 `repo-add --driver redis` / `--driver stream` / `--driver mysql`。K-23：换引擎不改身份、版本、读写语义。

## 目标引擎（冻结）

这是 ⓪ 权威和 ③ 派生用什么引擎。本地 SQLite 可以同时扛全文和窄列，**不代表**规模化仍焊在一个引擎上。

| 层 | 目标 | 不是 | 当前能跑 |
|---|---|---|---|
| Snapshot 权威 | FileGit / Dolt / Gitea | MySQL、行库 | `local.FileGitRepository`；`scale.DoltRepository`；`gitea.Repository`（Git 对象 API，无工作区） |
| APPEND 权威 | 有序段（开段热、闭段对象存储） | SR、Iceberg、Redis | `local.JSONLStream`。`scale.OpenStream` Append 仍 stub |
| 列索引 / 过滤 / 聚合 | StarRocks | Redis | 本地 SQLite `fields`。`scale.OpenStarRocks` 仍是 stub |
| 全文 | ES / 本地 SQLite FTS | SR | `local.OpenSQLite`（FTS5）；`scale.OpenElasticsearch`（MATCH） |
| 热尾 | Redis（可丢） | 仓、GT | scale `cache: redis`。不要把 Redis 当比较引擎或权威 |

职责仍可看成介质上的权威 / 索引 / 缓存 / 投影：Snapshot+APPEND = ⓪ 权威（两种）；全文+列索引 = ③；热尾 = ③ 缓存。一层引擎可以同时承载两层介质职责，但规模化必须按上表拆开。Catalog（①）和 Aspect（②）不在这张表里。

| 职责 | 存什么 | 回答什么 | 本地方案 | 规模化 |
|---|---|---|---|---|
| **权威** | 整条 Canonical（对象 / `StreamRecord`） | `READ` / `READ_STREAM` / 回读 | FileGit；APPEND 一体 JSONL | Snapshot：Dolt（git 形）；APPEND：有序段，闭段对象存储。**不是 PG** |
| **索引** | 指针：`object_id` / `eventId` / 偏移；`key` `filter` `text` `sort` 的 posting | `SEARCH` 定位、点查、审计窗定位后再回读 | SQLite FTS5 + `fields` | 全文 ES（MATCH）；比较定位走 StarRocks，不走 Redis、不走 PG |
| **缓存** | 热拷贝（常是整包 + cursor） | 加速已知键 / 热尾 | 无 | **Redis**（可丢；miss 回权威） |
| **投影** | 窄行/列：`summary` `stored`、分析列 | 列表摘要、计数/过滤/聚合 | 同 SQLite 的窄列 | StarRocks / Iceberg（**不是**冷权威、不是仓） |

写路径只有一条：**先落权威**，再可选更新索引、投影、缓存。冷热是权威按体量换介质，不是换 Surface。Redis 永远可丢；StarRocks / Iceberg 是投影常用的列存。本机 `audit.jsonl`、登记表 git、hook outbox 不走这套梯子。

比较放列存：要 `n > 5` 需要的是**已投影的 typed 列**（和本地 SQLite 一样只抽 AccessHints）。有 StarRocks 就用 SR，不要为比较再养 PG。不要整对象进 Redis。ES 只覆盖 MATCH 等文本面。没有 Hint，就没有索引也没有投影。`permissions` 进 Canonical；GRANT 正文默认不声明 Hint，因此不进 BM25。声明了 `filter` 就和其他知识一样进 IndexPlan。

**SR 的访问口像 MySQL，职责不像。** FE 是 MySQL 协议（`jdbc:mysql://fe:9030`、`?` 占位）。这只说明列索引适配器该用 MySQL 驱动连 SR。权威不是行库：Snapshot 是 FileGit/Dolt，APPEND 是有序段。不要把 PG 或 MySQL 当仓，也不要「先换 MySQL」当通往 SR 的台阶。`--driver mysql` 已拒。用户口仍是 `SearchRequest`。协议根不长 SR connector；场景侧连 FE、选表模型、Stream Load。

当前参考实现分两套目录：

- **`local/`**：FileGit（Snapshot）+ `JSONLStream`（Stream）+ SQLite（③）。`kc init` 默认 `profile: local`。不要 Redis。
- **`gitea/`**：远程 Snapshot（Git 对象 API + `PUT /branches` CAS，无工作区）。需要 Gitea 1.26+。Token 走 `KC_GITEA_TOKEN`。
- **`scale/`**：`DoltRepository`（Snapshot 口；git 形知识文件）、`OpenStream` stub、ES、StarRocks stub、Redis 缓存。

不要 `--driver mysql`。协议根不长 Hippo SR connector。

配置按职责拆成两份，不按「本地 / 托管」互斥：

| 文件 | 写什么 | 为什么总在 |
|---|---|---|
| `.kc/layout.yaml` | 这台机器的目录 | Catalog 登记表永远是本地 FileGit；知识仓 FileGit / SQLite 投影也落这里 |
| `.kc/stores.yaml` | 用哪种引擎，以及托管 host | `profile` / `repository` / `index` / `cache`；redis / elasticsearch / starrocks 的连接（无密码） |

密钥只走环境变量。路径一律相对 `--home`（默认 `.kc`），也可写绝对路径。不要做「空字段猜本地、有 host 猜托管」。

不要拆成互相替代的两份 yaml 文件：用 `profile: local|scale` 选栈。登记表始终是本机 FileGit，目录在 `layout.catalogs/<encoded-id>`。local profile 拒绝 Redis 当 index 或 cache。

## 本地方案（FileGit + SQLite）

`kc init` 写入：

`.kc/layout.yaml`：

```yaml
repos: repos                    # 成员知识仓根：<home>/repos/<encoded-repo-id>
catalogs: catalogs              # Catalog 登记表父目录：<home>/catalogs/<encoded-catalog-id>
projections: projections        # 检索投影：<home>/projections/<encoded-repo-id>.sqlite
```

`.kc/stores.yaml`：

```yaml
profile: local
repository: filegit
index: sqlite
```

落到磁盘（`--home .kc`）：

```text
.kc/
├── layout.yaml              本机目录
├── stores.yaml              引擎（本地默认只有这两行）
├── writer.json              幂等日志（不是 store）
├── catalogs/                layout.catalogs
│   └── kr_acme_catalog/     kr://acme/catalog 登记表 git（catalog.yaml / view-*.yaml / …）
├── repos/                   layout.repos
│   └── kr_acme_public_core/ 知识仓 FileGit（git config kc.repositoryId）
└── projections/             layout.projections
    └── kr_acme_public_core.sqlite
```

```bash
kc init --home .kc --catalog acme/catalog
kc store-set --repository filegit --index sqlite
kc store-set --repos-dir repos --catalogs-dir catalogs --projections-dir projections
kc repo-add --repo kr://acme/public/core
```

换本地盘位置时只改 `layout.yaml`，例如：

```yaml
repos: /data/kc/repos
catalogs: /data/kc/catalogs
projections: /data/kc/projections
```

登记表不要 `repo-add`。有哪些 Catalog / 知识仓扫 `catalogs/` 与 `repos/`（身份在 git 里），不要 `workspace.json`。

## 规模化方案集（`scale/`）

**目标**：Dolt（Snapshot）+ 有序段（APPEND）+ ES（全文 MATCH）+ StarRocks（列索引/过滤/聚合，协议根不长 connector）+ Redis（热尾缓存）+ Iceberg（湖表消费投影）。**不要 `--driver mysql`。** `DoltRepository` 已实现 Snapshot 口（git 形知识文件；原生 Dolt SQL 未装配）。`OpenStream` Append 与 StarRocks 仍 stub。ES 全文已实现。Redis 只做 cache。

`layout.yaml` 仍要：登记表 git 在本机。

```yaml
# layout.yaml — 不变；登记表仍在本机 catalogs/<encoded-id>
repos: repos
catalogs: catalogs
projections: projections
```

```yaml
# stores.yaml — scale profile（Stream Append / SR 仍 stub；Dolt 已实现 Snapshot 口）
profile: scale
repository: dolt
index: elasticsearch
cache: redis
elasticsearch:
  url: http://127.0.0.1:9200
redis:
  host: 127.0.0.1
  port: 16379
starrocks:
  host: 127.0.0.1
  port: 9030
  user: root
  database: kc
```

密码：`KC_REDIS_PASSWORD` / `KC_ELASTICSEARCH_PASSWORD`（或 `KC_ELASTICSEARCH_API_KEY`）/ `KC_STARROCKS_PASSWORD`。Redis 段是热尾缓存连接，不是权威，也不是比较引擎。

旧的一份 `stores.yaml`（内含 `filegit:` / `sqlite:`）仍可读；下次 `kc store-set` 拆成 layout + engines 两份。

## 写完之后：③ 怎么接到 ⓪

AccessHints 已经把脸分开了：`key` / `filter` / `text` / `sort` 是**索引车道**（`IndexLane`）；`summary` / `stored` 是**投影载荷**，不是车道。同一字段可以同时进两层（例如 `access: [filter, stored]`）。

```text
COMMIT → Snapshot（git）     APPEND → Stream（JSONL / 切段）
               → Receipt（commit / cursor 已 durable）
               →（可选）③ 更新索引 / 投影 / 缓存
```

和现在 `COMMIT` 先落 git、再 `AfterSnapshot` 增量编 SQLite 是同一形状。索引、投影、缓存失败不得回滚已成功的写，也不得让调用方以为没写上。重放可以从权威再填。审计不能问索引或投影「是不是少一条」。

`stores.yaml` 仍是两个可换槽（`repository` + `index`）。那是引擎配对，不是四层合成两层：本地 `index: sqlite` 同时承载索引和窄列；Redis 只应进缓存层。

## View 不独占索引

物理索引按 **仓** 建，不按 View。键是 `repository` + 该仓 pinned commit + 该仓 `schema/*`。`IndexPlan` 只是 Generation 上的配方：列出各成员各一份，不是联邦大表，也不是「每开一个 View 建一张表」。

```text
ViewGeneration {repo → commit}
  → SEARCH 扇出到各成员已有索引
  → 查询 clause 在各仓上 AND
  → 命中后回读各仓 Canonical
  → union，不覆盖
```

多个 View 钉到同一仓的同一 commit，共用同一份物理索引（SQLite 文件 / ES index / SR 分区），零份新表。StarRocks 也是一张（或按 `repository` 分区）列投影，View 只是 `WHERE repository IN (成员) AND commit = 钉死的那版`。

不要给行打 `view_id` 再按 View 复制整列——重叠 View 会把同一对象抄多份。View 若以后要对象子集（同一仓里只看某类实体），用查询时额外 AND 的 filter，或一份薄的 `object_id` posting；不要克隆列索引。当前 `define-view` 的 selector 是 **ref → commit**，不是对象过滤器；`AspectSelector` 只裁 READ 的 Aspect。

`kc search --repo` 打的是仓上的工作投影。跨仓检索走 IndexPlan 扇出，尚未做成独立 CLI 入口。

## APPEND：⓪ 的流，不是仓

Canonical 事件在 Stream 里；APPEND 不另搞一套 Surface。对象历史是 `LOG` / `DIFF`（Snapshot commit），不是这条流。不要 `repo-add --driver stream`。`JSONLStream` 旁放 git 形 Snapshot 是落盘同居。

1. **权威**：整条 `StreamRecord`。按体量从单文件 → 切段 → 近热远冷；协议仍是 `APPEND` / `READ_STREAM`。
2. **索引**：EventID hash、审计窗 `(recordedAt, eventId, type) → 偏移`、事件 schema 上的 lanes。只定位，再回读。
3. **缓存**：Redis 只加速热尾（当前 cursor + 最近 N 条全文），可丢，先权威。miss 必须回权威。
4. **投影**：`summary` / `stored` / 分析列。StarRocks / Iceberg 是投影实现，不是冷权威。

`.kc/audit.jsonl`、登记表 git、hook outbox、Writer 幂等日志不走这套梯子。

### 给定 continue / lookup，权威用什么

用户只问接续和点查。选 store 就看这两问，不要为 window / search 先换引擎。

| 体量 | 权威（有序段） | 点查 | 热 | 冷 |
|---|---|---|---|---|
| 小 | 一个 JSONL | 扫文件 | 就是这文件 | 没有 |
| 中 | 按尺寸/日切段；清单记 `(fromCursor, toCursor, 路径)` | 扫近段；慢了再加可重建的 `eventId → 段+偏移` | 最近几段本地 | 仍可全在本地盘 |
| 大 | 同一切段；近段快盘，闭段对象存储 | 必须有 `eventId → 段+偏移`（索引层，可从权威重建） | Redis 只缓存热尾全文 + 当前 cursor | 闭段仍按 cursor 接续，只是慢 |

**合适**

- 本地默认：FileGit 旁的 JSONL（已经是这样）。文件大了再切段，不要先上 Redis。
- 热尾加速：Redis 只在「消费者几乎都在头附近」时才加；miss 回段文件。

**不合适当这条流的权威**

| 引擎 | 原因 |
|---|---|
| Redis | 缓存，不是仓；不能只写 Redis |
| Iceberg / 分析型列存 | 投影，不是按 cursor 接续的 log |
| Elasticsearch | 检索面，无序、非 GT |
| git / Dolt | Snapshot；APPEND 本来就不进 tree |
| 行库整包 blob | 把整条流当一个 JSON 数组 |

冷段用按 cursor 范围命名的 JSONL（或仍按写入序的对象）即可。清单是权威的一部分，用来让 continue 找到下一段，不是用户口。

### 生产：写只有 APPEND 时，权威就是有序段

访问场景可以后定。只追加、不改、不删（修正再 append），物理形态已经是 **log segment**：

```text
正在写的开段     顺序追加（块存储 / 日志服务）
已经闭的段       不可变，进对象存储
清单             (fromCursor, toCursor, uri)
```

本地 JSONL 就是这个形状的单机版。生产不要改成「行存里改一条」；把开段放到托管盘或日志系统上，闭段放到对象存储。热 = 还在写、还没归档；冷 = 闭段。同一条 cursor 序。

这类存储（按贴合程度）：

| 合适程度 | 是什么 | 备注 |
|---|---|---|
| 本义 | 顺序段文件 / 对象 + 开段在块存储 | 和本地 JSONL 同一形状，只是托管 |
| 本义 | Kafka / Pulsar / Kinesis 这类 log | 就是开段+切段+可分层到对象存储；多一个系统 |
| 能用（不选） | 行库 **追加表**（分区，不 update） | 行库在模拟 log；不是目标权威 |
| 不合适 | 整段 JSON blob、Redis 当仓、Iceberg 当冷 log、把行库当 Snapshot 仓 | 整包重写 / 缓存 / 列存分析 / 行库冒充 git |

不必为了「最近要快」先上 Redis：开段留在快介质上就是热路径。Redis 仍是可丢缓存。访问面在这段 log 上长，不反过来选仓。

### 消费：可以另给一张表（Iceberg 和/或 StarRocks）

写仍只进有序段。用户要「能查到、能用引擎扫」，再提供**消费/列投影**存储。系统已有 StarRocks 时：

- **列索引 / 过滤 / 聚合**：SR 比 Postgres 合适（这是规模化 SQLite `fields` 的位置）。
- **开放湖表**：仍可提供 Iceberg；SR 可以直接扫 Iceberg，不必为消费再抄一份进 PG。
- APPEND **禁止**直写 SR / Iceberg。异步从闭段或钉死的 cursor 编；落后不影响 Receipt。

本地小流不必上 SR。命中后回读权威。SR 不是知识仓，也不是 log。

### 写：权威先成功

```text
APPEND → 仓的 Append 实现（JSONL / 切段 / 对象上的有序段）
      → Receipt（cursor 已 durable）
      →（可选）更新索引
      →（可选）更新投影
      →（可选）填热尾缓存
```

这和 `COMMIT` 先落 git、再 `AfterSnapshot` 增量索引是同一形状。索引、缓存或投影失败不得回滚已成功的 APPEND，也不得让调用方以为没写上。重放可以从权威再填。

不能出现「只写了 Redis」或「Redis 与权威不一致时信 Redis」。miss 必须回权威。

当前参考实现：`Writer.Append` 落到 `Repository.Append` 后出 Receipt；还没有 `AfterAppend`。FileGit 是 `streams/<ref>.jsonl`。SNAPSHOT 路径有 `AfterSnapshot`（CLI 把 `index.Index` 挂上 Catalog.Hook），把索引和 filter 列焊在同一份 SQLite 里。

### 冷热：同一份权威，按体量换介质

冷热切的是**同一条记录放在哪档介质上**，不是换语义。

| 体量 | 权威长什么样 | Redis |
|---|---|---|
| 小 | 一个文件就是热，也是全部 | 不必有 |
| 中 | 按日或按尺寸切段；最近几段本地 | 仍可无 |
| 大 | 近段快介质，旧段对象存储 | 只缓存热尾（cursor + 最近 N 条） |

旧段进对象存储之后，它仍是权威的一部分，只是读得更慢。冷权威可以是按日 JSONL/Parquet 对象，按 cursor 还能接续、按写入日还能裁审计窗。Iceberg **不是**冷权威的默认形态。

小流不要强行拆冷热。文件扫得动，多一个 Redis 只增加不一致窗口。

### 「存什么」：按访问活决定派生

权威里永远是整条记录：`eventId`、payload、digest、两套时间（`observedAt` / `recordedAt`）、`schemaRef`（`eventType` 若有则落信封，不靠扫 payload）。派生层**禁止默认整包拷贝**。没有 Hint，就没有分析投影——不要把整段 JSON 灌进 FTS 或 Iceberg。

| 活 | 读什么 | 进哪一层 |
|---|---|---|
| 接续 / 热尾 | `fromCursor` + 整包 payload | 权威；体量大才**缓存**最近 N 条全文 |
| 审计窗 | 时间范围内完备记录 | 权威（按写入日切段）；体量大才加**索引** `(recordedAt, eventId, type) → 偏移` |
| EventID 点查 | 一条记录 | 权威；体量大才加**索引** hash |
| SEARCH / 过滤定位 | 哪些 ID 命中 | **索引**（lanes）；命中后回读权威 |
| 分析（计数、过滤、聚合） | Hints 声明过的列 | **投影**（SQLite / StarRocks / Iceberg）；不存整段 payload |
| 列表摘要 | `summary` / `stored` | **投影**窄行；点开仍回读权威 |

三种活可以叠在一条流上，但各层失败隔离：索引落后只让 SEARCH 旧；投影落后不影响 cursor；缓存 miss 回权威；审计不能问索引或投影「是不是少一条」。`AppendCuts` 已经是 Generation 上流的钉（对标 snapshot 的 pinned commit）；索引和投影的 basis 应该钉 cut，而不是钉 `latest`。参考实现里 `ViewReadVersion.AppendCuts` 有类型，Reader 尚未组装。

### 用户只看见访问语义

先做流的本义，别的后做。调用方不点名介质。

```text
P0  continue   fromCursor + limit    整包接续；和写侧同一个不透明 cursor
P1  lookup     eventId               一条或 UNRESOLVED
后  window / search / cut            审计窗、事件检索、Generation 钉流端
```

`kc stream --from-cursor --limit` / `--event-id`。无界倒出只是小流调试。`window` 代码能跑，不是优先口；`search` 与 `cut` 仍是 `CAPABILITY_UNSATISFIED`。adapter 整段载入是实现债。
