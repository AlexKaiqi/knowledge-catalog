# Data Warehouse Workbench

公司工作台的 **数仓场景验收套件**（Go API）。它消费从 `main` 合入的协议实现，不在这里演进协议。

对照立项（[`validation/docs/立项.html`](../docs/立项.html)）：平台对源负责、组织对口径负责、个人是后置发表。例题跟口径走。个人习惯 / 问题分布进个人仓，**不进**公司分析 Workspace。用 `kc` 逐步实跑并核对文件：[`validation/docs/WALKTHROUGH_WORKBENCH.md`](../docs/WALKTHROUGH_WORKBENCH.md)。

## 怎么跑

```bash
export PATH="$HOME/.local/go/bin:$PATH"
go test ./validation/workbench -count=1
```

三仓都是 FileGit，不依赖 Docker。Gitea 成员身份由 `go test ./gitea` 的 T12 覆盖。`--as` / `.kc/allow.json` 的求值在 `cli/`；本套件用同一组角色规则走 `cli.MatchAllow`，核心路径仍是 Writer / Catalog / ControlPlane / Reader。

不要 `t.Parallel()`：步骤共享同一间 Catalog。

## 三仓 · 两 Workspace

```text
Catalog  kr://acme/catalog
├── kr://acme/public/metadata     平台统一元数据；采集 COMMIT
│     schema/table.* 、 Table:dwd.trade_order（structure / ownership）
├── kr://acme/org/semantics       组织口径 + 例题；改口径 PROPOSAL
│     schema/metric.* 、 Metric:gmv 、 Example:gmv-refund
└── kr://acme/personals/kai       个人习惯 / 问题分布；COMMIT + APPEND
      Habit:morning-review 、 Dist:error-by-topic ；流 practice
```

| Workspace | 成员 | 跟哪根已发布分支 | 谁跟 |
|---|---|---|---|
| `analyst-board` | metadata + semantics | 各仓 `refs/heads/main` | 分析 Agent |
| `kai-desk` | 仅 personal | 个人 `refs/heads/main` | 个人 Agent |

只给 personal 绑 JSONL。metadata 上 APPEND → `TARGET_REPOSITORY_DENIED`。不要 `repo-add` Catalog id。

## 正路径（每步 `DumpState()`）

CLI 对应口是 `kc read --catalog`（当前组合空间：`catalogId` / `repositories` / `workspaces`），不是 `kc audit`（登记表 git）。消费读是 `kc read --workspace`。

| 步 | 做什么 | Catalog |
|---|---|---|
| S0 | init + 三仓 register | workspaces 空；`OpenWorkspace(analyst-board)` 失败 |
| S1 | 写入三仓 + 流 `evt-1` | **不变**。坏 `schema_ref`、DERIVATION、幂等冲突都不动登记表 |
| S2 | `define-workspace` | 配方已登记。立刻 `OpenWorkspace` / `FederatedRead`；`Metric:gmv` 此时还不存在 |
| S3 | propose / preview / gate / merge | Preview 只写 ControlState。merge 快进 `main` 后，下次 `OpenWorkspace` **立刻**读到新 GMV |
| S4 | `kai-desk`；个人草稿 GMV；反例 overlay | desk 跟 personal `main`；overlay 上同一 `object_id` 两条 FederatedValue，不覆盖；`analyst-board` 仍是公司两仓 |
| S5 | 改 Workspace sources；path-hint、ownership、IfAbsent、过期 CAS、log/diff/provenance | 下次 `OpenWorkspace` 用新配方 |
| S6 | retire-workspace / archive-repo / archive-catalog | 配方退役后不能再 `OpenWorkspace`；仓归档禁写；Catalog 归档禁 define；个人仓仍可写 |

哈希与 JSON 以 `go test ./validation/workbench ./cli` 为准。
