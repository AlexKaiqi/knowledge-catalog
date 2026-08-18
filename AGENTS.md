# Agent 须知

这是 **Knowledge Catalog 通用知识底座**：一套 Catalog 协议的 TypeScript 参考实现（身份、来源、写边界、ViewGeneration、维护闭环）。不是检索应用，也不是某个开源元数据产品的 fork。

第一落地场景是 **数仓域**（物理表/列/作业/血缘 + 语义层指标/维度）。场景检出挂在本仓库的 gitignored 隐藏目录里，同一 Cursor 工作区能看到两棵树，**不要另开窗口、不要切分支**。

## 同一工作区里的两棵树

`.scenes/` 已被 `.gitignore` 忽略，不要放进 `.cursorignore`（agent 必须能看见）。不要在两个 worktree 检出同一分支。根目录 `src/` 和 `.scenes/data-warehouse/src/` 是同一仓库的两份拷贝，**改文件前先看路径**。

| 路径 | 分支 | 职责 |
|---|---|---|
| 仓库根（本目录） | `main` | 协议、store adapter、conformance |
| `.scenes/data-warehouse/` | `scene/data-warehouse` | 数仓域场景落地 |

发现缺口时：**判断归属 → 只改对应路径 → 在该树里提交 → 同步**。

- 协议/契约/Writer/Repository/View/T1–T12 不够 → 改 **仓库根** 并提交，然后把 main **合入**场景（见下）。
- 采集、source key、Recipe、源消息翻译、本地数据 → 只改 **`.scenes/data-warehouse/`**，不回写根目录。
- 场景本地数据、schema 草稿、源表清单、决策记录只放 `.scenes/data-warehouse/.data/`；SR 连接在 `.env`。都已 ignore，不要提交。
- 物理层业界对照与决策：`.scenes/data-warehouse/.data/decisions/physical-layer-industry.md`。
- **Schema 是知识**，不是项目源码。正式形态是知识 Repository 里的 `schema/*` 对象（Writer COMMIT，可 RESOLVE/READ/GET_PROVENANCE）。未入库前只能临时放 `.data/`，禁止 `schemas/`、`src/schemas/` 或任何会进 git 的路径。
- 场景树里改了 `src/contracts` / `src/api` / `src/adapters` / `src/catalog` / `tests/conformance` → 停手，把变更做到仓库根再合入。不要在场景分支演进协议。
- 搜索或打开 `src/**` 时，默认用仓库根；只有任务明确是场景落地才进 `.scenes/`。

同步方向只准 **main → scene**。随时合入用 merge（只合已提交的 main，工作区未提交的不算）：

```bash
git -C .scenes/data-warehouse merge main
```

底座不合并场景实现。不要对已共享的场景分支 rebase。场景侧其它 git 命令同样加 `-C .scenes/data-warehouse`。`git clean -fdx` 会删掉 `.scenes/`，不要对仓库根用 `-x`。

## 仓库根（main）可以改

```text
src/contracts/     身份、Surface、Repository、错误码
src/adapters/      FileGit、SQLite 投影
src/api/           Writer / Reader / ingest+reconcile / Refine
src/control-plane/ PROPOSAL → Preview → validateStructure / recordValidation → Merge
src/catalog/       ViewGeneration、联邦读、Promote / Release、git 登记表
tests/conformance/ T1–T12
docs/              设计、Aspect 读策略、kc 走通
```

`src/api/ingestion.ts` 的 `ingest` / `reconcile` 是 COMMIT 之上的薄编排，**不是**采集框架。不要在这里长仓库/数仓 connector。

## 不要做

