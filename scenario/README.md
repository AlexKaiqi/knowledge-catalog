# scenario/

公司工作台的 **协议故事套件**（Go API）。不是 `.scenes/data-warehouse/` 的采集落地，也不是禁止的 `tests/scenarios/`。

对照立项（[`docs/立项.html`](../docs/立项.html)）：平台对源负责、组织对口径负责、个人是后置发表。例题跟口径走。个人习惯 / 问题分布进个人仓，**不进**公司 `stable`。用 `kc` 逐步实跑并核对文件：[`docs/WALKTHROUGH_WORKBENCH.md`](../docs/WALKTHROUGH_WORKBENCH.md)。

## 怎么跑

```bash
export PATH="$HOME/.local/go/bin:$PATH"
go test ./scenario -count=1
```

三仓都是 FileGit，不依赖 Docker。Gitea 成员身份由 `go test ./gitea` 的 T12 覆盖。`--as` / `.kc/allow.json` 的求值在 `cli/`；本套件用同一组角色规则走 `cli.MatchAllow`，核心路径仍是 Writer / Catalog / ControlPlane / Reader。

不要 `t.Parallel()`：步骤共享同一间 Catalog，后一步依赖前一步的钉死指针。

## 三仓 · 两 View

```text
Catalog  kr://acme/catalog
├── kr://acme/public/metadata     平台统一元数据；采集 COMMIT
│     schema/table.* 、 Table:dwd.trade_order（structure / ownership）
├── kr://acme/org/semantics       组织口径 + 例题；改口径 PROPOSAL
│     schema/metric.* 、 Metric:gmv 、 Example:gmv-refund
└── kr://acme/personals/kai       个人习惯 / 问题分布；COMMIT + APPEND
      Habit:morning-review 、 Dist:error-by-topic ；流 practice
```

| View | 成员 | Release | 谁跟 |
|---|---|---|---|
| `analyst-board` | metadata + semantics | 公司 `stable` | 分析 Agent |
| `kai-desk` | 仅 personal | 个人 `desk` | 个人 Agent |

只给 personal 绑 JSONL。metadata 上 APPEND → `TARGET_REPOSITORY_DENIED`。不要 `repo-add` Catalog id。

## 正路径（每步 `DumpState()`）

CLI 对应口是 `kc read --catalog`（当前组合空间），不是 `kc audit`（登记表 git）。

| 步 | 做什么 | Catalog |
|---|---|---|
| S0 | init + 三仓 register | views / generations / releases 空；`OpenRelease(stable)` 失败 |
| S1 | 写入三仓 + 流 `evt-1` | **不变**。坏 `schema_ref`、DERIVATION、幂等冲突都不动登记表 |
| S2 | `define-view` 后 `Publish(stable)` | `G1={metadata:U1, semantics:S1}`；`Metric:gmv` 在 G1 上不存在 |
| S3 | propose / preview / gate / merge / promote | Preview 多登记一代 `Gpv`（还不是 Release）。merge 快进 `main` 后 **Release 仍指向 G1**。再 promote：快进合并时 `G2` 与 `Gpv` 是同一份 `{仓→commit}`（身份是内容哈希），变的是 Release 指针。 |
| S4 | `kai-desk`；个人草稿 GMV；反例 overlay pin | `desk` 钉在 K1，不跟随个人 main；overlay 上同一 `object_id` 两条 FederatedValue，不覆盖；`stable` 仍 G2 |
| S5 | path-hint、ownership、IfAbsent、过期 CAS、log/diff/provenance | **不 promote** |
| S6 | retire-view / retire-release / archive-repo / archive-catalog | 已钉死的 Generation 仍可读；仓归档禁写；Catalog 归档禁 define/promote；个人仓仍可写 |

失败枝（失败后 DumpState 与失败前逐字段相等）：`SCHEMA_REVISION_UNRESOLVED`、`NON_FAST_FORWARD`、`IDEMPOTENCY_CONFLICT`、`EVENT_ID_CONFLICT`、`GATE_UNSATISFIED`、`CANDIDATE_MOVED`、`PROMOTION_CAS_FAILED`、未 register 成员、`REPOSITORY_ARCHIVED`、`CATALOG_ARCHIVED`。采集写口径 / 分析 Agent 写仓由角色表拒绝（`FORBIDDEN` 在 facade）。
