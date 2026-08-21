# Gate：`merge` / `promote` 的证据清单

日期：2026-08-19  
范围：治理跃迁能不能发生，只看钉死的 Preview / Generation 上有没有必过的绿记录。不是出站调用，不是 `allow`。  
对照：GitHub Ruleset + required checks（不抄 webhook）。  
前置：`KNOWLEDGE_CATALOG_DESIGN.md`（K-08、K-09、第 8 章维护闭环）；`WALKTHROUGH_v5.1.md`（`validate` / `record-validation` / `merge` / `promote`）；`PERMISSIONS.md`（谁能 `merge`）；`HOOKS.md`（出站，方向相反）。

参考实现提供 `kc gate-add` / `gate-ls` / `gate-rm`，配置 `.kc/gates.json`。`merge` 在无清单时仍强制带一个 `--validation`（单门雏形）。有清单后以 `--require` 为准，不能靠省略 `--validation` 跳过。

---

## 0. 主张

Gate 是 **入站约束**：我们在 `merge` / `promote` 时查清单。不拨用户系统的电话。

证据怎么来的，底座不管：本进程 `kc validate`、用户 CI 调 `record-validation`、人把 suite 写成 `approval:steward`。底座只认：

```text
检查名 + PASSED|FAILED + 绑死的 previewId 或 generationId
```

Gate **不是** hook。Hook 是我们在动词的 `pre`/`post` 去调对方，见 `HOOKS.md`。`pre-merge` exit 0 不能冒充本清单：Preview 变了脚本还成功，K-08 就空了。

```text
allow  →  谁能调用 merge/promote
gate   →  这次跃迁要哪些已经绿的证据
hook   →  可选：踢 CI（post）或额外否决（pre），不替代本清单
```

`allow` 失败是无权。Gate 失败是证据不齐。不要混成一种错误。

---

## 1. 第一性原理

K-08：Review、Validation、Approval、MergeGate 必须绑精确 Candidate，不能绑分支名。

K-09：ValidationReport 必须绑完整 PreviewGeneration。因此能不能 `merge` **只看这代上的记录**。

F8：口径测试、两人审批不进协议。底座不解释 `metrics-contract` 是什么，只查这个名字是否 PASSED。

F6：证据是 PASSED/FAILED，不是模型分数。

`put` / `append` 不设 gate。采集要毫秒级走完；非法 payload 用 `pre` hook（`HOOKS.md`）。给 `put` 配 gate 会把写入拖成等 CI，调用方会绕开 Writer。

读路径不设 gate。可见性是 `allow`。

---

## 2. 我们提供什么

1. **内建一项检查** — `kc validate`（成员是否挂载、commit 是否还在）。可写进 `--require`，不能冒充口径测试。
2. **入站证据口** — 已有 `kc record-validation --suite <名> --outcome PASSED|FAILED`。不跑套件。用户系统（和人审）从这里写回来。
3. **清单** — `kc gate-add --on merge|promote --require …`。`merge` / `promote` 读清单，缺一则失败。调用方不能靠少传参数跳过。

不提供：套件实现、规则 DSL、审批产品、把 `validate` 跑成场景测试。

Candidate、目标 `main`、任一 Preview 成员或套件身份变化：旧报告作废（`CANDIDATE_MOVED` / Validation basis）。Gate 不改这条。

---

## 3. 操作面

配置在 `.kc/gates.json`，不是知识对象。谁能改 = 谁能写 `.kc/`。清单不能放进它要拦的那次 `put` 里。

```text
kc gate-add --on merge --repo kr://acme/public/semantic \
  --require validate,suite:schema-lint,suite:approval:steward

kc gate-add --on promote --catalog kr://acme/catalog --release stable \
  --require validate,suite:contract

kc gate-ls [--repo … | --catalog …]
kc gate-rm --id gt_…
```

`--require` 里的名字：

| 名字 | 证据从哪来 |
|---|---|
| `validate` | `kc validate --preview` / 对 Generation 的结构检查 |
| `suite:<名>` | `kc record-validation --suite <名> --outcome PASSED`，且绑定 **同一** previewId 或 generationId |

只卡治理跃迁：

| 命令 | 绑什么 | 挡住什么 |
|---|---|---|
| `merge` | PreviewGeneration + 精确 candidate | 候选 → `main` |
| `promote` | 这一代 Generation | 读者 Release 指针 |

`promote` 的门和 `merge` 的门分开：语义仓可能已进 `main`，`stable` 读者还要过合同门。读者跟 Release，不跟 `main`。

有 `gates.json` 之后，`merge` / `promote` 以清单为准，不能靠省略 `--validation` 跳过。无配置时保持今天的单门雏形（`merge` 仍带一个 `--validation`；结构 `FAILED` 不能合）。

---

## 4. 一次 `merge` 怎么过

```text
kc propose …
kc preview --proposal PR-42 --view dw
kc validate --preview PV1
kc record-validation --preview PV1 --suite schema-lint --outcome PASSED
kc record-validation --preview PV1 --suite approval:steward --outcome PASSED
kc merge --proposal PR-42 --preview PV1
```

内部顺序：

1. `allow --cmd merge`
2. 本 `--repo` 的 merge-gate：清单是否都 PASSED 且绑 `PV1`
3. 可选 `pre-merge` hook（额外否决，见 `HOOKS.md`，不替代本步）
4. CAS 快进 `main`。Release 仍不动
5. `post-merge` hook / `WATCH`（指针事件，见 `HOOKS.md`）

`promote --release stable` 换一份清单，绑 Generation，步骤同构。

CI 可以睡着：绿记录一小时前写过即可。这是 gate 相对 `pre` hook 的核心差别。

---

## 5. 数仓里 gate 做什么

```text
kc gate-add --on merge --repo kr://acme/public/semantic \
  --require validate,suite:metrics-contract,suite:approval:steward

kc gate-add --on promote --catalog kr://acme/catalog --release stable \
  --require suite:metrics-contract
```

物理采集仓通常 **不** 配 merge-gate（直 `put`）。财务仓另设自己的 `--require`。要求和 ACL 一样跟仓走，不跟 view 继承。

踢 CI、通知问答 Agent 是 hook，写在 `HOOKS.md`。

---

## 6. 明确不做

- 不把 gate 做成 hook 的一种 phase。
- 不让 `pre` hook 替代本清单。
- 不让 gate 改 ChangeSet。
- 不把 `allow` 做成 gate。
- 不给 `put` / `append` / `read` 设 gate。
- 不把场景测试跑进 `kc validate`。

---

## 7. 实现与验收

已落地：`gate/`（纯 `Check`）+ `kc gate-*`。`ControlPlane.Merge` 与 `Catalog.Promote` 调用清单。`Rollback` 不跑 promote gate。证据来自 `kc validate`（suite `structure`）和 `kc record-validation`（可 `--preview` 或 `--generation`）。`put` / `append` / `read` 无 gate。

无 `gates.json` 时现有 CLI 测试仍过（`merge` 仍带一份 PASSED `--validation`）。

Conformance：缺 `suite:metrics-contract` 不能 `merge`；Preview 变了旧 PASSED 作废；`promote` 不因 `main` 已快进而跳过自己的清单。
