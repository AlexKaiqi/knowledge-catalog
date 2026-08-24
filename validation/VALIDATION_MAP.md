# Knowledge Catalog 用户验证地图

## 目标

验证对象不是一条 Demo，也不是某组业务数字，而是用户能否把知识底座用于日常工作。下面这条只是最短主链：

```text
建 Catalog → 建/挂 Repo → 上传知识 → 组成 Workspace
→ Agent 消费 → 修改知识 → 下游感知新版
```

完整验收以“角色想完成的任务”为单位。实现包、CLI 动词和 T1–T12 只作为证据来源，不能替代用户结果。

## 用户和验证口径

| 角色 | 真正关心的结果 |
|---|---|
| Catalog owner | 建立知识空间、接入仓、发权、收场，并能审计发生过什么 |
| Knowledge producer | 上传、同步或追加知识；重试安全；并发时不覆盖别人 |
| Workspace owner | 把个人、团队、公共知识组成一棵不复制内容的树 |
| Agent / consumer | 只凭 Workspace 进入，能找、读、理解和引用知识，不必知道 Repo ID |
| Maintainer / reviewer | 修改个人知识，或通过 proposal、证据和 gate 发布共享知识 |
| Downstream integrator | 获得新版通知；新任务见新版；旧任务和旧回答仍可复现 |
| Operator | 切换本地/远端介质、检查状态、处理失败、归档且不丢审计 |

每条旅程都必须同时验证：成功结果、失败边界、不可变项、可复现证据。只断言“命令退出 0”不算通过。

## 完整任务地图

| ID | 用户任务 | 必测路径 | 通过标准 | 关键负例 |
|---|---|---|---|---|
| U1 | 建立和管理 Catalog | 首次 init、多 Catalog、读当前态、审计、归档 | Catalog 有稳定身份和独立 git 历史；Catalog 不是知识 Repo | 重复/未知 Catalog、归档后继续改配方 |
| U2 | 接入知识 Repo | 新建 Repo、`--dir` 挂已有 Git、`--link` clone、Gitea 远端 Repo | Repo 独立版本和生命周期；已有 Git 源不被污染；远端与本地读取语义一致 | 把 Catalog/Stream/Redis/MySQL 当 Repo；无效 Git；重复挂载 |
| U3 | 发布和同步知识 | PUT/REMOVE、ChangeSet COMMIT、SOURCE、DERIVATION、APPEND、connector Preview→Commit | 只推进目标 Snapshot 或 Stream；身份、schema、来源可读；幂等和 CAS 生效 | stale base、同 ID 异内容、越界 reconcile、schema 不可解析、归档后写 |
| U4 | 组织自己的 Workspace | 单仓/多仓、根与嵌套 mount、recipe、同对象多来源、配方升级 | 一次解析得到 `{Repo→commit, AppendCuts}`；不复制知识；路径唯一路由；来源不覆盖 | 未挂 Repo、重复 source、重叠/无归属路径、退役 Workspace |
| U5 | 发现、读取和理解知识 | list/read/search/schema/provenance/log/diff/inspect/stream | 所有结果都来自本次 pin；搜索命中后回读 Canonical；来源和版本坐标可解释 | 未知对象与未知版本区分；未声明检索能力明确拒绝 |
| U6 | 让真实 Agent 进入 | checkout 文件树、HTTP VFS、DSH `ctx.fs`，带身份进入 | Agent 只看组合树即可 list/read/search/edit；无权成员不出现；错误映射稳定 | Agent 绕过 Workspace 选 Repo；无权读写；VFS 把目录当文件 |
| U7 | 编辑个人知识 | checkout→编辑→commit、VFS write/edit/remove、路径写回 | 只写路径所属 Repo；CAS 防止丢更新；成功后工作树干净 | 无归属路径、stale version、create-if-absent 冲突 |
| U8 | 协作发布共享知识 | propose→preview→validate/record→gate→merge | 发布前消费者仍见旧 main；证据满足后原子快进；新消费才见新版 | candidate/main 移动、证据失败或错绑、hook 不能冒充 gate |
| U9 | 分享、授权和撤销 | allow、allowed、whoami、revoke，Repo 与 Workspace 两层 | 只授权指定 principal/命令/范围；撤销立即生效；Catalog 之间不串权 | define Workspace 自动发权；permissions Aspect 绕过 allow；HTTP 身份丢失 |
| U10 | 感知更新并复现旧结果 | Snapshot 通知、索引推进、fresh resolve、旧 pin replay、Stream cut | 当前任务保持 P0/V1；新任务得到 P1/V2；旧 pin 重放仍是 V1；通知失败不回滚提交 | 会话中途跟 HEAD；索引回绕 live；新 append 泄漏进旧 cut |
| U11 | 处理多仓并发与失败 | 两个 mount 同时修改、单仓竞争、hook/backend 失败、重试 | 每仓独立结果；已成功仓不伪回滚；失败仓仍可恢复；错误码稳定 | 假装跨 Repo 原子事务；部分失败被吞掉；重试重复写 |
| U12 | 运营和收场 | status/inspect/audit、sync、retire/archive、本地/远端/scale adapter | 权威/索引/缓存/投影边界不混；归档后禁写但历史可追；adapter 契约一致 | Gitea 假装可 checkout；stub 空成功；缓存或索引成为权威 |

