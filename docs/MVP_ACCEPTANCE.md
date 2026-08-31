# Knowledge Catalog MVP 验收

本页只回答一件事：当前实现是否足以让知识接入方发布可验证知识，并让知识消费方在固定版本上发现、检索、读取和溯源。

## 结论与边界

| 形态 | 当前结论 | 承诺边界 |
|---|---|---|
| 单实例 Server/Client 参考实现 | **MVP 合格** | 本机部署也由 Client 经 typed API 进入 Server；Dolt/Gitea 都提供 Snapshot authority 与精确 Knowledge 回读；SEARCH/RELATIONS 只经 exact-basis Retriever 发现候选，再按同一 commit 回读 Canonical。未配置对应能力时明确失败，不扫描降级 |
| 共享服务试点 | **有条件可用** | 需部署方提供 TLS、可信认证器、备份与单实例写入约束；Gitea 认证和远程 authority 可用，但不是完整生产平台 |
| 多实例生产服务 | **尚未验收** | 跨进程幂等/租约、独立 Catalog/Knowledge 服务部署、SDK/MCP、容量与故障演练仍不在当前保证内 |

这里的 “MVP 合格” 不表示每个 adapter 或宿主都已生产就绪。MVP 的判断标准是两条用户旅程，而不是包数量或命令数量：

1. **接入方**能从空环境建立 Repository，把带身份、Schema 和来源的知识写入，并从权威 commit 读回验收。
2. **消费方**能先发现 Workspace，再把本次任务固定为 `ResolvedWorkspace`，在同一 pin 上 SEARCH、READ 和 GET_PROVENANCE。

## 最短角色旅程

本机只是部署位置，不是另一套业务调用模式。首次启动时，宿主用 `kc local` 准备 Home 和第一个管理主体；此后所有 Catalog、Writer、Knowledge 和 Workspace File 请求都走 Server：

```bash
kc local init --home .kc --catalog acme/catalog
kc local repository attach --home .kc --repo kr://acme/public/core
kc local grant bootstrap --home .kc --principal user:local-admin
kc serve --home .kc                         # 终端 A

export KC_SERVER_URL=http://127.0.0.1:8080  # 终端 B
export KC_AS=user:local-admin
```

### 知识接入方

最小可读闭环是宿主 attach 之后，Client 执行 `kc writer put → kc knowledge read --repo`。Workspace 面向消费组合，不是写入前置条件。

```bash
# 需要 SEARCH 时先发布 schema/*；只做精确 READ 时可以不声明检索字段。
kc writer put --command-id schema-1 --repo kr://acme/public/core \
  --object schema/runbook.body \
  --value '{"entity":"Runbook","pattern":"record","fields":{"body":{"type":"string","access":["text"]}}}'

kc writer put --command-id source-1 --repo kr://acme/public/core \
  --object runbook/payment-oncall --schema-ref schema/runbook.body \
  --value '{"body":"切换支付流量前先检查冻结窗口"}' \
  --origin-kind SOURCE --source-ref file:///source/runbooks/payment-oncall.md

kc knowledge read --repo kr://acme/public/core --ref refs/heads/main \
  --object runbook/payment-oncall
kc knowledge provenance --repo kr://acme/public/core --ref refs/heads/main \
  --object runbook/payment-oncall
```

批量文件或外部源仍只有一条写边界：`ingest` / `connector.Preview` 生成 ChangeSet，人工或系统检查后由 `commit --command-id` 提交。采集器、源凭证和业务映射留在底座之外。

### 知识消费方

消费入口是 Workspace。不要让调用方猜 Workspace ID，也不要跨多条命令各自追随 `latest`。

```bash
kc catalog show                         # 发现可用 workspaces 与成员仓
kc catalog workspace resolve --workspace agent > pin.json  # 每次任务固定一次坐标
kc operations projection sync --repo kr://acme/public/core --ref refs/heads/main
kc knowledge search --workspace agent --pin pin.json --query 冻结窗口
kc knowledge read --workspace agent --pin pin.json --object runbook/payment-oncall
kc knowledge provenance --workspace agent --pin pin.json --object runbook/payment-oncall
```

