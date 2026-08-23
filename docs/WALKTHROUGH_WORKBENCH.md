# 公司工作台走通（三仓 · 两 Workspace）

用 `kc` **逐步实跑**立项拆法：平台对源负责、组织对口径负责、个人是后置发表。

> 消费口是 `kc read --workspace`。Workspace 只做组合（哪几个仓、跟哪根已发布分支）。发布者把知识仓已发布分支往前推（COMMIT / merge）后，**下次**读即可见。哈希以 `go test ./scenario ./cli` 为准。

对照：[`scenario/README.md`](../scenario/README.md)（Go API 套件）；[`WALKTHROUGH_v5.1.md`](WALKTHROUGH_v5.1.md)；[`PERMISSIONS.md`](PERMISSIONS.md)。

```bash
export PATH="$HOME/.local/go/bin:$PATH"
export KC_HOME=/tmp/kc-workbench
kc() { go run ./cmd/kc -- --home "$KC_HOME" "$@"; }
```

```text
Catalog  kr://acme/catalog
├── kr://acme/public/metadata     平台统一元数据；采集 COMMIT
├── kr://acme/org/semantics       组织口径 + 例题；认领走 PROPOSAL
└── kr://acme/personals/kai       个人习惯 / 问题分布；COMMIT + APPEND
```

| Workspace | 成员 | 跟哪根已发布分支 | 谁跟 |
|---|---|---|---|
| `analyst-board` | metadata + semantics | 各仓 `refs/heads/main` | 分析 Agent |
| `kai-desk` | 仅 personal | 个人 `refs/heads/main` | 个人 Agent |

看四列：成员库 `main`、候选 Ref、Catalog 当前态 `read --catalog`、读者 `read --workspace`。

一次 `read --workspace` 开始时对各 source `GetRef(selector)`，**命令内冻结、不落盘**。并发 merge 要等下一次命令。

---

## S0　工作区 + 三仓 + 发权

```bash
kc init --catalog acme/catalog
kc repo-add --repo kr://acme/public/metadata
kc repo-add --repo kr://acme/org/semantics
kc repo-add --repo kr://acme/personals/kai
kc read --catalog
# catalogId / repositories / workspaces（workspaces 空）

kc allow --principal steward --cmd define-workspace --catalog kr://acme/catalog
kc allow --principal steward --cmd retire-workspace,index-plan,archive-catalog --catalog kr://acme/catalog
kc allow --principal analyst --cmd read-workspace --catalog kr://acme/catalog --workspace analyst-board
```

`OpenWorkspace(analyst-board)` 此时失败：还没有配方。

---

## S1　写入三仓（登记表不变）

采集 COMMIT 进 metadata；口径 schema 进 semantics；个人习惯 COMMIT + APPEND。坏 `schema_ref`、DERIVATION、幂等冲突都不动 Catalog。

维护读仍是 `kc read --repo … --ref refs/heads/main`。

---

## S2　define-workspace 即可读

```bash
kc define-workspace --as steward --workspace analyst-board --revision 1 \
  --source kr://acme/public/metadata=refs/heads/main \
  --source kr://acme/org/semantics=refs/heads/main

kc read --workspace analyst-board --object Table:dwd.trade_order
kc list --workspace analyst-board
kc search --workspace analyst-board --match gmv
kc log --workspace analyst-board --object Table:dwd.trade_order
kc provenance --workspace analyst-board --object Table:dwd.trade_order
kc describe-schema --workspace analyst-board --object schema/table.structure
kc index-plan --workspace analyst-board
kc read --catalog   # 只有 catalogId / repositories / workspaces
```

S2 时 `Metric:gmv` 还不存在。SEARCH 用这次 `ResolveWorkspace` 解开的 commit；发布者刚推上已发布分支，**下次** search 能命中。


---

## S3　认领 GMV：propose ≠ merge；merge 后立刻可见

```bash
kc propose --proposal-id PR-gmv --repo kr://acme/org/semantics \
  --target refs/heads/main --candidate refs/heads/candidates/PR-gmv \
  --object Metric:gmv --value '{"formula":"sum(pay_amt)"}'

kc preview --proposal PR-gmv --workspace analyst-board
# previewId = Hash(Workspace + overlay + 内容)；只写 .kc ControlState，不写登记表

kc validate --preview <previewId>
kc record-validation --preview <previewId> --suite approval:steward --outcome PASSED
kc merge --proposal PR-gmv --preview <previewId> --validation <reportId>

kc read --workspace analyst-board --object Metric:gmv
# merge 后立刻看见新 GMV（跟 semantics 的 main）
```

Preview 要求当前 Workspace 解析到的 target commit == `proposal.BaseCommit`，再 overlay candidate。Gate 只查 `--on merge`，证据绑 `previewId`。

---

## S4　个人 desk

```bash
kc define-workspace --workspace kai-desk --revision 1 \
  --source kr://acme/personals/kai=refs/heads/main

kc read --workspace kai-desk --object Habit:morning-review
```

desk 跟 personal `main`。个人草稿 GMV 不进 `analyst-board`。overlay Preview 上同一 `object_id` 两条 FederatedValue，不按 public/group/personal 覆盖。

---

## S5　改配方，下次读生效

`define-workspace` 提高 revision、增删 `--source` 后，下次 `OpenWorkspace` 用新配方。维护口仍 `log` / `diff` / `provenance --repo --commit`。过期 `expectedTargetCommit` → `NON_FAST_FORWARD`。

---

## S6　收场

```bash
kc retire-workspace --as steward --workspace analyst-board
# 之后 OpenWorkspace / read --workspace 失败

kc archive-repo --repo kr://acme/org/semantics
# 仓禁写；新 OpenWorkspace 不选入

kc archive-catalog --as steward --catalog kr://acme/catalog
# 整间禁 define-workspace；个人仓仍可写
```