## 必须串行跑通的代表旅程

地图不是要求做笛卡尔积，但以下六条跨能力链必须有端到端证据。

### J1 首次使用

```text
空 Home → init Catalog → 新建 Repo → 上传知识 → 定义 Workspace
→ resolve/inspect → Agent 读取并取得 provenance
```

### J2 带入自己的知识

```text
已有普通 Git → mount --dir 或 --link → 组成 Workspace
→ Agent 读取 → 修改 → 只回写自己的 Repo
```

必须证明源 Git 不被加专有标记；`--link` 修改 clone，不修改源仓。

### J3 团队治理发布

```text
消费者在 P0 读 V1 → 维护者 propose V2 → preview/gate
→ merge → 原消费者仍用 P0 → 新消费者解析 P1 并读 V2
```

### J4 外部系统同步

```text
外部当前态 → connector Address 对账预览 → ChangeSet COMMIT
→ 后续 APPEND 事件 → checkpoint/cursor 前进 → 重放不重复
```

Connector 是墙外进程，不要求 `kc connector-run`；验收的是它能通过稳定写面进入。

### J5 分享与撤销

```text
owner 发 Workspace + Repo 读权 → Agent 消费 → owner revoke
→ 同一身份立即被拒绝 → 其它独立 grant 不受影响
```

### J6 多仓失败恢复

```text
Agent 同时改两个 mount → 第二仓基线被别人推进 → commit
→ 第一仓明确 APPLIED → 第二仓明确失败并保持 dirty → 用户可重做 diff
```

## 能力缺失如何处理

发现缺口时先分类，避免用文档掩盖实现问题：

1. 已承诺且阻断上述 U1–U12：补实现和用户旅程回归测试。
2. 已有实现但只有包级断言：补跨层验收，不重复造协议。
3. 明确降级：结果中必须出现 `Skipped` 或 `CAPABILITY_UNSATISFIED`，不能空成功。
4. 冻结的未来面：列入未支持清单，不拿来降低当前已承诺能力的结论。
5. 外部环境阻塞：保留可复跑命令和前置条件，不能写成 PASS。

当前冻结而非本轮承诺的面包括独立 `WATCH_UPDATES` 订阅、MCP、关系展开、树形 LIST、流 search/tail、StarRocks 列索引和规模化 Stream。Stream 的 continue/lookup/time window 已支持。下游更新的当前承诺是 post hook 推送 + 新命令重新 resolve + pin 可重放。

## 完成判定

- U1–U12 每项都有用户结果证据，不靠业务数字代替。
- J1–J6 至少各有一条自动化跨层旅程。
- 本地和真实 Gitea 至少各跑一次；Gitea 不支持 worktree 必须诚实降级。
- Agent 接缝必须经过真实 `kc serve` 和真实 `FileSystem` 实现；完整 LLM harness 还必须实际启动模型 Agent。
- 任何新增缺口必须落为测试、明确降级或冻结项，不能留成“应该可以”。

具体执行结果见 [PROGRESS.md](PROGRESS.md)，底层状态×操作全量目录见 [../../docs/TEST_CATALOG.md](../../docs/TEST_CATALOG.md)。TPC-H fixture 只给 U3/U5 提供真实数仓内容 oracle。
