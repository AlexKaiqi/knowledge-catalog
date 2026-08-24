# Knowledge Catalog 用户验证结果

> 执行时间：2026-08-24 CST
>
> 结论：**完整用户地图已经建立并执行；参考实现的核心用户任务均有通过证据。真实 DSH 文件系统接缝和带模型的六角色 headless Agent 治理闭环均已通过。规模化 Stream、StarRocks、MCP、独立 WATCH 等仍是明确冻结能力，不计作已支持。**

## 地图状态

| ID | 用户任务 | 状态 | 本轮或既有证据 | 未覆盖 / 降级 |
|---|---|---|---|---|
| U1 | 建立和管理 Catalog | **PASS** | init、多 Catalog 隔离、read/audit、retire/archive、公司工作台 S0–S6 | — |
| U2 | 接入知识 Repo | **PASS** | 新建；已有 Git `--dir`；本轮真实 `--link` clone；真实 Gitea T12 和混合 Workspace | Gitea 无本地 worktree，明确 `Skipped` |
| U3 | 发布和同步知识 | **PASS** | PUT/REMOVE/ChangeSet、SOURCE/DERIVATION、APPEND、connector Preview→Commit、幂等/CAS/schema/provenance | 没有内置 connector runner，这是分层设计 |
| U4 | 组织自己的 Workspace | **PASS** | 单/多仓、根/嵌套 mount、recipe、固定 commit/AppendCuts、同对象多来源 | — |
| U5 | 发现、读取和理解 | **PASS** | read/list/search/schema/provenance/log/diff/inspect/stream；索引命中回读 Canonical | 关系展开、树形 LIST 未实现 |
| U6 | 真实 Agent 进入 | **PASS** | DSH 插件真实 `kc` 集成测试 45/45；六个独立 headless Agent 均加载 bundled Skill 并完成治理闭环 | — |
| U7 | 编辑个人知识 | **PASS** | checkout 写回、VFS write/edit/remove/CAS；DSH FileSystem 实际读写编辑 | — |
| U8 | 协作发布共享知识 | **PASS** | proposal→preview→validation→merge；本轮新增完整 HTTP 旅程 | — |
| U9 | 分享、授权和撤销 | **PASS** | 本轮新增 allow/allowed/whoami/revoke 全链，撤销立即拒绝；HTTP `X-Kc-As` | — |
| U10 | 更新感知与复现 | **PASS** | Snapshot pin、增量索引、post hook、旧 pin replay、Stream cut 不漂移；真实 Consumer 在一次任务内读取冻结 Workspace | 无独立 WATCH API |
| U11 | 多仓并发与失败恢复 | **PASS** | 本轮新增两个 mount：一仓成功、一仓竞争失败且保留 dirty；幂等/重做 diff | 跨 Repo 事务明确不提供 |
| U12 | 运营和收场 | **PASS（参考实现）** | status/inspect/audit/sync/lifecycle；FileGit/Dolt/Gitea/JSONL 契约 | scale Stream、StarRocks 是 stub；Gitea checkout 降级 |

## 本轮补出的真实能力缺口

这次不是只补文档。按用户公开词汇执行时发现两个命令只接受内部兼容名 `--workspace`：

- `allow` / `allowed --workspace` 没有把 Workspace 写进授权范围。
- `preview --workspace` 被错误判定为缺少 `--workspace`。

已修复为统一走 Workspace 解析，并用用户旅程测试钉住。否则访问管理和 HTTP 治理链在文档上存在、用户实际却走不通。

## 本轮新增用户旅程

`cli/user_journey_test.go`：

| 测试 | 跨过的能力 |
|---|---|
| `TestUserJourneyLinkExistingRepository` | 已有 Git → clone 挂载 → Workspace → VFS 消费；源 Repo HEAD 不变 |
| `TestUserJourneyManageAgentAccess` | grant → allowed/whoami → Agent read → revoke → 立即拒绝；独立 Repo grant 保留 |
| `TestUserJourneyKnowledgeGrantDoesNotAuthorizeAccess` | permissions Aspect 中有 SELECT，但没有 Repo allow 时仍不可见；显式发权后才可读 |
| `TestUserJourneyStreamPinDoesNotDrift` | P0 固定 AppendCut → live append → P0 仍 1 条 → fresh consumer 见 2 条 |
| `TestUserJourneyCrossRepoWriteReportsPartialOutcome` | 两仓编辑 → 一仓成功、一仓竞争失败 → 明确逐仓结果和可恢复 dirty |
| `TestUserJourneyUpstreamUpdateDoesNotRewriteReferencingRepository` | 上游 selector 跟到 V2，但引用方 Repo commit 和知识保持不变 |
| `TestUserJourneyFrozenCommandsDoNotPretendToWork` | capabilities/relation/watch/tree 等冻结入口明确 `USAGE_INVALID` |
| `TestUserJourneyGovernedPublishOverHTTP` | HTTP init/put/workspace/resolve/inspect/propose/preview/validation/merge/read V2 |

