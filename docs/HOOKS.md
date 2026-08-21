# Hook：出站接用户系统

日期：2026-08-19  
范围：底座在现有 `kc` 动词的 `pre` / `post` **去调用**用户脚本或 HTTP。不是权限，也不是 `merge` / `promote` 的门槛。  
对照：Git hooks / `pre-receive`、GitHub webhook、K8s Validating Admission（不抄 Mutating）。  
前置：`KNOWLEDGE_CATALOG_DESIGN.md`（F6、F8、K-21、第 7.4 节 `WATCH_UPDATES`）；`PERMISSIONS.md`（`allow` 先于 hook）；`GATES.md`（跃迁门槛，方向相反）；`CONNECTORS.md`（入站镜像，方向相反）。

参考实现提供 `kc hook-add` / `hook-ls` / `hook-rm`，配置 `.kc/hooks.json`。无配置时不发出站调用。不得用 Git `.git/hooks` 冒充本能力。

---

## 0. 主张

Hook 是 **出站**：某个 `kc` 命令获准之后、或已经落盘之后，我们去调用户自己的系统。底座不解释脚本里是校验 source key 还是通知 Agent。

它不是 gate。Gate 不拨电话，只查已经写在 Preview / Generation 上的证据，见 `GATES.md`。

```text
allow      →  谁能调用这条命令
connector  →  对方感知源变了，来提交 ChangeSet（见 CONNECTORS.md）
hook       →  我们在这条命令的 pre/post 去调对方
gate       →  merge/promote 时清单是否已绿（对方早先 record-validation）
```

用户系统若要让 CI 跑起来，用 `post-propose` hook **踢** CI；CI 用 `record-validation` **写回来**。写回属于 gate 的入站口，不是 hook 的一种 phase。

---

## 1. 第一性原理

F8：领域脚本不能进协议对象。出站点必须薄：何时调、给什么、失败怎么算。

F6：`pre` 只允许机械否决（exit code）。禁止改 ChangeSet、补 provenance、让 LLM 判真。

K-21：hook 进程不得自己再 `put`。派生重算是独立命令、独立 `command_id`。`post` 失败不撤销已成功的命令（回滚分层：投影重建 / `rollback` / 再 `put`）。

读路径不挂 hook。`read` 必须可重放；可见性是 `allow`。

---

## 2. 我们提供什么

底座提供 **生命周期点 + 两种投递**。用户提供可执行文件或 URL。

1. **点** — 现有 `kc` 动词 × `pre`/`post`。一条命令打一次，不是每个 Address。
2. **`--run`** — 本地进程，stdin 给规范化命令。
3. **`--url`** — HTTP，body 同契约。适合 `post`。
4. **`WATCH_UPDATES`** — 对 `post` 事件流做授权裁剪后的订阅，不是另一套 CDC。

不提供：套件内容、规则 DSL、审批流、在 hook 里代写知识。

`REPLAYED`（同 `command_id` 重放）不再打 hook。

写面不要混：`pre-put` 与 `pre-propose` 是两条。`put` 已含提交；只有用户走 `kc commit` 时才有 `pre-commit`。

---

## 3. 操作面

配置在 `.kc/hooks.json`，不是知识对象，不进成员仓。谁能改 = 谁能写 `.kc/`。Git 把 hook 放在 `.git/hooks` 而不是树里，原因相同。

```text
kc hook-add --on put --phase pre \
  --repo kr://acme/public/physical \
  --run ./hooks/pre-put-physical

kc hook-add --on promote --phase post \
  --catalog kr://acme/catalog \
  --url https://agent.example/hooks/kc

kc hook-ls
kc hook-rm --id hk_…
```

`--on` 只允许现有动词。`--repo` / `--catalog` 与 `allow` 同一资源。

| 相 | 何时 | 可以 | 不可以 |
|---|---|---|---|
| `pre` | 通过 `allow` 之后、CAS 落盘之前 | **放行或拒绝** | 改 payload、当 gate 用（不绑 Preview） |
| `post` | Receipt 已持久（`APPLIED`） | 通知、重建投影、踢 CI | 回滚这次提交；带未授权正文 |

`pre`：stdin 含仓、`--as`、Address 列表、digest。exit 0 放行，非 0 整命令失败、无部分提交。超时 **fail closed**。对方必须在线。适合写路径上机械、短的否决（缺 source key）。

`post`：body 只有指针：

```text
{cmd, as, repo|catalog, newCommit|generationId, receipt}
```

要内容自己 `read`（仍受当时 `allow`）。失败进 Outbox 重试。`post` 不挡住已经发生的跃迁。

有人 `git push` 绕过 Writer 是部署事故，不是产品开关。

---

## 4. 和 gate 的边界

允许：`post-propose` 踢 CI，CI 再 `record-validation`（证据进 `GATES.md`）。  
禁止：`pre-merge` exit 0 就算过门。能不能 `merge` 只看这代上的记录，见 `GATES.md`。

`pre-merge` 若存在，只是额外否决，**不替代** gate 清单。

---

## 5. 数仓里 hook 做什么

```text
kc hook-add --on put --phase pre \
  --repo kr://acme/public/physical \
  --run ./hooks/check-source-key

kc hook-add --on put --phase post \
  --repo kr://acme/public/physical \
  --run ./hooks/notify-index-ready
  # 工作投影由 index/ 经 Catalog.Hook AfterSnapshot 增量更新（Writer/Merge → Store → Catalog），不必再全量 rebuild

kc hook-add --on propose --phase post \
  --repo kr://acme/public/semantic \
  --url https://ci.example/kc/propose

kc hook-add --on promote --phase post \
  --catalog kr://acme/catalog \
  --url https://qa-bot.example/hooks/kc
```

进 `main` / 给读者的必过检查不写在这里，写在 `GATES.md`。

派生：`post-put` 踢作业，作业再 `put` 带 DERIVATION 信封。

---

## 6. 明确不做

- 不把 gate 做成 `phase: gate`。
- 不 Mutating：不改 ChangeSet。
- Hook 内不调 Writer。
- 不给 `read` 挂 hook。
- 不把场景采集器做成 hook。采集仍在 Writer 之前预览 ChangeSet。入站流程见 `CONNECTORS.md`。
- 对象级 payload 的 webhook（K-20 / 防旁路）。

---

## 7. 实现与验收

已落地：`hook/`（CLI facade 调用；Writer / Catalog / Repository 不 import）+ `kc hook-*`。`pre` 必须 `--run`、5s 超时 fail closed。`post` 支持 `--run` 与 `--url`；失败进 `.kc/hook-outbox.jsonl`，不撤销命令。`REPLAYED` 不打 hook。读路径、`init` / `allow` / `hook-*` / `gate-*` 不挂 hook。

Catalog 另有 **进程内** `catalog.Hook`（`AfterSnapshot` / `AfterPin` / …），给 `index/` 这类底座 sidecar 用。`COMMIT` / `merge` 在 `repository.Store` 上发事件，Catalog 自己转给 Hook；CLI / `kc serve` 不得在命令后补通知。不写 `.kc/hooks.json`，也不是本文件的出站契约。

仍未做：独立 `WATCH_UPDATES` 订阅口（投递端就是 `post` 事件）。无 `hooks.json` 时现有 CLI 测试仍过。

Conformance：`pre-put` 非 0 无 commit；`REPLAYED` 不打 hook；`post-promote` 收不到未授权仓正文；`post` 失败不回滚 `promote`。