SEARCH 命中必须从同一 basis 回读 Canonical。`partial` 必须附带 claims；能力不足返回 `CAPABILITY_UNSATISFIED`，不能伪装成零命中。精确 READ 无法诚实表达缺失成员时 fail closed。

## 产品 MVP 必须满足

### 接入方

| ID | 用户结果 | 机器可判定条件 |
|---|---|---|
| P1 | Repository 能独立接入 | `kc local repository attach` 后可按 Repository ID 解析 authority；Catalog 只登记，不复制正文 |
| P2 | 身份不依赖路径 | 文件移动后 `object_id` 和 KnowledgeRef 不变 |
| P3 | 写入可安全重试 | 同 `command_id` + 同 digest 返回原 Receipt；异 digest 返回 `IDEMPOTENCY_CONFLICT` |
| P4 | 并发写不静默覆盖 | 过期 `expectedTargetCommit` 返回 `NON_FAST_FORWARD`；失败无部分提交 |
| P5 | Schema 与正文同仓解析 | 带 `schema_ref` 的 PUT 只能引用 target Repository 可解析的 `schema/*` |
| P6 | 来源可验收 | READ 返回固定 commit 的值；GET_PROVENANCE 返回各知识单元的来源信封 |
| P7 | 批量接入先预览 | `ingest` / `connector.Preview` 不写仓；只有 Writer `commit` / `propose` 改 Snapshot |

### 消费方

| ID | 用户结果 | 机器可判定条件 |
|---|---|---|
| C1 | 能发现消费入口 | `kc catalog show` 返回 `catalogId`、repositories 和 workspaces |
| C2 | 一次任务版本一致 | `kc catalog workspace resolve` 返回 `{repo → commit}` 与 `pinId`；所有消费命令接受同一 `--pin` |
| C3 | 多仓读取不覆盖 | 同 `object_id` 的成员结果并集返回，public/group/personal 不互相覆盖 |
| C4 | 搜索结果可信 | Provider 只给 CandidateRef；公开 hit 在 SearchView basis 回读 Canonical，并带 version/evidence/completeness |
| C5 | 能区分空、缺能力和部分结果 | 零命中、`CAPABILITY_UNSATISFIED`、`partial + claims` 形状不同 |
| C6 | 可追溯 | READ、SEARCH、LOG 和 GET_PROVENANCE 都绑定明确 Repository commit |
| C7 | 授权不越层 | `read-workspace` 不隐式授予成员 Repository 的 `read`；裸 READ 不返回假完整结果 |
| C8 | Agent 能解释并选入口 | 自然语言询问核心概念、接入边界或 SEARCH 缺能力时，Agent 只加载随包 Skill 即给出可判定的正确答案和最小下一步 |

### 服务与运维

| ID | 用户结果 | 机器可判定条件 |
|---|---|---|
| S1 | Transport 唯一 | 业务 CLI 是 typed Client，即使本机部署也不打开 Home；HTTP route 调用共享应用服务，不依赖 CLI parser/command table |
| S2 | 身份来源可信 | 每个 Server 请求都有 principal；新 Home 只能用一次性 `kc local grant bootstrap` 建立首个管理主体，后续授权经 Server；认证模式从可信认证器注入并拒绝伪造身份 header |
| S3 | 可判断存活与就绪 | `/livez`、分 surface `/readyz`、`/metrics` 不依赖知识响应正文 |
| S4 | 权威与派生可区分 | Snapshot 是权威；索引可丢可重建，且暴露 basis/lag/capability |
| S5 | 分层可执行 | `internal/arch` 阻止 Catalog 感知知识协议、Writer 依赖 Retrieval 等反向依赖 |