实际执行结果：8/8 PASS。HTTP 旅程还覆盖了 Workspace stream 和 checkout，与 CLI 共用相同语义。

外部系统同步的代表旅程也已通过 `./validation/playbook.sh all` 黑盒执行：真实 MySQL 当前态进入知识 Repo，随后真实 binlog update 只应用一次、重复事件重放一次、旧 position 被拒绝，知识 commit、Stream 和 connector checkpoint 按顺序推进。TPC-H 数值只承担这条旅程里的内容校验。

## Agent 与远端实跑结果

### DSH Agent 文件系统接缝

以当前 `main` 编译 `kc`，运行插件完整测试：

```text
Test Files  7 passed (7)
Tests      45 passed (45)
```

集成套件实际启动 `kc serve`，由真正的 `LoomFileSystem` 完成跨 mount 的 write/read/list/stat/edit、CAS、错误映射和大小限制；不是 mock HTTP。

`dsh-plugin/scripts/e2e-agent-roles.sh` 从空工作空间启动 Owner、Producer、Reviewer、Consumer、Auditor 和 Unauthorized Actor 六个独立模型 Agent。每个 session 的 trace 均证明实际加载了 bundled `knowledge-catalog` Skill 并调用 `kc`；最终 oracle 证明 Proposal 未提前移动 main、合并值和 provenance 正确、未授权写入前后 HEAD 不变。

### 真实 Gitea

Docker Gitea 本轮实际运行，以下全部 PASS：

```text
catalog.TestLoomAcceptanceMixedGiteaAndLocal
cli.TestVFSOverHTTP
gitea.TestT12GiteaContract
gitea.TestGiteaReadPinnedCommitNotWorktree
gitea.TestGiteaRawFileStoreRoundTrip
```

证明了本地 + 远端混合组合、联邦读、HTTP VFS 定向写、pinned commit、CAS、提案、幂等、schema 和归档。远端 Gitea 不能物化为本地 worktree，结果明确标为 `Skipped`，但仍可经 Writer/VFS 写回。

## 已执行命令

```bash
go test ./cli -run 'TestUserJourney' -count=1 -v
go test ./scenario -run TestCompanyWorkbench -count=1 -v
go test ./catalog ./cli ./writer ./local ./gitea -count=1
go test ./reader ./index -count=1
go test ./catalog -run TestLoomAcceptanceMixedGiteaAndLocal -count=1 -v
go test ./cli -run TestVFSOverHTTP -count=1 -v
go test ./gitea -count=1 -v
go test ./... -count=1
./validation/playbook.sh all
```

DSH 的等价干净环境流程是：复制 `package*.json`、tsconfig、src、test 到临时目录，`npm ci --legacy-peer-deps`，以 `KC_BIN=<当前源码编译出的 kc>` 运行 `npm test`。

## 尚未声称支持的能力

这些不是“测试漏了”，而是当前产品边界；调用时必须明确失败或没有入口：

- 独立 `WATCH_UPDATES` 订阅 API（当前用 post hook 推送）。
- MCP facade、关系展开、树形 LIST、流 search/tail；durable time window 已支持。
- scale Stream 与 StarRocks 列索引（stub）。
- 自动 Fork 三方同步和 Vendor 只读副本。

`SURFACE_MISMATCH` 与 `SCHEMA_UNSUPPORTED` 目前是不可达的冻结错误码：公开请求结构不允许 Surface 冲突，参考实现也不承诺通用 schema validator。重复 Address 和 blob/aspect 混用已统一为 `OBJECT_ID_CONFLICT`；Base Stream 的 ordering profile 是 `NONE`，producer checkpoint 回退由数仓 connector 独立拒绝，不在通用 APPEND 中伪造 position 字段。Scale Stream 和 StarRocks 仍未实现，但写、读和 opener 都明确返回 `CAPABILITY_UNSATISFIED`，不会再把本地 JSONL 或空结果伪装成 scale 能力。

## 数仓内容验证的位置

TPC-H 的行数、金额和 binlog position 继续放在 `fixtures/tpch-sf001/expected/`，作用是证明数仓知识内容正确。它们会作为 U3/U5 的内容样本，不再代表 Catalog、Workspace、Agent、授权或更新感知是否完成。
