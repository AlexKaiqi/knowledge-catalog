# DSH 内置浏览器全 Action 多角色验收（2026-08-24）

## 结论

协议能力覆盖通过，首次对话体验为“有条件通过”。DSH trace 证明 53/53 个公开 `kc` verb、bundled `knowledge-catalog` Skill，以及 Loom 文件 `Read/Write/Edit/List/Glob/Grep` 均被真实 Agent 调用；最终知识、来源、流、权限拒绝和生命周期状态也由独立只读 oracle 复核。

首次运行仍需要测试控制器纠正参数和重启宿主刷新 Loom pin，说明底层抽象成立，但 Agent 接入体验尚未达到“一句话稳定完成”。本次已经把实跑发现的参数契约补进 Skill，并修正可复跑任务矩阵；未把采集执行塞进 Knowledge Catalog。

## 边界与环境

“全部 action”以 `cli/command.go` 的唯一公开命令表为准，共 53 个 verb；不穷举所有 flag 组合，也不要求外部 Dolt/Gitea/Redis/ES/StarRocks。

- Evidence：`/tmp/kc-dsh-browser-all-actions.UjiS3R`
- KC Home：`/tmp/kc-dsh-browser-all-actions.UjiS3R/kc-home`
- DSH profile：`loom-browser-all-actions`
- DSH Web：`http://127.0.0.1:3080`
- KC HTTP：`http://127.0.0.1:19380`
- 主 Catalog / Repo / Workspace：`kr://e2e/catalog` / `kr://e2e/public/core` / `all-actions`
- 生命周期 Catalog / Repo：`kr://e2e/archive` / `kr://e2e/scratch/archive`
- 角色：owner、producer、reviewer、consumer、auditor、mallory、lifecycle

## 从零浏览器路径

1. 启动固定 `KC_HOME` 的 DSH Web，复用 `3080`；在 Codex 内置浏览器打开该地址。
2. 首次页面出现“内测声明”且确认失败。定位到模型 patch 错误禁用了 DSH `settings` 服务；恢复服务、重启 `3080` 后在同一标签页继续。
3. 在侧栏展开 `knowledge-catalog`，点击“在 knowledge-catalog 中新建会话”。确认 preset 为 `dsh-loom`、访问模式为 `Full access`。
4. 在输入框粘贴对应角色任务并点击“发送消息”；观察 Agent 加载 `knowledge-catalog` Skill 和每条工具调用。
5. `KC_AS` 是进程级固定身份，不能靠多个聊天隔离。每完成一个角色，停止 DSH，使用同一 profile、同一 KC Home、同一 `3080`，仅替换固定 `KC_AS` 后重启，再在同一浏览器页新建会话。
6. Producer 在同一宿主里先推进 Ref、再使用 Loom 文件工具时命中旧 pin。新聊天仍沿用旧 pin；重启 DSH 宿主后，新会话获得新 pin，Write/Edit/List 与 proposal 随后成功。
7. 七个角色完成后离线聚合 DSH trace，再以 owner 只读 CLI 检查最终 Canonical 状态。浏览器中的业务操作始终由 DSH Agent 发起。

## 逐角色完整操作与状态变化

