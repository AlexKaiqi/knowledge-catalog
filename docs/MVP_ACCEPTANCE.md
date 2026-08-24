# Knowledge Catalog MVP 场景地图

状态：MVP 权威验收目标。实现、测试和文档都以本文件的用户结果为准。

## 1. MVP 结果

用户能启动一套持久化的 `kc` 服务，把普通 Git、FileGit、Gitea、Dolt
Repository 挂入 Catalog，用唯一的 Workspace 配方组合成一棵树，并通过
`dsh-plugin` 把这棵树挂进 DeepSeek Harness。真实模型 Agent 必须能够：

1. 发现和读取组合知识，回答可由仓内证据判定的问题；
2. 在可写 mount 中创建、修改、删除文件，运行开发任务并经 CAS 写回正确仓；
3. 接收另一个 Agent 写下的反馈，在后续独立任务中读取反馈并修正结果；
4. 使用 MVP 查询原语定位文件和知识；
5. 在更新、撤权、并发、服务重启和成员故障后给出可解释、可恢复的结果。

包级测试、mock HTTP、只验证 VFS 接口、只跑一次模型、复用旧状态，都不能单独
构成 MVP 验收。

## 2. 产品边界

- **唯一组合模型是 Workspace。** 不保留 View、Generation、Release 的公开兼容面。
- **普通 Git 是核心能力。** 没有 `object_id` 的仓也能 mount、组合、VFS、检出和写回。
- **知识能力渐进增强。** Address / Aspect / provenance / schema / search 叠加在普通
  Git 能力之上，不成为挂载前置条件。
- **运行形态是生产服务。** MVP 必须验证进程重启、持久状态、并发请求、身份传递、
  外部 Repository 和稳定错误；不是只可嵌入测试的内存对象。
- **Agent 是阻塞验收。** 使用真实 DeepSeek Harness、`dsh-plugin` 和已配置模型。
- **查询原语属于 MVP，但排在 Agent VFS 主链之后。** 先由真实任务暴露所需的
  list/glob/grep/rg/exact-read 能力，再补协议；MVP 不先建设通用 RQL、向量或图查询。
- 数仓、代码库、文档库只是验证内容；领域 connector 不进入 main 的通用产品面。

## 3. 完整场景地图

每个场景都必须有：成功结果、关键负例、状态不变量和机器可判定证据。

| ID | 用户场景 | 必须完成的结果 | 关键负例与不变量 |
|---|---|---|---|
| M0 | 从零启动生产服务 | 空目录初始化 Catalog；启动 `kc serve`；健康检查、身份和配置可读；重启后 Catalog、授权、幂等记录和仓挂载仍在 | 不能依赖测试进程内对象或旧 `.kc`；重启不能丢写入或改变 Repo HEAD |
| M1 | 接入 Repository | 新建 FileGit；挂已有普通 Git；clone `--link`；挂真实 Gitea；挂 Dolt；均保留独立版本和生命周期 | 不污染 `--dir` 源仓；Catalog/Stream/MySQL/缓存不能冒充 Repo；不支持的能力显式失败 |
| M2 | 组织 Workspace | 根 mount、嵌套 mount、只读和可写成员组成唯一树；配方可随 Git 分享；一次解析得到固定 `{Repo->commit, AppendCuts}` | 不复制成员内容；路径不能重叠或无归属；同仓不能重复；配方不发权 |
| M3 | DSH 挂载 Workspace | `dsh-plugin` 连接真实服务，把 Workspace 暴露为 Harness 文件系统；Agent 不需要 Repo ID 即可 list/read/write/edit | Agent 不得绕过 Workspace 选择 Repo；目录不能当文件；服务错误必须稳定映射到工具错误 |
| M4 | Agent 知识问答 | 真实模型从多个 mount 定位证据，回答固定问题，并返回足以核对的文件路径、版本或引用；冲突来源不被静默覆盖 | 答案不能来自 prompt 内泄漏或预置常识；无权成员不能通过答案、计数或错误差异泄漏 |
| M5 | Agent 开发写回 | 真实模型读取需求和代码，修改文件、运行测试、检查结果；create/edit/remove 写回路径所属 Repo；最终工作区与 Repo 状态一致 | 只读 mount 写入被拒；无归属路径被拒；stale base 不覆盖并发提交；重试不重复写 |
| M6 | 反馈闭环 | Agent A 产出；独立 Reviewer/Feedback Agent 读取产出并写反馈；新的 Agent 任务读取原结果和反馈，完成修正；三段结果和 commit 可追溯 | 不通过复用同一会话记忆作弊；反馈必须落入 Workspace 可见权威；未采纳反馈必须能由 oracle 判失败 |
| M7 | 查询原语 | DSH 任务可做目录枚举、glob、grep/rg、精确文件读取，并在需要时映射到知识级 exact read；结果固定在本次 Workspace pin | 查询不能绕过授权或 pin；不能把索引当正文；不支持的复杂查询明确拒绝，不能空成功 |
| M8 | 更新感知与复现 | Agent 任务在 P0/V1 开始；上游推进 V2 后本任务仍可重放 P0；新任务解析 P1 并看到 V2；Stream cut 同样不漂移 | 会话中途不能偷偷跟 HEAD；旧 pin 不能读到新 append；新版本不能改写引用方 Repo |
| M9 | 身份、授权和撤销 | owner 发 Workspace + Repo 权限；指定 Agent 可读/写允许范围；撤权后新调用立即拒绝；其他 grant 不受影响 | Workspace 配方、知识中的 permissions Aspect、模型身份声明都不能自行发权；HTTP 身份不能丢失 |
| M10 | 共享知识治理发布 | 消费者在旧 pin 读 V1；维护 Agent propose V2；preview、validation、gate、merge 后，新任务读 V2，旧任务仍读 V1 | candidate/main 移动、错误证据、缺 gate 必须拒绝；hook 不能冒充 gate；发布不复制到个人仓 |
| M11 | 并发、部分失败和恢复 | Agent 同时改多个 mount；一个成功、一个因竞争失败时逐仓报告；失败仓保持可恢复差异；重新读取、重做 diff 后可继续 | 不伪装跨 Repo 原子事务；成功仓不假回滚；同 command-id 异内容、服务超时和重放不能产生重复提交 |
| M12 | 运营与生命周期 | `status/inspect/audit` 能解释服务、Catalog、Workspace、pin、Repo 和索引状态；服务重启后继续；retire/archive 后禁新操作但历史可查 | Gitea 不假装本地 worktree；Dolt/Gitea/FileGit 语义不能漂；归档不能删除审计证据 |
| M13 | 外部知识采集 | 墙外 Collector 拉取源当前态，按 Scope 预览 patch/reconcile，再经 Writer COMMIT；新增、修改、删除和无变化均有可判定结果；Agent 新任务看到新 SOURCE 快照 | Collector 不成为第四个写入面；超 Scope、缺 `sourceRefs`、过期 base 和源拉取失败不得部分写；空预览不产生 commit；同一源批次重放不重复写 |

