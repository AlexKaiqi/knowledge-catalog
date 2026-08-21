# Connector：入站镜像（外部权威）

日期：2026-08-20  
范围：源系统才是领域权威时，如何感知变更、拉取当前态、译成 ChangeSet。不是出站 hook，不是 gate，不是第四种 Write Surface。  
对照：DataHub MetadataChangeProposal + 独立 ingestion、Google Knowledge Catalog Entry 同步（权威仍在 BigQuery / IAM）、Unity 变更流给外部发现目录。  
前置：`KNOWLEDGE_CATALOG_DESIGN.md`（F3、F4、K-21、第 12 章 A）；`HOOKS.md`（方向相反）；`GATES.md`（`put`/`commit` 不设 gate）；`writer/README.md`。

参考实现提供 `connector/`：**Address 级、带 Scope 的对账预览**。无 `kc connector-*`，无插件宿主。Hive / Ranger 客户端不进本仓库根。

---

## 0. 主张

有一类知识真正权威在外部系统（metastore DDL、作业定义、特权库 GRANT）。Catalog 不是第二份 SoR，而是：

```text
感知「可能变了」→ GET 源上当前完整状态 → 译成 Address 全量值
→ connector.Preview（patch 或 reconcile）→ Writer COMMIT（origin_kind=SOURCE）
```

仓里留下的是「某次 `producedAt` 我们看见源长这样」，带 digest 与来源信封。读者仍钉 Generation / Release，不跟源的 `latest`。lag 是常态。

Connector 是 **入站、独立进程**（甚至独立仓库）：对方调我们的 Writer。Hook 是 **出站**：我们写完去调对方。不要合成一种东西。

```text
allow       → 谁能调用 commit / put / append
connector   → 源变了，对方来提交 ChangeSet
hook        → 我们在动词 pre/post 去调对方
gate        → merge/promote 时清单是否已绿
```

---

## 1. 第一性原理

F3：组织内多个独立权威。表结构的权威在 Hive，GRANT 的强制权威在 Ranger；仓的权威是身份、版本、来源信封。仓内 `structure` / `permissions` 都是对外部事实的 SOURCE 知识，允许落后。

F4：权威变更与观察过的事件承诺不同。CDC 行不是表的当前态；作业实例不是作业定义。

K-21：写入只经 Writer。Connector 不得直写 git。不新增 Surface。无人值守同步走 `COMMIT`，不走 `PROPOSAL`。

K-13：查询层不把多权威写成覆盖。`structure` 采集不得盖 `classification`。拆 Scope（按 Aspect / 变化源），不要一个「Hive connector」写光整对象。

写代数只有整单元 `PUT` / `REMOVE`。事件载荷几乎总是缺字段，不能当 PATCH。

---

## 2. 我们提供什么

底座只冻结 **入站契约 + 对账 kit**。不运行源、不调度、不铸 `object_id`。

1. **ABI** — 已有的 `CommitChangeSet` JSON（`kc commit --changeset` / `POST /v1/commit`）。任意语言只要交出这一份。
2. **kit（`connector.Preview`）** — 纯函数。输入：本 connector 允许写的 Scope、源侧译好的 Desired、仓内 Observed digest。输出：ChangeSet 预览。不 import Writer / Catalog / CLI，也不被它们 import。
3. **两种 mode**
   - `patch`：只对 Desired 做 PUT（新增 `IF_ABSENT`，变化 `IF_DIGEST_EQUALS`）。**不**因 Observed 多出来的地址而 REMOVE。给增量信号用。
   - `reconcile`：在 **Observed ∩ Scope** 上做集合差，源侧消失则 REMOVE。给反熵 / 删除信号用。REMOVE 宇宙是这次传入的 Observed，不是「仓里该 Aspect 的全部」——全量对账由调用方把范围内的地址都放进 Observed。
4. **信封** — `originKind=SOURCE` + `sourceRefs` + `producedAt`。缺 `sourceRefs` 拒预览。
5. **Checkpoint / Signal 形状** — JSON 类型，kit **不落盘**。映射表、cursor、源密码留在 connector 侧。

不提供：源 SDK、Recipe DSL、`kc connector-run --plugin`、自动 `promote`、source key → `object_id` 的中央表。