| 角色 | 浏览器内 Agent action | 实际结果与进入状态 |
|---|---|---|
| Owner | `whoami` → `init` → `store-set` → `store-ls` → `catalog-add` → `repo-add` → `mount` → `register` → `put`×3 → `receipt` → `remove` → `define-workspace`×2 → `overlay` → `hook-add/ls` → `gate-add/ls` → `allow`×15 → `allowed`×2 → `status` → `read --catalog` | 建立两间 Catalog、两个 Repo、两个 Workspace、Schema 与 v1 Policy、角色授权、hook 和 merge gate。`repo-add` 后定义 Workspace 首次失败，Agent 补 `register` 后成功，证明“本机仓存在”与“Catalog 承认仓”是两步。原任务把 hook 绑在 `read` 上不合法，控制器改为 `commit` 后成功。 |
| Producer | `whoami` → `ingest` → `commit` → `receipt` → `append` → `receipt` → `put/remove` → `vfs-write` → Loom `Write/Edit/List` → `propose` | 采集目录先生成 ChangeSet，再通过 Writer commit；流写入 `evt-1`；Workspace 写入 `notes/raw.json` 和 `notes/producer.md`；创建候选 `PR-ALL-1`。Loom 写在 Ref 前移后正确返回 `NON_FAST_FORWARD`，宿主重启刷新 pin 后成功。 |
| Reviewer | `whoami` → `preview` → `validate` → `record-validation` → `merge` → `read` | gate 要求的结构报告与 `approval:steward` 报告均绑定同一 Preview，merge 成功，Policy 进入 v2。Reviewer 没有 `read-workspace`，合并后自检被拒绝；Consumer 随后独立回读。复跑矩阵已补 reviewer 读授权。 |
| Consumer | `whoami` → `resolve` → `read` → `list` → `search`（EQ 与 MATCH）→ `describe-schema` → `stream` → `index-plan` → `inspect` → `checkout` → `sync` → `status` → `vfs-list/read` → Loom `List/Glob/Grep/Read` | 回读 `v=2,status=governed`，检索命中，流中见 `evt-1`，Checkout/Sync 与两套 VFS 读取成功。首次矩阵漏授 `index-plan`，调用按预期拒绝；Lifecycle 覆盖了成功路径，复跑矩阵已补授权。 |
| Auditor | `whoami` → `read --catalog` → `audit`（Workspace、kc、system）→ `log` → `provenance` → `describe-index` → `index-sync` → `diff` → `read --repo` | 确认 Catalog 组合态、v1→v2 历史、`producer` 与 `agent://producer/PR-ALL-1` 来源、索引基准和最终 Canonical 值。 |
| Unauthorized (mallory) | `whoami` → `put` → Workspace `read` → Loom `Write` | 三条访问全部 `FORBIDDEN`；主仓前后 commit 均为 `4fc20d64fe25a375bfec902796e3f0ebe7238d8a`，没有越权副作用。首次宿主误设为 `intruder`，Agent 在 whoami 后立即停止；修正为 `mallory` 后从新会话完整重跑。 |
| Lifecycle Admin | `whoami` → `allowed` → `revoke` → `allowed` → `hook-ls/rm/ls` → `gate-ls/rm/ls` → `index-plan` → `retire-workspace` → `archive-repo` → `archive-catalog` → `status` | `temp-reader` 规则删除；hook/gate 清空；`retire-me` 标记 retired；辅助 Repo/Catalog 归档；主 Catalog、core Repo、all-actions 保持可用。 |

## Action 覆盖

Trace oracle 观察到全部 53 个公开 verb：

`allow, allowed, append, archive-catalog, archive-repo, audit, catalog-add, checkout, commit, define-workspace, describe-index, describe-schema, diff, gate-add, gate-ls, gate-rm, hook-add, hook-ls, hook-rm, index-plan, index-sync, ingest, init, inspect, list, log, merge, mount, overlay, preview, propose, provenance, put, read, receipt, record-validation, register, remove, repo-add, resolve, retire-workspace, revoke, search, status, store-ls, store-set, stream, sync, validate, vfs-list, vfs-read, vfs-write, whoami`。

此外观察到全部 Loom 文件工具：`Read, Write, Edit, List, Glob, Grep`。所有角色都加载了 bundled `knowledge-catalog` Skill。Agent 试探过 `help/commit-changeset/ingest-commit/apply` 等不存在 verb；它们不计入 53 个公开 action，参数参考已补进 Skill 以消除这类试探。

## 最终 oracle

- Trace：`/tmp/kc-dsh-browser-all-actions.UjiS3R/trace-oracle.json`，`pass=true`、`observedVerbCount=53`、无 missing verb/tool。
- 主 Catalog：`archived=false`；Workspace `all-actions` 仍挂载 core。
- 主 Repo：`archived=false`，HEAD `4fc20d64fe25a375bfec902796e3f0ebe7238d8a`。
- Policy：`{"v":2,"status":"governed","note":"reviewed in browser"}`。
- Provenance：`actorRef=producer`，`sourceRefs=["agent://producer/PR-ALL-1"]`。
- Stream：cut/head cursor `1`，包含 `eventId=evt-1` 与 payload `{"kind":"browser","status":"observed"}`。
- 辅助 Repo/Catalog：均 `archived=true`；`retire-me.retired=true`。
- Hook/Gate：均为空；`temp-reader` 规则不存在。

## 是否符合预期

- 符合：Repo 中声明/开发、外部运行、只通过写入协议进入 Catalog；Catalog 不承接采集调度。Snapshot、Stream、Workspace、Proposal/Gate、索引、审计/来源和权限边界彼此独立。
- 符合：固定主体不能由模型覆盖；越权访问无副作用；Reader/索引最终回读 Canonical。
- 不完全符合：`kc` 工具的自由形 `flags` 缺少可发现 schema，Agent 会用空调用猜参数；已增加 `references/action-flags.md`。
- 不完全符合：Loom pin 只能随 DSH 宿主重启刷新，新建聊天不够。MVP 安全性正确，但工作台应提供显式“刷新/重开 Workspace composition”动作。
- 不完全符合：原用例把只读 hook、Reviewer 回读、Consumer index-plan 的权限矩阵定义错了；任务模板已修正。

逐角色原始任务在 `dsh-plugin/scripts/browser_all_actions_tasks.json`；trace 对账器在 `dsh-plugin/scripts/browser_all_actions_oracle.py`。