## 自动化证据

```bash
make test          # 临时 OpenSearch + component + boundary + 应用/transport 合同
make test-cover    # short suite、公开动词覆盖和 statement coverage 门禁
make test-race     # 并发敏感包的 race detector
make test-plugin   # DSH MountController、Skill、只读人用浏览、构建与包内容
make test-kcfs-e2e # Docker Linux /dev/fuse：kcfs + DSH MountController 真实生命周期
make test-agent-e2e # 真实模型六角色；需要 dsh + 模型凭证，禁止 host/filesystem 旁路
make test-agent-ux-e2e # 真实模型概念问答；检查 Skill trace、语义组和零旁路
make test-service-e2e # 真实 Gitea + OpenSearch、双身份 HTTP 旅程
make test-all      # 再验收真实 Gitea / Dolt / OpenSearch / Linux FUSE
```

关键证据入口：

- `cli/mvp_acceptance_test.go`：从空 Home 固定本页两条最短角色旅程；
- `cli/service_roles_live_test.go`：真实 Gitea 认证、Dolt/OpenSearch 上的 provider/consumer 独立身份、固定 pin 与更新隔离；
- `knowledge/writer/*_test.go`：P2–P7；
- `snapshot/commandlog/*_test.go`：跨写面的 command-id claim、重放和冲突；
- `catalog/*_test.go`、`cli/consume_flow_test.go`：C1–C7；
- `index/*_test.go`、`retrieval/*_test.go`：候选回读、basis、能力与 continuation；
- `cli/user_journey_test.go`：通过测试专用 embedded seam 验证共享应用语义；`cli/serve*_test.go` 和 remote CLI 测试验证产品 Server/Client 边界；
- `cli/command_evidence_test.go`：以生产 `cliSurface` 为分母的逐命令成功与风险分级边界报告；
- `cli/http_contract_inventory_internal_test.go`、`cli/http_surface_coverage_test.go`：以生产 route registry
  为分母的 57 条 HTTP 路由所有权、method、namespace 与 HTTP/Client 成功语义；
- `dsh-plugin/scripts/agent-scenarios.json`：真实 Agent 验收的机器可读分母，登记六个核心角色、
  四个首次使用/概念问答和 `DW-AGENT-01` 数仓 companion；runner 与清单漂移立即失败；
- `dsh-plugin/scripts/e2e_agent_roles.py`：真实 Agent 分别完成 source 发布、Workspace 治理检查、
  固定 pin 读取、audit/log/provenance 审计、坐标冲突恢复与越权写拒绝；每个角色保存回答和
  Skill/shell trace，最终状态 oracle 同时证明合法写入生效、越权写入未污染权威状态；
- `dsh-plugin/scripts/e2e_agent_questions.py`：真实 Agent 回答消费者心智模型、提供方
  接入边界和缺能力恢复问题；每题保存回答、Skill-only trace 和确定性语义 oracle；
- `internal/arch`：分层与术语守卫。

`make test` 通过证明共享应用语义、分层和 typed transport 合同；`make test-service-e2e` 是完整 Server/Client 产品旅程的 live 证据。依赖外部服务或 Linux FUSE 的能力，只有对应 live 测试真实通过才可对外宣称；SKIP 不是 PASS。

## 当前已知缺口