`writer.Ingest` / `writer.Reconcile` 仍是本地目录 / **object_id 实体 blob** 的薄编排（T7）。Address 级、按变化源拆 Scope 的对账在本包。两者都只出预览。

---

## 3. 流程

```text
                    ┌─ 源系统（领域权威）─┐
                    │  Hive / SR / Ranger │
                    └─────────┬───────────┘
           signal（key）      │     GET 当前态
                              ▼
                 Connector 进程（墙外维护）
                   译 Address + 全量 value
                   object_id 已铸则冻结
                              │
                              ▼
                    connector.Preview
                   Desired × Observed × Scope
                              │
                              ▼
                 ChangeSet JSON（可空则跳过 COMMIT）
                              │
                              ▼
              Writer COMMIT / kc commit / POST /v1/commit
                              │
                              ▼
                 Repository commit（SOURCE 快照）
                 promote 另做；不跟 latest
```

### 3.1 感知 ≠ 状态

变更通知（CDC、审计、webhook、版本号轮询）只带 **source key**。不要把事件载荷当成新 Aspect 值。乱序、丢失、部分列是常态。

删除信号：用 `reconcile`，Observed 只放这次涉及到的地址，Desired 不含已删的 key。不要拿一次增量 `patch` 去扫整仓。

### 3.2 翻译在墙外

| 墙外（connector / 场景） | 墙内（kit / Writer） |
|---|---|
| 连源、Watch、Fetch、List | Scope 校验 |
| source key → 已有 `object_id`（首次铸出后冻结） | digest 对账 |
| 一个源记录 → 一个或多个 Unit（表 + 列） | PUT/REMOVE + 前置条件 |
| Recipe、映射表、checkpoint 文件 | `SOURCE` 信封 |
| 调度（cron / Airflow） | CAS、`command_id` 幂等 |

`object_id` 不是 FQN、不是路径。现名变了只改 source key / `qualified_name`。映射表属于 connector 私有状态，不是协议对象。

### 3.3 增量与反熵

| 何时 | mode | Observed | Desired |
|---|---|---|---|
| 信号：若干 key 变了 | `patch` | 这些 Address 在仓内的 digest（没有则省略） | 源上 GET 到的当前 Unit |
| 信号：若干 key 删了 | `reconcile` | 这些 Address | 空，或不含已删 key |
| 周期全量 | `reconcile` | **范围内**仓内全部 Address | 源上 List+Fetch 的全部 Unit |

漏信号是预期。全量 `reconcile` 是正确性的底，增量是降本。不要把 CDC 当 GT。

### 3.4 提交

- 预览 `empty=true`：不要 `commit`（Writer 拒空 ChangeSet）。
- `command_id`：`connector:<id>:<runKey>`。同内容重试同一 id；源又变了或 CAS 过期则换新 id 再 diff。`RunKey` 可对 operations 做 canonical digest。
- `NON_FAST_FORWARD`：重读 head，刷新 Observed，换 `command_id`。
- 不自动 `promote`。CommitReceipt 不等于读者看见的 Release。

可选：把「T 时刻收到信号 / 对账 cursor」APPEND 到 observations 仓。那是观察，不是表实体的当前权威。

---

## 4. 契约形状

### 4.1 Scope

一个 connector 只拥有一部分 Address，典型是若干 `aspectName`（再加可选 `objectPrefix`）。

```text
hive-structure     aspects: [structure]     → Table.structure / Column.structure
ranger-permissions aspects: [permissions]   → Table.permissions（SOURCE 知识；强制仍在 Ranger）
steward-class      aspects: [classification] 人写；采集碰不到
```

GRANT 与 schema 若 Release 节奏或读者集合不一致，把 `permissions` 放到另一仓（四元组），不要「因为是权限就不进仓」。

Desired 超出 Scope → `SCOPE_DENIED`，整次预览失败。  
Observed 超出 Scope → 忽略，计入 `summary.ignored`，永不因此 REMOVE。

`allowEntity` 才允许无 `aspectName` 的 Entity / Relation blob。空 Scope（既无 aspects 又不许 entity）非法。