- 不要在仓库根加 `src/collectors/**`、`tests/scenarios/**`、具体源系统客户端；这些只属于 `.scenes/data-warehouse/`。
- 不要把 schema 写成项目文件。Schema 是知识对象，走 Writer；草稿只放 `.data/`。
- 不要把 `.scenes/` 提交进 git，也不要写进 `.cursorignore`。
- 不要为场景新增 Write Surface。采集输出仍是 ChangeSet 预览，经 Writer `commit` / `append`。
- 不要把路径、URN、文件名当成 `object_id`。`object_id` 在文件 frontmatter；源系统标识是 source key，映射表属于场景侧。
- 不要用 PROPOSAL/MR 做无人值守同步。自动写入走 COMMIT；事件走 APPEND；历史是 git commit。
- 不要把 View 做成又一个 Repo，不要把 public 知识拷进 personal。用户看见的是 ViewGeneration。
- 不要按 public/group/personal 覆盖联邦读结果。
- 不要把 Projection/FTS 当权威。命中后回读 pinned commit 的 Canonical。
- 不要直写 git / 工作区文件来绕过 Writer。
- 不要新增通用 PATCH、跨 Repo 事务、运行时跟随 `latest`。
- 不要提交、不要改 git config，除非用户明确要求。

## 协议要点

- Catalog 语义只有一套。单 source 是 ViewGeneration 成员数为 1，不是另一套模式。
- 协议层只依赖 `Repository` 接口。当前 store 是 FileGit（Snapshot = git，Append = gitignored JSONL）。
- 写必须选唯一 target Repo + 一种 Surface：`COMMIT` | `PROPOSAL` | `APPEND`。变更代数只有 PUT / REMOVE。`PUT Aspect` 替换一个分区（DataHub MCP），不是通用 PATCH。
- 唯一键是 Address：`object_id` + `aspectName` + `memberKey`。同一 `object_id` 可有多个 Aspect 文件。禁止把 Entity blob 和 Aspect 混在同一对象上。
- Reader：`READ(ref)` 拼装（可 `AspectSelector`）；`readAddress` 读单单元。Projection 用同一 selector 编 FTS；`permissions` 等 ACL 投影不进索引。见 `docs/ASPECT_ACCESS.md`。
- `expectedTargetCommit` 过期 → `NON_FAST_FORWARD`；同 `command_id` 异 digest → `IDEMPOTENCY_CONFLICT`。重试用同一 command_id；内容变了换新 id 并重做 diff。
- DERIVATION 必须带固定 `inputViewReadVersionRef` + algorithm，否则拒写。源同步标 `SOURCE`。
- `COMMIT` 推 Ref；`promote` 只动 **Release**（发布名 → Generation）。Agent 用 `readRelease`，不要跟 `main`。
- `GET_PROVENANCE` 返回该对象各单元上贴的来源信封，不爬 `sourceRefs`，也不等于 git log。
- `PIN_VIEW` 把 ViewDefinition 的 selector 各解析一次并登记 Generation。`checkGeneration` / `validateStructure` 检查成员 repo 已挂载且 commit 存在。`recordValidation` 只绑定传入的 PASSED/FAILED，不跑测试套件。
- `LOG` 返回对象引入各 digest 的 commit（后续未改该对象的 commit 不占一条）。`DIFF` 是两个 pinned commit 上的对象值。`GET_PROVENANCE` 不是 git log。
- Catalog 工作区 namespace（`kc init --namespace`）和独立登记表已有；WriteBinding、跨进程幂等、HTTP/MCP 网关尚未实现。缺这些先问归属，再决定补 main 还是场景。

## 命令

```bash
npm install
npm run typecheck
npm test            # 只跑仓库根；.scenes 不收集
npm run kc -- help
```

CLI（`src/cli/`）把协议动词摊成命令。`.kc/workspace.json` 只记挂载的成员 repo；Catalog 登记表是独立 FileGit `.kc/repos/_catalog`（`kr://<namespace>/catalog`），不是成员 View 的 source。Writer 幂等日志是 `.kc/writer.json`。`promote --view` = `Catalog.publish`（先 pin 再 CAS Release）。`kc validate` 跑结构检查；`record-validation` 只记录外部套件结果。不要把采集器写进这个 CLI。

用 `.venv` 跑 Python（本仓库协议代码是 TypeScript）。T8 需要 Node 的 `node:sqlite`（建议 Node 22+）。

## 文档

- `README.md` — 结构与 conformance 表
- `docs/KNOWLEDGE_CATALOG_DESIGN.md` — 设计与 K-01..K-24；读协议见第 7 章
- `docs/ASPECT_ACCESS.md` — Aspect 读/检索业界对照与决策
- `docs/WALKTHROUGH_v5.1.md` — 用 `kc` 走通：操作与进入的状态