- State Binding 已有独立动态投影和双 basis，但 Stream window、持久化 observation generation 与多实例生命周期仍未验收。
- `kc serve` 已按正式 namespace 形成模块化单体；handler 不读 CLI command registry，进一步拆成独立进程仍是部署选择。
- command-id 能覆盖当前进程/共享日志的重试语义，但多实例协调、分布式租约和灾难恢复尚未形成生产验收。
- command-id / Receipt 目前覆盖知识写面；authority attach、grant 等管理写入还没有统一的重放合同。
- 已有 Go typed client；尚无多语言 SDK、MCP Gateway。Workspace 发现入口是 `kc catalog show` 或 `/catalog/v1`。
- Gitea 提供 Snapshot/File Gateway，但尚未提供不扫描的 layer ② Unit/Schema locator，因此不能宣称具备 Gitea Knowledge READ/SEARCH 能力。
- Gitea/Dolt 等 authority 需要 `make test-all` 的真实环境证据；`make test` 已用临时 OpenSearch 验收检索语义，但不能替代生产容量、备份、升级和故障演练。
- Gitea adapter 为原子 ref CAS 使用短生命周期 `kc-wip/*` branch；Gitea 1.26 的异步 action notifier 可能在清理后记录“ref 不存在”，不影响 commit/ref 结果，但生产日志治理仍需改用无临时 branch 的底层 commit API。
- Linux 宿主 VFS 是可选文件体验，不是接入或消费协议成立的前提；首版只读。

## Linux VFS 子验收

VFS 的目标是把 Workspace 的多个 Repository 子树投影到已有项目目录。它不增加知识语义，也不把编辑器写文件升级为新的 Write Surface。

```text
现有项目根
├── 用户原有文件
├── vendor/policy/       <- repo A: policy/
├── docs/catalog/        <- repo A: docs/
└── schemas/shared/      <- repo B: schema/
```

每个 `kcfs` 进程只解析一次 Workspace selector；所有 mount 共用该 pin，进程退出前不跟随 Repository HEAD。

| ID | 结果 | 机器可判定条件 |
|---|---|---|
| V1 | 附着任意已有项目 | 项目根只要求是可访问目录；非 mount 内容的 inode/bytes 不变 |
| V2 | 多目录组合 | 每个 `WorkspaceSource.Path` 是独立 mountpoint；可来自不同 Repository |
| V3 | 同仓多子树 | 同一 Repository 可投影多个不重叠的 `SubPath`；pin 中仍只有一个 commit |
| V4 | 一致视图 | `cat`、`rg`、IDE 和 Agent 读到相同 bytes；所有 mount 使用同一 pin |
| V5 | 固定版本 | mount 期间上游 ref 推进不改变 bytes；重启 `kcfs` 后才解析新 pin |
| V6 | 只读 | create/write/truncate/rename/remove 均失败，Repository ref 和原文件不变 |
| V7 | 授权 | 先检查 `read-workspace`，再逐 Repository 检查 `read`；无权成员不进入 plan/mount |
| V8 | 安全路径 | 拒绝绝对路径、`..`、反斜杠、NUL、根挂载、重叠 mount 和 symlink 穿越 |
| V9 | 宿主失败可解释 | 缺 `/dev/fuse`、`fusermount3`、TreeStore capability 或非空 mountpoint 时明确失败 |
| V10 | 无 Agent 专用 VFS | DSH 不替换标准 filesystem/search 工具，不导出第二套 `loom-fs` / `loom-search` |
| V11 | 首次使用可发现、可恢复 | 新项目的“知识”侧栏默认启用但折叠且不预扫；未连接时显示恢复动作；Skill 从自然语言引导 SEARCH→Canonical READ，不要求用户先懂命令 |

环境要求：Linux 可访问 `/dev/fuse`，安装 `fusermount3`；容器显式暴露设备和挂载 capability；每个 mountpoint 不存在或为空。首版不支持单文件 mount，也不允许挂到项目根。

```bash
go test ./workspacefs ./catalog ./cli ./internal/arch -count=1
npm --prefix dsh-plugin run typecheck
npm --prefix dsh-plugin test
./scripts/e2e-kcfs-linux.sh
./scripts/e2e-kcfs-docker.sh
```

若未来提供可写文件体验，应使用显式 checkout/overlay + reconcile/commit，保留 `expectedTargetCommit`、`command_id`、逐 Repository 结果和冲突恢复。不能把普通 FUSE write 直接解释成协议 COMMIT，也不能伪装跨 Repository 原子事务。