### 4.2 Plan → Preview

`Desired[]`：已翻译的 Unit（Address + 全量 value + 可选 `schemaRef` / `pathHint` / `sourceKey`）。kit 不铸身份。  
`Observed[]`：仓内 digest。调用方自己 READ；kit 不碰 Repository。

对账：

| 情况 | `patch` | `reconcile` |
|---|---|---|
| Desired 有、Observed 无 | PUT `IF_ABSENT` | 同 |
| Desired 有、digest 不同 | PUT `IF_DIGEST_EQUALS` | 同 |
| Desired 有、digest 同 | 跳过 | 跳过 |
| Observed 有、Desired 无 | 跳过 | REMOVE |

### 4.3 对外部实现的 ABI

不 import Go 也可以。稳定口就是 ChangeSet：

```json
{
  "targetRepository": "kr://acme/public/physical",
  "targetRef": "refs/heads/main",
  "baseCommit": "<head>",
  "expectedTargetCommit": "<head>",
  "message": "connector hive-structure reconcile",
  "provenance": {
    "originKind": "SOURCE",
    "actorRef": "hive-structure",
    "sourceRefs": ["hive://cluster/db"],
    "producedAt": "2026-08-20T03:00:00Z"
  },
  "operations": [
    {
      "op": "PUT",
      "address": {"kind": "Aspect", "objectId": "Table:c.db.t", "aspectName": "structure"},
      "value": {"qualified_name": "db.t"},
      "schemaRef": "schema/dw.table.structure",
      "precondition": {"type": "IF_DIGEST_EQUALS", "digest": "…"}
    }
  ]
}
```

契约版本随协议走。connector 发布说明钉 ChangeSet / Address 规则，不要钉 `kc` 内部包路径。协议 bump 时用「源快照 → 合法 ChangeSet」fixtures，外部实现自己跑。

---

## 5. 代码怎么养

| 层 | 路径 | 谁改 | 发布 |
|---|---|---|---|
| 入站契约 | `repository.CommitChangeSet`、Writer、本文件 | 仓库根 | 协议版本 |
| kit | `connector/` | 仓库根 | 随协议；无 CLI 动词 |
| 参考 connector | `.scenes/data-warehouse/` | 场景树 | 不进 `go test ./...` |
| 生产 connector | 源团队自己的仓 | 他们 | 独立版本，只依赖 HTTP/CLI |

Writer / Catalog / `index/` / CLI **不** import `connector/`。  
`connector/` **不** import Writer / Catalog / CLI（测试 roundtrip 除外）。  
不要在根上加 `collectors/`，不要 `kc connector-run`。

Recipe（连哪套 Hive、哪个 `--repo`、多久全量）是 connector 配置，不是 `schema/*` 知识对象。

---

## 6. 明确不做

- 进程内插件 / 源 SDK 进 main
- 为镜像再开 Write Surface
- 用 hook 做采集（方向反了）
- 无人值守走 `PROPOSAL`
- 采集成功自动 `promote`
- 事件载荷当 PATCH；CDC 行当表实体权威
- 一个 connector 写同一对象上所有 Aspect
- kit 持久化映射表或 checkpoint
- 读路径跟随源 `latest`
- 把仓内 `permissions` digest 当成 SELECT 放行（Ranger / Unity 才是强制权威；见 `ASPECT_ACCESS.md`、`PERMISSIONS.md` §1.1）

语义层指标、术语、ViewDefinition 是另一类：权威在仓里。不要用本流程去「同步」它们。

---

## 7. 实现与验收

已落地：`connector/`（`Preview` / `Scope` / `Envelope` / `CommandID` / Checkpoint 形状）。无 `kc` 动词。无源客户端。

仍未做：场景侧 Hive 参考 connector、HTTP 以外的跨进程幂等、MCP 入站。

Conformance（包测试，不占 T 号）：`patch` 不因多余 Observed 而 REMOVE；`reconcile` 在 Observed∩Scope 上 REMOVE；Desired 超 Scope → `SCOPE_DENIED`；缺 `sourceRefs` 拒预览；空 ChangeSet 标 `empty`、不强迫 commit；预览可被 `Writer.Commit` 落盘。