### 3.1 地图如何落到每一轮实跑

`dsh-plugin/scripts/accept-mvp.sh` 是唯一正式入口。它对每个 R run 先跑通用
Collector 对账契约，再调用 `e2e-dsh.sh` 建全新服务和真实 Agent 旅程。下表不是“已有单测”
清单，而是每轮 clean room 都必须产生的证据：

| 地图 | 每轮实际路径与 oracle |
|---|---|
| M0 | 构建当前源码；空 `KC_HOME` 启服；`/health`；写入 Workspace、grant、receipt、pin；杀进程重启；逐项比较重启前后状态；48 个并发 HTTP 读 |
| M1 | 每轮都建 FileGit、已有普通 Git 与 `--link` clone；断言源 Git 未被打 stamp/Stream；R2 另建 Gitea 容器和远端仓，R3 另建原生 `.dolt` 数据库；checkout 能力必须如实报告 |
| M2 | 建根、嵌套、只读、可写 mount；resolve 保存仓 commit 与 AppendCuts；重叠路径、无归属路径、旧 `view` flag/verb 都必须失败 |
| M3 | 新 DSH profile 加载本轮构建的插件；dump-config 证明 `loom-fs` 生效；后续全部 Agent 从 Workspace 文件系统操作，不向 prompt 泄漏 Repo 路由 |
| M4 | J1 publisher 写不在 prompt 中的唯一证据，独立 consumer 用 VFS 定位并回答；外部 oracle 比对答案、路径；另以无权身份证明不可见且不可写 |
| M5 | J2 Agent 读需求、改代码、跑测试、写回；另跑只读拒绝、stale base、幂等冲突、目录当文件、无归属，以及 Agent remove 后 live oracle |
| M6 | J3 author、reviewer、reviser 三个独立 DSH 会话；反馈必须先持久化，新会话修订产物；外部 oracle 检查必要内容和禁止内容 |
| M7 | 查询 Agent 必须实际调用 list、glob、grep、rg、exact Read；知识 Search 由 Go 全量契约验证；结果仍经 Canonical 回读 |
| M8 | J4 让旧 Agent 跨外部更新保持 V1、新 Agent 看到 V2；另追加 Stream V2，旧 pin 只得 V1、live 得 V1+V2 |
| M9 | J5 授权两个主体，撤销其一；同身份新调用立即拒绝，另一主体继续成功；HTTP `X-Kc-As` 贯穿服务和插件 |
| M10 | J7 Maintainer propose，Reviewer 记录证据，gate 后 merge；错误/缺失证据和 candidate 移动先失败；旧 Agent pin 仍 V1，新 Agent 为 V2 |
| M11 | J6 Agent 跨两个 mount 写；外部制造第二仓竞争；断言逐仓 APPLIED/NON_FAST_FORWARD、无假回滚，随后重读并恢复；并发同 command-id 只产生一次 commit |
| M12 | 每轮读取 status/inspect/audit/checkout；Repo、Workspace、Catalog 依次 retire/archive；新操作失败但 audit 增长且可读；保存首尾 pin、状态、服务日志 |
| M13 | 每轮先跑 `TestConnectorChangeJourney` 覆盖 Scope、sourceRefs、stale、空 diff、重放和零部分写；J8 再由真实 Agent 验证 S1→S2 的增改删、旧 pin 和 SOURCE 来源 |

R1/R2/R3 的差别只在被验证的外部 Snapshot adapter；其余地图和 oracle 完全相同。
正式输出目录必须包含 `connector.log`、`run.log`、`home/serve.log`、首尾状态与 pin、
模型 patch、Agent transcript 清单和压缩 transcript。缺任何一类，整轮不计数。

## 4. 必须串行跑通的真实 Agent 旅程

### J1 挂载并回答

```text
空状态 -> 启动服务 -> 挂多个 Repo -> 定义 Workspace -> DSH 挂载
-> Agent 用 list/rg/read 找证据 -> 回答固定问题 -> oracle 核对答案与来源
```

### J2 开发并写回

```text
空状态 -> 挂只读知识仓 + 可写代码仓 -> Agent 阅读任务与约束
-> 修改代码 -> 运行测试 -> VFS CAS 写回 -> 核对只有目标 Repo 前进
```

### J3 反馈再开发

```text
Agent A 产出 V1 -> Feedback Agent 在独立会话写反馈
-> Agent B 在新会话读取 V1 + 反馈 -> 产出 V2 -> oracle 证明反馈被正确采纳
```

### J4 更新与旧任务复现

```text
Agent 任务固定 P0 -> 上游发布 V2/追加事件 -> 原任务重放仍是 V1
-> 新 Agent 任务解析 P1 -> 回答必须体现 V2
```

### J5 授权撤销

```text
授权 Agent -> DSH 可读写 -> revoke -> 同一身份新调用立即失败
-> 其他独立 Agent/Repo grant 仍有效
```

### J6 多仓竞争恢复

```text
Agent 修改两个 mount -> 外部推进第二仓 -> 提交
-> 第一仓 APPLIED、第二仓 NON_FAST_FORWARD 且差异保留
-> Agent 重新读取、重做修改并完成第二仓
```

### J7 受治理发布

```text
Agent 读取 V1 -> Maintainer Agent propose V2 -> Reviewer 产出证据
-> gate + merge -> 新 Agent 读取 V2 -> 旧 pin 仍回答 V1
```

### J8 Collector 更新闭环

```text
外部源 S1 -> connector 译成 Desired + Observed -> Preview -> SOURCE COMMIT
-> Agent 读到 V1 -> 外部源发生新增/修改/删除 -> reconcile 预览
-> 空预览跳过，非空预览用稳定 command-id COMMIT
-> 旧 Agent pin 仍读 V1，新 Agent 读 V2 并返回 SOURCE 证据
-> 重放同批次不新增 commit；超 Scope/无 sourceRefs/过期 base 由 oracle 证明零部分写
```

## 5. 从零重复验收

MVP 完成前，J1-J8 与 M0-M13 必须在以下三次**独立 clean-room run** 中全部通过：

| Run | Repository 拓扑 | 目的 |
|---|---|---|
| R1 | FileGit + 已有普通本地 Git | 验证最低依赖、本机持久化、普通 Git 不受污染 |
| R2 | FileGit + 全新真实 Gitea | 验证远端身份、CAS、网络服务、无本地 worktree 的降级和恢复 |
| R3 | FileGit + 全新 Dolt | 验证替换 Snapshot Adapter 后用户语义不变 |

每次运行必须满足：

1. 使用新的临时 `KC_HOME`、Catalog、Repository、Workspace、服务端口和 DSH 会话；
2. R2/R3 使用新的远端仓或数据库状态，不继承上一次 refs；
3. 重新构建当前源码的 `kc` 与 `dsh-plugin`；依赖下载缓存可复用，产品状态不可复用；
4. 使用真实已配置模型，不使用录制响应或 mock LLM；
5. 记录服务日志、Agent transcript、工具调用、Pin、每仓 before/after commit、测试输出和失败恢复过程；
6. 每个 oracle 独立判定，不能只以 Agent 自述“已完成”为准；
7. 三次必须连续全绿。任一次失败都修复后从 R1 重新开始计数。

## 6. 完成判定

只有同时满足以下条件，长期目标才可以结束：

- M0-M13 没有 `应该可以`；每项是 PASS、明确的 MVP 非目标，或可复现失败。
- J1-J8 在 R1-R3 三次独立 clean-room run 中连续通过。
- 问答、开发、反馈三类任务都由真实 DSH 模型 Agent 完成并由外部 oracle 核验。
- FileGit、Gitea、Dolt 的挂载、读写、CAS、重启语义符合各自能力，不静默降级。
- Workspace 是唯一公开概念；旧 View/Generation/Release 入口、文档和落盘兼容面已删除。
- 查询原语足以支撑验收任务，且 pin、授权、Canonical 回读边界有回归证据。
- `go test ./...`、dsh-plugin 测试和 clean-room 验收全部通过。
- 文档描述、CLI Help、HTTP 动词和真实行为一致。
