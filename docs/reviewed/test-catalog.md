# 验证体系、用例规范与执行证据

本文拥有验证方法、用例补充规则与证据判读。具体库存由代码与场景清单生成，执行结果按 run 保存；不在本文维护“本仓当前全绿”的人工结论。

范围：仓库根协议（`kernel` / `catalog` / `writer` / `reader` / `controlplane` / `cli`）。

对照：场景目录树（操作→进入的状态；`python3 .data/scenes/tree.py`）、`cli/mvp_acceptance_test.go`（接入/消费最短闭环）、`cli/user_journey_test.go`（通用端到端旅程）、T1–T12（`README.md` Conformance）。公开命令是否落到场景上由 `TestSceneFeaturesCoverPublicCLI` 钉住，不另维护一份覆盖清单。

本目录按当前 Snapshot + Aspect Binding + 声明式 AccessSpec 契约验收。旧 APPEND/Stream/AppendCuts surface 已退役，命令表与 HTTP 都必须拒绝它们。

这不是 TPC-H。清河茶铺走查夹具在 `.data/scenes/.../named-repositories-created/`。底座要用另一张图：任意知识仓在空 home 上，经过哪些状态、每个状态下哪些操作合法、失败必须落到哪个错误码。

补齐时：**判断归属 → 只改仓库根 → `go test` 能红能绿**。不要把数仓表名、Hive GRANT、compose 写进本目录的用例。

---

## 0. 怎么用

根协议套件由 `scripts/testsuite.sh` 分组；文档/结构、插件和真实 Agent 各有显式入口：

```bash
make test           # 默认：lakeFS HTTP 夹具的普通/索引场景 + 真实 OpenSearch
make deploy-local-scenes # 独立真实 lakeFS 部署场景；需已有 local 测试栈
make test-contracts # 显式旧混合套件：component + boundary + 应用/transport 合同
make quality        # gofmt/tidy/vet/staticcheck + 复杂度/文件体积/重复门禁
make test-e2e       # 共享应用语义 + typed Client/HTTP/Catalog 边界
make test-race      # command log / hook / Reader / Index / CLI 并发路径
make test-cover     # 文档/surface + short suite + statement coverage 最低阈值
make validation-inventory # 只生成声明库存 JSON，不跑测试或启动外部服务
make check-validation # 只验证库存/结果工具本身，不启动外部服务
make test-plugin    # DSH MountController、固定 pin、Skill 与 build/package
make test-agent-e2e # 真实模型六角色，严格检查 Skill/shell trace、状态 oracle 与权限边界
make test-agent-metric-e2e # KC-AGENT-01：`.data/scenes/` 状态目录里的 Agent as 任务；需要 OpenSearch
make test-agent-ux-e2e # 真实模型自然语言问答，检查概念/入口/恢复语义与 Skill-only trace
make test-service-e2e # Gitea + OpenSearch 下的 provider/consumer 双身份验收
make test-taihu-live # 真实 Taihu introspection；需 KC_LIVE_TAIHU=1 与资源方密钥 / Bearer
make test-adapters  # Gitea + OpenSearch
make test-docker    # adapters + State runtime + 双角色 service E2E + Linux/FUSE
make test-all       # lakeFS 夹具及真实部署场景 + contracts + docker + plugin；不含付费Agent/Taihu/规模资格
```

`make test`、`make test-lakefs` 和 `scripts/testsuite.sh` 的无参数 / `lakefs` / `local` 入口
统一运行 `TestProductScenes` 与 `TestMetricPermissionScenes`。场景执行器自动准备进程内
lakeFS HTTP 假服务，经真实 adapter 访问；索引使用真实 OpenSearch。设置
`KC_TEST_OPENSEARCH_URL` 时复用该服务，否则启动并清理一次性 OpenSearch。
不要求 `deploy-local` 配置。两组场景不执行独立 Go evidence、动态 State 和走查叶，
不代替完整组件、架构与逐命令合同。

`make deploy-local-scenes` 独立运行 `TestLiveLakeFSSceneDFS`，也保留在显式 `make test-all`
中。只有这条真实部署路径需要先显式 `make deploy-local-up` 准备 lakeFS、PostgreSQL、
MinIO、OpenSearch 等陪伴；缺环境直接失败，不自动重建或清盘。开发栈不接受场景测试。

原默认混合套件保留为 `make test-contracts` / `contracts`。它与显式 `make test-e2e`、race、
coverage 仍包含旧本地夹具；不把它们称为 lakeFS 部署验收。`contracts`、
`make test-e2e`、race 与 coverage 会启动一次性 OpenSearch；项目不保留第二套检索语义。
`testing.Short()` 只隔离专门的 Gitea live adapter 用例；显式
adapter/Docker 组不得把环境缺失静默算作通过。命令覆盖由 CLI 测试进程实际记录调用结果，并在
`KC_ASSERT_E2E_COVERAGE=1` 时与唯一 `cliSurface` 命令表对账；每个公开命令必须至少被调用一次、
至少有一个通过 `body` 断言的成功场景。只读命令至少验证一个有意义的协议边界；按语义 action
识别的状态变更命令至少验证两个独立失败场景（例如形状/目标状态/授权），不用无价值的
unknown-flag 复制用例刷数。风险分级直接读取 `cliSurface` 的 action，新增别名或命令不能靠另一份
手工名单绕过门禁。无参数只读命令验证未授权枚举或身份形状。Help 只是展示文本，不作为覆盖分母。
这些逐命令测试使用 test-only embedded seam 验证 Server 与 transport 共用的应用服务，不将该 seam 暴露为产品调用方式。生产 `Run`、remote CLI 和 HTTP 测试另行证明：部署管理、Server 启动和客户端配方预处理以外的业务命令，无 Server 必须失败关闭。所有领域命令进入逐命令门禁；
四个 help 主题及未知主题恢复动作单独验证，serve 的 help/flag 边界由 Go 测试验证，真实监听、
ready 与优雅退出由 service/kcfs 进程级旅程验证。
需要逐命令审计时可运行
`KC_COMMAND_COVERAGE_REPORT=/tmp/kc-command-coverage.json make test-e2e`；报告区分调用数、成功运行数、
已断言成功场景、已断言失败场景、语义 action、风险要求及错误码；成功必须经过 `body` 的状态/JSON
验证，失败必须经过 `expectCode` / `expectMsg`，未断言调用不会伪装成验证证据。

Agent 验收也有显式分母：`dsh-plugin/scripts/agent-scenarios.json` 固定六个核心操作角色
（provider、governor、consumer、auditor、recovery、unauthorized）、四个首次使用/概念问答，
并登记 `KC-AGENT-01`（`.data/scenes/` 各状态目录里以 `Agent as` 开头的任务块）。两个核心 runner 启动前会把实现与清单逐项
对账；全量门禁必须生成每个场景的回答、trace、oracle 和汇总，过滤器只用于单场景调试。
核心角色通过宿主 shell 调公开 `kc` CLI 并使用人工注入的固定任务上下文，因此可在 macOS 运行；
真实 MountController、只读挂载和 FUSE 生命周期仍由 Linux Docker `make test-kcfs-e2e` 独立验收，
任何平台/能力缺失都不能在 Agent runner 中以成功码伪装为 PASS。

HTTP 使用独立分母：测试从生产 route registry 提取正式路由，不读取 CLI 命令表；当前数量查生成库存。
每条路由必须通过已声明 method 可达、未声明 method 返回 405，并有 transport 责任方：
remote CLI typed-dispatch 的真实请求体回放到生产 handler 的严格 DTO 解码边界；HTTP/宿主
专属入口由直接 handler 成功旅程拥有。同一 route 可由多个 CLI 别名覆盖，但不得再与 HTTP-only
责任重叠；请求用例数不是去重后的路由数。领域成功/失败语义只在应用层
旅程验证一次，认证、固定 pin、Canonical 回读等高风险组合另做真实 HTTP E2E，不为每个 transport
机械复制整套领域用例。新增、删除或重复认领路由都会使门禁失败。`kcfs` 不伪装成领域 HTTP surface：help/plan/mount 及
daemon-mount/stop 控制器边界由 Go + Docker Linux/FUSE 验收。DSH 的 `/api/loom/vfs` 则单独覆盖
GET 列表/分页/预览、POST 偏好写入、只读与路径/游标/方法边界。

`make quality` 是防回退门禁，不把单一指标误当成设计结论：生产函数圈复杂度上限 50、Go 文件
上限 700 行、大段复制上限 150 token；`internal/testkit` 和测试文件不参与结构阈值，但仍参与
编译、`vet` 与 `staticcheck`。超过 30 圈或 500 行应在改动时人工复核职责是否需要拆分；覆盖率
继续由 `make test-cover` 的 55% statement gate 管理。阈值可通过对应 `KC_MAX_*` 环境变量收紧。

测试保留规则：

- 每条测试必须独占一种失败风险；同一属性只在拥有语义的最低层验证一次，除非 transport/provider 转换本身就是合同。
- E2E 只保留跨组件公开旅程、认证边界和固定 basis 等无法由组件测试证明的行为；参数解析、退役命令和纯形状错误用表驱动单测。
- 已退役能力的正路径测试直接删除，不用永久 `t.Skip` 伪装成证据；冻结能力只保留“公开入口明确拒绝”的反例。
- 新测试若不能说明“删掉后哪种真实回归会漏掉”，不进入套件。

声明与覆盖必须成对：

- 端口、守卫与 conformance 集合必须有生产实现或调用方，并有一个能让它失效的反例入口。只有接口定义而无实现、无调用方按端口编程，或只有文字而无反例的守卫，不得作为「已抽象 / 已保护」的证据。
- 同一合同套件新增 provider 时必须登记覆盖矩阵行。「合同存在但该 provider 从未运行」是缺陷，不得默认为通过。
- 删除或降级契约、conformance、架构守卫断言由人类 owner 决定；Agent 只产出候选清单、理由与风险，不自行清理。

复核是周期动作，不是一次性结论：

- 按现有入口复核：`make validation-inventory` 给出声明库存；Provider 合同调用点给出覆盖矩阵；`internal/arch` 的覆盖目录给出守卫边界；三者都不启动外部服务。
- 复核结论必须回到拥有该风险的文档或 `TASK.md` 条目；历史结论不作为本次证明，报告本身按 run 保存。

每条用例四列：

| 列 | 含义 |
|---|---|
| 前置 | 系统已进入的状态（见 §1）。不是「随便有个 home」 |
| 操作 | 所验证的协议动作摘要；精确输入见用例和公开合同，一次只动一个 Surface |
| 预期 | 后态（哪一列变了）或错误码。只读操作写「状态不变」 |
| 现况 | `已定位` 表示已登记断言入口，**不表示本次运行通过**；`partial` 未钉完整风险；`gap` 缺断言；`frozen` 无公开正路径或已退役，只验证拒绝。日期、命令、环境与通过状态仅从同次 run 读取 |

旅程场景：`.data/scenes/` 按可复用状态树嵌套，组织、执行和断言规范见 [`.data/scenes/README.md`](../../.data/scenes/README.md)。构建树提供可重建 fixture；具名 bundle 另声明 `entry_state`，允许消费、维护和重部署任务从既有状态开始。树脊为部署 fixture → System Schema → `repository-attached`（只读验证既有配置源并原子登记）→ 草稿 → Schema → 实例。单独 `repository-registered` 层已合并。尚未登记但已配置源的维护写合同仍由 System 节点上的 probe 验证。新部署不隐式初始化业务 Snapshot。并列 `managed-repository-created` 节点由正式配置 Go Oracle 验证：已有普通主体显式创建平台仓，不预建目标仓、不追加静态绑定，随后发布与重部署续用；仅作为独立 evidence，不建立状态目录。

覆盖分别统计公开命令触达、状态/失败边界合同和完整用户任务；三者不能互相代替。命令是否齐看 `cliSurface` 对上场景树证据（`TestSceneFeaturesCoverPublicCLI`）；节点 `_meta.yaml` 的 `fixture` 声明共享前态，probe 与具名 Go evidence 分别承担场景执行与独立 Oracle。目录中未出现某场景，不等于全仓没有实现。状态与验证用例分别声明关注点；视图可以从既有状态开始展示，隐藏前置不取消构建依赖。主视图并集校验节点、关系与验证证据，声明完整性不等于运行通过；具体组织和入口仍由场景 README 拥有。`TestProductScenes` 与 `TestMetricPermissionScenes` 按构建树复用父 fixture、遍历节点并记录 `_results/latest.json`；`bundles` 是带前态的时间局部旅程，不是执行分母。关键消费探在同一 feature 内以同一已认证主体走正式 `Run → HTTP`，发现入口、固定 pin、检索和读取；部署替换由正式 Run/config/HTTP 测试验证。细粒度协议探仍可使用 test-only embedded seam，不可用它替代产品 transport 证据。

独立授权动作见[权限体系](permissions.md)的授权面；只有被多个用例实际消费的授权前态保留为 `*-granted` 状态，其余授权在 probe 内完成。检索同时覆盖 Snapshot 声明投影与动态 Binding 派生观察。所有知识材料自包含于场景树，由对应 Writer 步骤进入 Snapshot；不读取数仓目录。数仓实体只在墙外黑盒 integration suite 中维护。易变当前值走 Binding 句柄和墙外拉取。`Agent as` 块给 Agent，确定性 `Then` 才是协议 Oracle；形状错误与单命令边界继续由表驱动测试验证。

产品视图按 owner 文档的稳定用例条目生成；文档 ID 与路径由 `docs/graph/` 解析，具体关联在场景节点的 probe、具名 Go evidence 或构建断言上就近声明。产品条目没有验证入口时必须列出具体 gap；局部证据与剩余 gap 可以并存。关联存在只说明可追溯，不证明整项承诺被完整覆盖，也不代表执行通过。产品视图的引用/缺口检查与工程视图的场景并集检查分别进行，二者都不能替代同次运行的验收结果。字段与操作入口由场景 README 拥有；不在产品页、本文或视图定义里复制用例成员清单。

观察点固定看这七列（推演里的四列 + 三条派生）：

```text
成员库 main / 候选 Ref     ⓪ Snapshot
Aspect declaration/version ② ValueSource / DeclarationDigest
Catalog 登记表             ① DumpState + 登记表 git
命令内 pin                 ① ResolveKnowledgeSet，不落盘
Canonical 正文             ② READ / GET_PROVENANCE
ControlState               提案 / Preview / Validation（stateDir/control.json）
工作投影                   ③ basis / lag；可丢
```

`--as` / hook / gate 是 facade，不进七列正文；失败时主状态必须不变。

### 0.1 场景 feature：先观测，再钉后态

写法、目录约定、执行入口和场景合同见 [`.data/scenes/README.md`](../../.data/scenes/README.md)。本目录只保留覆盖格子；不要在这里复制第二套 Gherkin 规范。

Given/When/Then 是可证伪观察。`Then the command succeeds` 不是后态。construct 进入状态后必须用公开 `kc` 钉字段；probe 独占另一种风险，并从该状态取得隔离环境，临时后态不被其它 probe 或子状态继承。删掉只有 succeeds、没有字段的步骤，`TestSceneFeaturesPinObservedState` 必须变红。

### 0.2 方法 → 用例 → 库存 → 运行结果

设计 owner 先决定应然约束；`architecture-invariants.md` 将其映射为禁止观察与验证入口；
本目录把风险展开为前态、动作、后态/不变状态，再选择拥有该语义的最低层。包 README、
公开类型与 Conformance 给出已选定合同；测试不得反向缩小设计。`mvp-acceptance.md` 只综合
用户任务和仍有的缺口；`scale-benchmark.md` 单独拥有容量负载与资格门槛。

设计书也应保留方向性用例：谁在什么前提下完成什么任务、应观察到什么、哪些结果不可接受。
这些用例说明设计为何存在，不复制命令、协议字段、测试代码或当前通过率。用例 ID 在各 owner
内稳定；具体断言经场景 `verifies` 关联，尚未实现的部分明确记为缺口，不能为了挂链接造空场景。
例如动态物化 U1–U9 描述 State 消费与后续 Stream 方向，产品视图同时展示已有局部证据和剩余
缺口。一次局部测试的存在，不能升级为整条方向性任务已实现或已验收。

补充用例时依次完成：

1. 写明设计 owner、已有不变量/旅程 ID，以及删掉该用例会漏掉的独立风险。先在现有层寻找断言，避免跨 transport 重复整套领域语义。
2. 确定前态、显式主体与权限、操作、成功后态、失败后必须不变的状态。并发/重试/删除/旧 pin/撤权等交叉只在存在真实交互风险时展开。
3. 最低层合同验证语义；provider Conformance 验证替换等价；transport 验证坐标/认证/DTO；正式角色旅程验证同一主体完成任务。新增 scene 的写法只遵循 `.data/scenes/README.md`。
4. 先得到会失败的反例，再实现并定向跑绿；按变更范围执行正式分组。需要外部服务或真实模型的项保持独立选择，不把 skip 或未执行折算为通过。
5. 更新拥有该条风险的用例入口或现有场景 manifest，生成库存，再保留同次运行证据。不要新增第二份打勾表或抄录自动计数。

```bash
make validation-inventory                 # stdout 是 JSON；静态声明，不运行用例
python3 scripts/validation.py run --scope docs-and-validation -- make check-surface check-validation
python3 scripts/validation.py run --scope selected-contract -- go test -json -count=1 -run '<选择式>' ./package
# 正式分组自带运行记录；需要相应依赖时才执行
make test
```

生成库存读取 Go AST、生产 `cliSurface`、生产 HTTP 注册、场景目录（`.data/scenes/`）和
Agent manifest；给出文件/行号、独立分母及文档具名 Test 的解析结果。Go 声明跨平台包含，
动态子测试只在执行事件中出现；声明数量不是用例通过率。未解析的具名 Test 引用单独计数，
`make check-validation` 对此失败；生命周期 TestMain 和历史短编号不当作具名测试解析。场景状态、bundles、commands、
HTTP routes 和 Agent 任务互不相加；bundle 是任务导航，不是 DFS 执行分母。

每次显式运行保存在被忽略的 `.validation/runs/<run-id>/`：`manifest.json` 绑定开始时间、
命令、scope、Git revision/dirty、已跟踪与未忽略源文件的内容指纹，以及工具/系统和白名单选择项；
`inventory.json` 是这次声明库存，`output.log` 保留原输出，`go-test.jsonl` 保留收到的 Go 原始事件，
`report.json` 记录结束时间、退出码、package/test 的 pass/fail/skip、重复执行编号和产物摘要。
`executionStatus` 只表示命令退出；`status` 区分含跳过、源码变化与中断，`verification` 给出
事件计数和未观测的声明库存，任何一个字段都不是全产品完成率。
`exitCode` 保留子命令原始退出码；`runnerExitCode` 是验证入口的实际退出码。即使子命令成功，
检测到失败事件或执行期间源码变化仍返回非零，避免 CI 假绿；跳过与未选项继续显式区分。
coverage 组保存 profile，CLI 进程另写逐命令断言报告；scene 按 run/test/node 保存结果。

判读规则：

- 只有同一次、对应代码指纹、对应执行范围的成功事件才能支持“通过”。`sourceChanged` 为真时，这次运行不能证明单一源码状态。
- 被跟踪文档不得把不存在或未入库的证据当作通过依据：`.validation/` 下的产物按 run 保存且不入库，不能单独承担「已验证」的结论；引用审查报告时，被引用的结论必须能在被跟踪文档里找到。
- `skip` 是已执行后跳过；库存中没有终态事件的是未观测，可能未选中、平台排除或提前中断，不能算 pass，也不自动叫 skip。
- 非 Go 命令和没有 JSON 事件的子进程仅有 `command-exit-only` 结果；需继续检查它们自己的 oracle/summary 和原输出，不能假称逐用例覆盖。
- 仅有 manifest、缺最终 report 是未完成；失败也保留原输出和已产生的结果。不合并不同 run 的节点 latest 文件，不把历史 `/tmp` 记录当作本次证明。
- 55% 是当前 statement 固定下限，不能称为与上一版比较后的“不回退”。未保存实际依赖版本/配置/硬件时，普通测试记录不能替代规模资格 manifest。

CI 的验证 job 执行文档图、surface 与结果工具检查；其它 job 在各自 scope 下运行并始终归档。
本页的入口链接表示可验证库存；具体发布/验收结论必须引用 run-id、scope 与必要原始产物。

---

## 1. 状态不是一条线

TPC-H 故事是线性的（空库 → 13 表 → 口径 merge）。底座是**正交维的乘积**。不要为每个组合各写一条；先覆盖维内跃迁，再覆盖下面标明的交叉格。

### 1.1 正交维

| 维 | 取值 | 谁改 |
|---|---|---|
| 部署 | 未初始化 / 既有耐久状态 / 缓存丢失 / 恢复或失败关闭 | 显式 deployment init；serve 只恢复 |
| Catalog 生命周期 | 空登记表 → 已接入 Repository → 有 Workspace → Workspace 退役 → Catalog 归档 | `catalog repo attach` / `dataset define` / `dataset retire` / `catalog archive` |
| 仓生命周期 | 外部既有 authority 只读接入，或显式平台供给并准入 → 新 commit → Catalog 成员归档 | `catalog repo attach` / `catalog repo create` / `COMMIT` / `catalog repo archive` |
| Snapshot Ref | `main=root` → `main=U*` → 存在 candidate → merge 后 `main=C*` | `writer put`/`writer commit` / `governance proposal create` / `governance proposal merge` |
| Binding | Snapshot value / Schema origin Bound State / DescriptorRef / 非法实例 value_source | Bound State 不 PUT 空 Aspect；Catalog 不感知 |
| Workspace 配方 | 无 / 单 source / 多 source / 同 `object_id` 多仓 | `dataset define`（提高 revision） |
| 命令 pin | 无 Serving / 本次冻结 / 下次命令重解 | `ResolveKnowledgeSet`；命令内不得跟 `latest` |
| 维护闭环 | 无 / 已 propose / 已 preview / PASSED\|FAILED / 已 merge | ControlPlane |
| 索引 | 空 / 跟 HEAD / lag / schema 触发 rebuild | `AfterSnapshot` Desire（可丢）+ serve HEAD 对账 |
| 授权 | 主人 / `--as` 命中 / `--as` 拒绝 | `kc admin grant add`（不改七列） |

### 1.2 典型正路径（补齐时按这条走通一遍）

```text
W0 持久配置与外部 authority 已准备
 → deployment init              W1 Catalog 与耐久服务状态初始化，System 信任根可见
 → catalog repo attach          W2 既有仓已接入，published HEAD 不变
 → put / commit                 W3 Canonical 在 main=U1；Catalog 不变
 → dataset define                  W4 配方已登记；read --dataset 立刻可读
 → propose                      W5 candidate=C1，main 仍 U1；read --dataset 仍旧值
 → preview + validate PASSED    W6 ControlState 有 Preview；登记表仍无 pin
 → merge                        W7 main=C1；下次 read --dataset 见新值
 → 再注册 Repository + dataset define rev  W8 同 object_id 两条 FederatedValue，不覆盖
 → retire / archive             W9 Workspace 不可 Open；仓禁写；Catalog 禁 define；未归档仓仍可写
```

并行、不插入这条线：`binding show`（只解析声明）、`search`/`operations access-spec describe`（命中回读这次 pin）、`--as`（拒绝则七列不动）。

### 1.3 和现有套件怎么对齐

| 现有 | 覆盖的是哪一段 | 不是什么 |
|---|---|---|
| `cli/write_flow_test.go` | W0→W3 的 CLI 动词 | 不含 Workspace / merge |
| `cli/read_flow_test.go` | W3 上维护读 | 不含 `--dataset` |
| `cli/consume_flow_test.go` | W4 消费口 | 不含提案 |
| `cli/mvp_acceptance_test.go` | 接入方/消费方最短 MVP 旅程 | 从空 Home 验证仓内发布、Catalog 发现、pin、SEARCH/READ/PROVENANCE |
| `cli/user_journey_test.go` | W1–W9 通用用户旅程 | 从空 Home 跨层验证，不绑定业务域 |
| T1–T12 | 不变量，不是状态机步骤 | 不代替「从 W5 merge」 |
| TPC-H graph canvas | 数仓域 S0–S8 | **不要**当底座覆盖率 |

---

## 2. 用例目录

状态栏写的是前置，操作栏是风险的语义摘要，不是另一份可复制命令合同。精确 argv、API 输入
与断言以定位到的用例及公开包合同为准；不能用 embedded 测试入口冒充正式 Client。

本表是风险与断言入口索引，不是某个历史 `go test` 的结果快照。补齐断言后才可把 `gap`/`partial`
改成 `已定位` 并填可定位的测试名；是否执行通过，按 §0.2 的同次运行证据判断。

### 2.1 W 工作区 / Store

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| W-01 | W0 | `deployment init --config` | 持久配置选定远端 Catalog；初始化独立服务状态，不创建业务 Snapshot | 已定位 | `TestDeploymentSurvivesInstanceReplacement` |
| W-02 | W1 | 再显式 init 同一配置 | 不覆盖既有登记与授权；不静默初始化缺失状态 | 已定位 | `TestDeploymentSurvivesInstanceReplacement` / `TestDeploymentMissingDurableStateFailsClosed` |
| W-03 | W1 | `serve --config` 指向未初始化部署 | 失败关闭，不自动建空 Catalog/授权 | 已定位 | `TestDeploymentDoesNotInitializeOnOpen` |
| W-04 | W1 | 配置新增 Catalog，显式 init 后替换缓存 | 两间独立 Catalog Snapshot、各自 Workspace 与 grant 范围恢复；新增配置不能由 serve 隐式初始化 | 已定位 | `TestDeploymentAddsCatalogExplicitlyAndRecoversIsolation` / `TestCatalogIsolationDoesNotShareAllow` |
| W-05 | W1 | `catalog repo attach` 指向 Catalog ID | 拒绝：登记表不是成员仓 | 已定位 | `TestCatalogRepoWriteErrors` |
| W-06 | W1 | 配置未知 driver 或明文秘密 | 读取配置失败，不修改 authority | 已定位 | `TestStoreConfigRejectsSecrets` / `TestDeploymentRejectsInstanceBoundAuthorities` |
| W-07 | W1 | deployment status / catalog show / catalog audit | 分别为部署恢复状态、组合库存、Catalog 权威历史 | 已定位 | `TestDeploymentSurvivesInstanceReplacement` / `TestCatalogAuditIsGitLog` / `TestSnapshotRegistryPersistsMembershipWithoutKnowledgeSemantics` |
| W-08 | W0 | 业务命令无 Server | 失败关闭，不回退为直开工作目录 | 已定位 | `TestProductCommandsRequireServer` |
| W-09 | W1 | 旧 local / register 命令 | 入口拒绝；不保留隐藏兼容别名 | 已定位 | `TestDeploymentContractRetiresLocalAndManualRegistration` |
| W-10 | W1 | 更换实例并删除 cache | 配置、Catalog Snapshot、知识 Snapshot、授权/gate、Receipt 与治理状态保留；缓存可重建 | 已定位 | `TestDeploymentSurvivesInstanceReplacement` / `TestDeploymentMissingDurableStateFailsClosed` |
| W-11 | W1 | 丢失状态卷、账本或已初始化 Catalog 权威后再次 init | 拒绝重置；恢复不重写既有 Writer ledger，不将损坏策略解释为空 | 已定位 | `TestDeploymentCannotResetLostDurableVolume` / `TestDeploymentDoesNotRecreateLostCatalogBranch` / `TestDeploymentRecoveryDoesNotRewriteControlLedger` / `TestDeploymentMalformedPolicyFailsClosed` / `TestManagedRepositoryReadyReplayRejectsLostCatalogAuthority` |
| W-12 | W1 | Catalog Snapshot 权威或配置的检索后端不可用 | readiness 失败关闭；合法 driver 拼写与装配使用同一规范化规则；拒绝本机 Git remote 字段 | 已定位 | `TestDeploymentReadinessRequiresCatalogAuthority` / `TestDeploymentReadinessUsesNormalizedIndexDriver` / `TestDeploymentRejectsLegacyCatalogGitRemoteField` |

### 2.2 C 组合平面（①）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| C-00 | 已部署、普通主体仅获 Catalog 创建准入 | create → put 与 commit --dir → 固定 commit READ/PROVENANCE → 替换实例 → 幂等重放和 CAS 维护 | 新仓无静态绑定；旁观者拒绝；撤权后重启与 create 重放不补权 | 已定位 | `TestManagedRepositoryProviderCreatesPublishesAndResumes` / `TestManagedRepositoryProviderOnLiveGitea` |
| C-01 | W1 | `catalog repo attach` 未配置/不存在的仓 | 拒绝；不建 Snapshot、不改 Catalog 成员 | 已定位 | deployment / write errors |
| C-02 | W2 | `dataset define` 未挂载 source | `KNOWLEDGE_SET_INVALID`；登记表无该 Workspace | 已定位 | T11 / S0 |
| C-03 | W2 | `dataset define` 重复来源缺显式路径、使用不同版本或子树重叠 | 拒绝歧义；同仓同版本的互不重叠子树可分别映射 | 已定位 | T11 / `TestRepeatedRepositoryMustShareCoordinateAndDisjointSubPaths` / `TestOneRepositoryCanProjectSeveralDisjointSubtrees` |
| C-04 | W3 | `dataset define` 合法 | 进入 W4；立刻 `OpenKnowledgeSet` / `read --dataset` | 已定位 | S2 / T11 |
| C-05 | W4 | `resolve --dataset`（无 `--object`） | pin 只有 `{仓→commit}`；不读正文、无动态 cut；带 `--object` 返回 `USAGE_INVALID` | 已定位 | `TestConsumeViewFollowsPublishedBranch` `TestCommandSpecificUsageBoundaries` `TestKnowledgeResolveAndObjectLogOverHTTP` |
| C-06 | W4 | `writer put` 再 COMMIT | **Catalog 不变**；main 前进；已打开的 pin 仍钉旧 commit | 已定位 | S1 / `TestOpenedKnowledgeSetPinDoesNotMoveWithLaterCommit` |
| C-07 | W4 | `dataset retire` | `OpenKnowledgeSet` → `KNOWLEDGE_SET_INVALID`；其它 Workspace 仍可用 | 已定位 | S6 / `TestWorkspaceAndCatalogLifecycle` |
| C-08 | W4 | `catalog repo archive` | 该仓 `COMMIT`/`PROPOSE` → `REPOSITORY_ARCHIVED`；新 OpenKnowledgeSet 不选入 | 已定位 | lifecycle / write errors / S6 |
| C-09 | W4 | `catalog archive` | `dataset define` → `CATALOG_ARCHIVED`；未归档成员仓仍可写 | 已定位 | S6 |
| C-10 | W8 | `dataset define` 提高 revision、改 sources | **下次** OpenKnowledgeSet 用新配方；本次 pin 不变 | 已定位 | S4 |
| C-11 | W4 | `CheckResolved` / `governance preview validate --preview` | 只检查 Snapshot 成员与 commit，不解析 Binding | 已定位 | catalog/control tests |
| C-12 | W4 | `dataset define --as steward --request-id …` | 登记表 git stamp 含 as / request-id / ruleId | 已定位 | `TestCatalogGitStampsPrincipal` / F-03 HTTP evidence |
| C-13 | W4 | `audit --dataset` vs `log --dataset --object` | audit=配方历史；log=对象引入 commit | 已定位 | consume_flow |
| C-14 | W3 | 无 Workspace 时 `read --dataset` | `KNOWLEDGE_SET_INVALID` | 已定位 | S0 / consume_flow |
| C-15 | W4 | 重复 attach / retire / archive | 当前权威不新增 commit；远端已变化则拒绝，不能依据过期内存报成功；只读视图拒写 | 已定位 | `TestRemoteCatalogLifecycleNoOpChecksAuthority` |
| C-16 | W4 | 远端拒绝提交或两个旧视图并发提交 | 已接受 HEAD、内存状态与远端保持一致，失败候选不泄漏；并发由 Git CAS 拒绝覆盖 | 已定位 | `TestRemoteRegistryRejectedPushLeavesStateAndCacheHeadUnchanged` / `TestRemoteRegistryConcurrentWritersUseAuthorityCAS` / `TestCatalogInstancesSharingRegistryDoNotOverwriteEachOther` |
| C-17 | 两仓已有普通文件，消费者仅获 Dataset 读权 | 经 Server 发布三个来源片段，按新目录枚举并读取同名文件 | 三个目标路径各自返回准确字节、原仓与固定版本；未选中内容不出现在列表，原路径与开头的向上路径读取拒绝 | 已定位 | `TestDatasetDirectoryDeliveryReorganizesMultipleRepositories` |
| C-18 | C-17 已发布并打开旧视图 | 更新一个源仓，随后调整目录并发布新版 | 源 HEAD 前进不改变服务版；新请求采用新路径与字节，旧 pin 继续旧路径与字节，两版路径不串用 | 已定位 | `TestDatasetDirectoryDeliveryRepublishKeepsOldLayoutAndBytes` |
| C-19 | C-17 已发布 | 尝试相同或嵌套目标路径，再以合法布局重试该 revision | 冲突候选拒绝，当前布局、pin 与内容不变；失败不占用 revision，修正后可发布 | 已定位 | `TestDatasetDirectoryDeliveryRejectsConflictsWithoutReplacingRelease` |
| C-20 | 多仓中各有若干文件 | 逐文件挑选、改名并混排到同一目标目录 | 只交付选中文件，枚举、读取与授权范围一致，目标冲突拒绝 | 已定位 | `TestDatasetFileDeliveryServesRenamedAndMixedEntries` / `TestDatasetCloneMaterializesDeliveredTree` / `TestDatasetCloneJourney` |
| C-21 | C-17 已发布，选中子树之外有兄弟文件 | 从交付目录读取或枚举带中部上跳的相对路径 | 在访问来源前拒绝路径逃逸，不返回范围外字节或目录项 | 已定位 | `TestDatasetDirectoryDeliveryRejectsTraversalInsideRelativePaths` |

C-17–C-19、C-21 位于 `cli/dataset_directory_delivery_test.go`，C-20 位于
`cli/dataset_file_delivery_test.go` 与 `cli/dataset_clone_journey_test.go`；都以现有 lakeFS
进程内夹具经真实 adapter 和 typed Client/Server 验证文件交付，不包含操作系统 FUSE 挂载或
真实 lakeFS 部署验收。它们作为具名 Go evidence 挂在场景树的 `dataset-defined` 前态，使用自己的
setup；默认的两组 feature 场景不自动执行这些独立 Go 用例。C-21 的路径校验依据是
`cli/datasetfs.go` 的 `datasetFSRepositoryPath` / `validKnowledgeSetFSRelative`：拼接清理后的
结果必须仍落在所选子树内，路径中部的父目录片段不能把请求带出来源子树；回归用例同时要求
文件读取和目录枚举拒绝。执行结果按 run 保存，不在本文维护「全绿」结论。

### 2.3 K 快照写（COMMIT / PUT / REMOVE）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| K-01 | W2 | `writer put` 一条 Address | `main=U1`；Receipt `APPLIED`；Catalog 不变 | 已定位 | write_flow / T6 |
| K-02 | W3 | 同 `command_id` + 同 digest | `REPLAYED`；HEAD 不变（K-18） | 已定位 | T4 / S1 |
| K-03 | W3 | 同 `command_id` + 异内容 | `IDEMPOTENCY_CONFLICT`；HEAD 不变 | 已定位 | T4 / write errors |
| K-04 | W3 | 过期 `expectedTargetCommit` | `NON_FAST_FORWARD`（K-06） | 已定位 | T2 / S5 |
| K-05 | W3 | 空 ChangeSet | `USAGE_INVALID` | 已定位 | T3 / write errors |
| K-06 | W3 | `originKind=DERIVATION` 无 VRV+algorithm | `PRECONDITION_FAILED`；Catalog 不变 | 已定位 | S1 / write errors |
| K-07 | W3 | `schema_ref` 指向不存在的 `schema/*` | `SCHEMA_REVISION_UNRESOLVED` | 已定位 | `knowledge/writer/schema_test.go` / S1 |
| K-08 | W2 | 同一 Changeset 先 PUT schema 再引用 | 接受 | 已定位 | schema_test |
| K-09 | W3 | `schema_ref` 指向外仓 | 拒绝 `SCHEMA_REVISION_UNRESOLVED` | 已定位 | schema_test |
| K-09a | W3 | PUT Domain Schema | Meta Schema type/access/shape 校验；失败不移动 HEAD | 已定位 | `TestSchemaDefinitionMustConformToSystemMetaSchema` |
| K-09b | W3 | PUT 引用实例 | 同批/既有 Schema 校验 Address、required、type、additionalProperties | 已定位 | `TestSchemaInstanceValidationUsesSameChangesetDraft` / `TestSchemaInstanceValidationRejectsMissingAndUnknownFields` |
| K-09c | W3 | 更新既有 Domain Schema | breaking 复用返回 `SCHEMA_INCOMPATIBLE` 且 HEAD 不动；新增非必填字段成功 | 已定位 | `TestSchemaEvolutionRejectsBreakingReuseAndAllowsOptionalAddition` |
| K-09d | W1 | 选择 Workspace 前分页发现 Schema | System Repository 可直接读取固定 commit 的两页 Schema，响应带 coverage/continuation | 已定位 | `TestSystemSchemaDiscoveryIsBoundedAndWorkspaceIndependent` |
| K-09g | W1 | 把内置 System Schema 导入 Snapshot | 空 LakeFS/Gitea 写入与二进制 digest 一致的 `schema/*`；已占用且失配返回 `PRECONDITION_FAILED`；System 只能由 `deployment system publish --config` 显式发布，普通 attach 仍拒绝 | 已定位 | `TestPublishSystemSeedsEmptyTreeAndRefusesOverwrite` / `TestLocalSystemPublishSeedsLakeFSAuthority` / `TestLocalSystemPublishImportsBuiltinSchemasIntoLiveGitea` |
| K-09h | W3 | Canonical `_schemas/` 与类型目录 | `schema/*` 默认平铺在唯一的 `_schemas/`；实例按 Schema 实体类型分目录（`metrics/`、`tables/`），不用 `objects/`；System 跟踪源与发布树一致 | 已定位 | `TestDefaultPathPlacesSchemasUnderSchemasDirectory` / `TestDefaultPathPlacesInstancesUnderTypeDirectories` / `TestSchemaExamplesIngestAndDescribe` |
| K-09e | W3 已有带 `schema_ref` 的单元 | 再 PUT 同一 Address 但省略 `--schema-ref` | 继承既有声明并校验；违约返回 `SCHEMA_INSTANCE_INVALID` 且 HEAD 不动 | 已定位 | `TestSchemaValidationCoversInheritedSchemaRef` / `TestSchemaAddressMatchingAppliesWithoutExplicitMetaSchema` |
| K-09f | W3 多实例引用同一 Schema | 更新该 Schema / REMOVE 该 Schema | 反向依赖有界索引校验受影响实例，失配 `SCHEMA_INSTANCE_INVALID`；仍有引用者时 REMOVE 返回 `SCHEMA_INCOMPATIBLE`；同批迁移或同批删除可通过 | 已定位 | `TestSchemaUpdateValidatesAlreadyPublishedInstances` / `TestSchemaRemovalRequiresNoRemainingReferrers`；有界反向索引由 writer `referrers` 契约承接（native 索引专用守卫用例随 Dolt adapter 退役删除，重新引入 native provider 时恢复） |
| K-10 | W3 对象已在 | `--if-absent` | `PRECONDITION_FAILED`；HEAD 不变 | 已定位 | S5 / write errors |
| K-11 | W3 | 再 PUT 同 `object_id`、换 `path-hint` | 身份不变；旧 commit 仍旧路径（T1 / K-04） | 已定位 | T1 / S5 |
| K-12 | W3 | 先后 PUT 两个 Aspect | 拼装对象两分区独立；`readAddress` 单单元 | 已定位 | T12 provider conformance |
| K-13 | W3 Entity blob 已在 | 再 PUT 同 id 的 Aspect | `OBJECT_ID_CONFLICT`；HEAD 不变 | 已定位 | writer conformance |
| K-14 | W2 Tree fixture | 两文件同一 Address | 通用 Tree interpreter 拒绝重复 Address | 已定位 | provider-independent reader tests |
| K-15 | W2 | `kc writer commit --dir` | 对照当前版本求差后提交；frontmatter `object_id` 胜路径；目录扫描与校验在 Writer `Ingest`。不写出 ChangeSet | 已定位 | T7 / `TestWritePath` / `TestT7Ingest` |
| K-16 | 目录写入 | `kc writer commit --dir` / `kc diff` | 与 K-01 同：只推进成员 Ref。对照当前版本求差是 commit 内部动作；`kc diff` 用同一对照且不写仓 | 已定位 | write_flow / `TestDiffDirectoryAgainstCurrentVersion` |
| K-17 | W3 | `writer remove` | 对象在新 commit 上 UNRESOLVED；旧 commit 仍可读 | 已定位 | T12 / read_flow |
| K-18 | W2 | `writer put --repo` = Catalog id | `TARGET_REPOSITORY_DENIED` | 已定位 | S0 |
| K-19 | W9 仓已归档 | `writer put` / `governance proposal create` | `REPOSITORY_ARCHIVED` | 已定位 | write errors / S6 |
| K-20 | W3 多 op | 任一 op 失败 | 无部分提交（T3） | 已定位 | T3 |
| K-21 | W3 | `writer.Reconcile` 预览 | 只出 ChangeSet；确认后走 `writer commit` | 已定位 API / **frozen CLI** | `TestT7Reconcile`；Help 明示 connector kit 在墙外，无 `kc reconcile` |

### 2.4 B Aspect Binding

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| B-01 | W2 | PUT Domain Schema `origin` + 已有实体 | 同一 commit `binding show` / `access`；不另存 null Aspect；不调用 runtime 也可看声明 | 已定位 | `TestResolveBindingFromSchemaWithoutInstanceFile` / CLI Binding E2E |
| B-02 | W2 | PUT Domain Schema origin（State） | record schema 仍由 schema 声明；底座无 APPEND | 已定位 | `TestResolveBindingFromSchemaWithoutInstanceFile` |
| B-03 | W2 | PUT ResourceDescriptor 知识对象 | Descriptor 是 invoke 的实例；`kc access` 不另存句柄 | 已定位 | CLI invoke E2E / `TestApplyKnowledgeCommitRejectsInstanceBinding` |
| B-04 | W3 已有 Bound Schema | PUT 相同实体，只改 Schema origin | 实体 Snapshot digest 不变，declarationDigest 改变，LOG 保留 Schema revision | 已定位 | `TestBindingDeclarationChangeIsVersionedWhenValueIsUnchanged` |
| B-05 | W2 | PUT 实例 value_source=binding / 声明不完整 | `USAGE_INVALID`，失败关闭 | 已定位 | `TestIngestRejectsInstanceBinding` / `TestValidateBindingRejectsAmbiguousAndIncompleteDeclarations` |
| B-06 | W2 有手写 frontmatter | `writer commit --dir` / `Ingest` 非法 value_source | Snapshot 扫描失败，不降级成普通 Snapshot value | 已定位 | `TestIngestRejectsMalformedOrInvalidValueSource` |
| B-07 | W8 两仓同一 Address 都声明 Binding | `binding show --dataset` | `ResolvedBinding[]` 两条；上层必须处理歧义 | 已定位 API；DSH 工具拒绝多条 | Binding API tests |
| B-08 | W4 State Binding + 注入 StateLookup | `read --dataset` | 返回绑定后的值，同时携带 declaration/observation 双 basis | 已定位 | `knowledge/serving` + HTTP E2E |
| B-09 | W4 State Binding、无 runtime | `read --dataset` | `CAPABILITY_UNSATISFIED`，不得把 `null` 占位当结果 | 已定位 | CLI E2E |
| B-10 | W4 Stream Binding | 普通 `read --dataset` | `CAPABILITY_UNSATISFIED`，不隐式数组化 Stream | 已定位 | `knowledge/serving` tests |
| B-11 | W4 State Binding | VFS read 同一单元 | 返回固定声明文件且不调用 StateLookup | 已定位 | HTTP VFS/Binding E2E |
| B-12 | 任意 | `append` / `stream` CLI 或 HTTP | unknown command / 404 | 已定位 | `TestAppendAndStreamSurfacesStayAbsent` |
| B-13 | W4 + 独立 `resource-access/v1` runtime | `read --dataset` | Knowledge Server 按 Domain Schema `origin` HTTP 传 pinned Binding、已验证身份、调用方认证头（Taihu `Authorization` / `X-Tai-Identity`）与关联信息；runtime 返回 value+basis | 已定位 | `TestHTTPStateLookupCallsIndependentResourceRuntime` `TestHTTPStateLookupForwardsCallerAuthentication` `TestTypedKnowledgeReadForwardsTaihuCallerAuthentication` + `make test-state-runtime-e2e` Docker runtime + HTTP Binding/VFS E2E |
| B-14 | State Binding 缺 lookup/read 或 runtime 返回 bare result | `read --dataset` | `CAPABILITY_UNSATISFIED`，不猜 operation、不接受无 basis 正文 | 已定位 | `TestHTTPStateLookupRejectsUnsupportedAndDishonestRuntime` |
| B-15 | SEARCH 命中含 State Binding | Snapshot-only query 命中后逻辑 hydrate；State-field query 使用独立动态投影 | 两条路径都返回绑定后的值和 observation basis | 已定位 | `TestWorkspaceSearchHitUsesLogicalStateHydration` / `TestLiveHTTPDynamicStateSearchJourney` |
| B-16 | 无公开 Workspace LIST | 旧 list surface | 明确拒绝；State hydrate 只由 READ/SEARCH hit 使用，维护扫描与文件投影保持声明视图 | 已定位 | `TestFormalServiceNamespacesAreExplicitAndRetiredRoutesStayMissing` |
| B-17 | State refresh 已发布 | VFS/Repository read 同一 Address | HEAD 与占位值不变；observation 不进入 Snapshot | 已定位 | `TestStateRefreshFindsDynamicValueWithoutChangingSnapshot` / Docker journey |
| B-18 | W4 + ResourceDescriptor + 独立 runtime | `invoke --object … --operation … --input …` | Descriptor 在同一 pin 回读；只使用声明中的 runtime/protocol/origin/call；透传固定仓/commit/object 与输入；缺失 origin/operation 或混用 Binding 参数失败关闭 | 已定位 | `TestCatalogViewsChecksAndKnowledgeResolve` / 走查叶 `resource/shop-sql` |

#### 动态消费的方向性用例与待补风险

应然任务由[外部资源访问](resource-access.md)拥有；下表只定位独立验证风险。
场景的 `product/materialization/U1` 至 `U9` 视图从 owner 提取任务，关联具名测试，并保留
尚未证明的范围。`partial` 表示仅定位到部分断言，不表示执行通过。

| ID | 设计用例 | 前态与动作 | 必须证明 | 现况与证据边界 |
|---|---|---|---|---|
| B-19 | U1 | 源变化、丢通知后查询当前状态，包含零命中和分页 | 查询范围的覆盖与时效可判定；过期不冒充当前，分页不混依据 | gap：现有同 basis 命中不能证明零命中时效 | 无 |
| B-20 | U2 | 分别返回业务空值、取值错误与缺 lookup | 空值有成功观察；失败不制造 MISSING，缺能力不返回占位 | partial：未证明旧 revision 在时效策略下的交付 | `TestObservedNullProvesMissingAndFailedRefreshKeepsPublishedRevision`、`TestBoundReadFailsClosedWithoutStateRuntime`、`TestStateRuntimeFailuresAndInvalidBasisFailHonestly` |
| B-21 | U3 | 丢失、重复、乱序通知并替换进程 | 后台独立恢复，断档有解释，不移动 Snapshot | partial：只证明显式 notice 后拉取；不证明丢通知与重启恢复 | `TestProjectionControllerNoticePullsStateWithoutChangingSnapshot` |
| B-22 | U4 | latest-only 源覆盖旧值，替换进程，再删除或到期清理观察记录 | 保留期内重读原观察；丢失与到期可区分，不改读 latest | gap：active Serving State 与 revision 摘要不证明历史保留 | 无 |
| B-23 | U5 | Dataset 保留旧声明，HEAD 改 Schema/origin，随后 runtime 退役 | 旧声明独立维护；不可用时不偷换 HEAD；发布不等于动态就绪 | partial：只证明声明范围与版本检查，不证明旧声明维护 | `TestDatasetStateObservationRequiresPublishedDeclaration` |
| B-24 | U6 | 后台可见、消费方不可见或被撤权；另有显式授权共享的正例 | READ/SEARCH/分页/历史消费遵守对应源授权，共享不会隐式扩大 | partial：Dataset 文件层已具名——撤权后同一 pin 的 mounts/枚举/挂载相对/交付寻址读取全部拒绝，未选中内容不可枚举，SEARCH 按已发布清单过滤，pin 不扩大范围；绑定 State 的消费方不可见与撤权判定仍未证明 | `TestDatasetDeliveryRejectsReadsAfterGrantRevocation`、`TestDatasetDirectoryDeliveryReorganizesMultipleRepositories`、`TestDatasetSearchExcludesPathsOutsidePublishedList`、`TestDatasetRegressionHTTPDatasetPinCannotBroadenScope` / `TestDatasetRegressionSemanticVFSMustRespectDatasetScope` |
| B-25 | U7 | origin 变更、重定向、凭证 audience 不符；用户请求与后台对账交错 | 不向不适用目标发凭证，后台不借用用户/通知身份 | partial：只证明身份传输与缺主体拒绝 | `TestHTTPStateLookupForwardsCallerAuthentication`、`TestHTTPStateLookupRequiresCallerPrincipal` |
| B-26 | U8 | 大量观察中单 key 改变，另一个来源持续失败 | 稳态整体工作量随变更而非总量增长；独立工作继续，受影响查询不虚报完整 | partial：只计 lookup，不证明复制、摘要与索引写入成本或失败隔离 | `TestStateRefreshObjectsOnlyTouchesNamedAddress` |
| B-27 | U9 | 普通 READ 遇到 Stream；后续请求有界事件窗口及断档恢复 | 普通读取不数组化；未来窗口明确顺序、进度与缺口，订阅独立设计 | partial：只证明当前拒绝边界；窗口与订阅仍为后续方向 | `TestOrdinaryReadRejectsStreamBinding` |

### 2.5 R 维护读（`--repo` + `--commit`/`--ref`）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| R-01 | W3 | `resolve --repo --commit` | 对象 `RESOLVED`；带 `--aspect` 时为 Address status；`--member` 无 `--aspect` 为 `USAGE_INVALID`；缺失 Address 为 `UNRESOLVED` | 已定位 | read_flow / `TestCatalogViewsChecksAndKnowledgeResolve` |
| R-02 | W3 | resolve 不存在对象 | `UNRESOLVED`（不是错误信封） | 已定位 | read_flow / `TestKnowledgeResolveAndObjectLogOverHTTP` |
| R-03 | W3 | `read` 未知 sha | `VERSION_UNRESOLVED` | 已定位 | T12 |
| R-04 | W3 | `read` 已知 commit、无对象 | `KNOWLEDGE_REF_UNRESOLVED` | 已定位 | T12 |
| R-05 | W3 后续无关 commit | `log --object` | 只占引入该 digest 的 commit；`After` 对上一页最后一条 exclusive | 已定位 | T12 / S5 / `TestProviderIndependentRepositoryContract` `TestCatalogRepoReadFlow` |
| R-06 | W3 两版本 | `Reader.Diff` | 两 pinned 上的对象值；无公开 CLI | 已定位 | T12 / `TestCatalogRepoReadFlow` |
| R-07 | W3 | `provenance` | 本对象信封链；不是 git log，不爬 `sourceRefs` | 已定位 | provider conformance / S5 / T7 citation |
| R-08 | W3 | 退役的 Knowledge LIST | 公开入口拒绝；维护 scan 不作为消费枚举 | frozen | `TestRemovedCommandsAreRejected` |
| R-09 | 有 `schema/*` | `describe-schema` | AccessHints；非 schema 对象忽略 | 已定位 | `knowledge/reader/schema_test.go` |
| R-10 | 多 Aspect | `read --aspect` / `readAddress` | 单单元 | 已定位 | S5 |
| R-11 | 有 permissions Aspect | READ 使用 `AspectSelector` exclude；SEARCH 只按 schema access hints | Canonical 仍在；Reader 不持第二套投影 | 已定位 | T8 |
| R-12 | W4 | `--dataset` 兼 `--repo`/`--commit`/`--ref` | 拒绝组合 | 已定位 | consume_flow |

### 2.6 V 消费 Serving（只 `--dataset`）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| V-01 | W4 然后又 COMMIT | **新** `read --dataset` | 解到新 HEAD（跟已发布 selector） | 已定位 | consume_flow / T11 / serving |
| V-02 | 已 OpenKnowledgeSet | 命令进行中再 COMMIT | 本次 pin 不动（K-11） | 已定位 | `TestOpenedKnowledgeSetPinDoesNotMoveWithLaterCommit`；Help 明示一条 CLI 命令只 resolve 一次，跨命令跟已发布 Dataset 或抄 `--repo --commit` |
| V-03 | W8 同 object_id 两仓 | `read --dataset --object` | 两条 FederatedValue，不按 scope 覆盖（K-13） | 已定位 | T11 / S4 / WriteCheckout 两文件 |
| V-04 | W4 | `read --dataset` / `resolve --dataset` 不存在对象 | **空数组**，不是错误（维护口才是 `KNOWLEDGE_REF_UNRESOLVED` / `UNRESOLVED`） | 已定位 | consume_flow / `TestCatalogViewsChecksAndKnowledgeResolve` `TestKnowledgeResolveAndObjectLogOverHTTP` |
| V-05 | W1 | 未知 Workspace | `KNOWLEDGE_SET_INVALID` | 已定位 | consume_flow |
| V-06 | W4 | `search --dataset` | 各仓在**这次 pin** 上 SearchAt，不回绕 live | 已定位 | consume_flow / `TestSearchAtDoesNotRewindLive` |
| V-07 | W4 知识仓无显式 mount | Workspace File Gateway `mounts:list` / kcfs | `CAPABILITY_UNSATISFIED`；禁止扫描知识仓伪造工作树 | 已定位 | `TestKnowledgeOnlyWorkspaceCannotCheckoutByScanning` |
| V-11 | W4 | `catalog show` + `workspace resolve` + `access describe` + `projection describe --commit` | CatalogState + pin + AccessPlan + 钉死 basis 的投影；不是新协议对象 | 已定位 | consume_flow |
| V-12 | W4 | `resolve` / `log --object` / `provenance` / `describe-schema --dataset` | RESOLVE 是 status（可打到 Address）；Workspace 缺失为 `[]`、`--repo` 缺失为单条 `UNRESOLVED`；log 是有界页且拒绝 `--aspect`/`--member`；provenance 拒绝 Address 坐标 | 已定位 | consume_flow / S5 / `TestCatalogViewsChecksAndKnowledgeResolve` `TestRemoteProviderReadBackAndConsumerDiscovery` `TestKnowledgeResolveAndObjectLogOverHTTP` |
| V-14 | W4 | `dataset define` | **不发权**；无 `--repo` allow 不能读成员 | 已定位 | `TestKnowledgeSetAuthorizationCoverageIsHonest` |

### 2.7 M 维护闭环（提案）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| M-01 | W4 | `governance proposal create` | candidate=C1；**main 不动**；`read --dataset` 仍旧值（K-07） | 已定位 | T9 / S3 |
| M-02 | W5 | `governance preview create --dataset` | Preview 只写 ControlState；登记表无 pin yaml | 已定位 | T9 / S3 |
| M-03 | W5 | `governance preview validate --preview` | 结构检查（仓已挂、commit 在）；写出 ValidationReport | 已定位 | T9 / hook_gate |
| M-04 | W6 | `governance validation record --suite --outcome` | 只绑定传入 PASSED/FAILED，不跑套件 | 已定位 | T9 |
| M-05 | 结构 FAILED 或 suite FAILED | `governance proposal merge` | `GATE_UNSATISFIED`；main 不动 | 已定位 | T9 / hook_gate |
| M-06 | W6 PASSED | `governance proposal merge` | main 快进到 C1；**下次** `read --dataset` 见新值 | 已定位 | T9 / S3 / CLI |
| M-07 | preview 后 candidate 再提交 | `governance proposal merge` | `CANDIDATE_MOVED` | 已定位 | T9 / S3 |
| M-08 | preview 后别人推走 main | `governance proposal merge` | `NON_FAST_FORWARD` | 已定位 | T9 |
| M-09 | 有 `operations gate add --on merge --require validate,suite:x` | 缺 suite 证据 | `GATE_UNSATISFIED` | 已定位 | T9 / hook_gate |
| M-10 | Preview 成员变了，拿旧 PASSED | `governance proposal merge` | `VALIDATION_BASIS_MISMATCH` | 已定位 | T9 |
| M-12 | W6 | pre-merge hook 成功、gate 仍缺 suite | hook ≠ gate；仍 `GATE_UNSATISFIED` | 已定位 | `TestPreMergeDoesNotSatisfyGate` |
| M-13 | W4 | `writer put` / `read` | 不查 gates.json | 已定位 | `TestReadPathAndPutIgnoreGates` |

### 2.8 I 索引（③）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| I-01 | W3 | COMMIT | `AfterSnapshot` Desire 后 worker 增量；`search` 能命中；索引非权威 | 已定位 | `TestCatalogHookUpdatesIndexAfterCommit` `TestSearchAfterPutIsIncremental` |
| I-02 | W5 | `governance proposal create` | **不**通知 Catalog Hook | 已定位 | `TestProposalDoesNotNotifyCatalog` |
| I-03 | live 已到 U2 | `SearchAt(U1)` | 不把 live rewind 到 U1 | 已定位 | `TestSearchAtDoesNotRewindLive` |
| I-04 | 改 `schema/*` AccessHints | COMMIT | rebuild（不是增量 content） | 已定位 | `TestIndexSchemaChangeForcesRebuild` |
| I-05 | W8 | `operations access-spec describe --dataset` | 每仓一份逻辑 AccessSpec，不按 Workspace 建表 | 已定位 | `TestPlanAccessTwoRepositories` |
| I-06 | permissions 无 text hint | 编索引 | 省略；声明了 access 才进 | 已定位 | index_test |
| I-07 | W4 | `operations projection describe --repo` | basis / lag / compiled hints | 已定位 | `TestDescribeIndexShowsCompiledSpec` / consume |
| I-08 | 未声明 MATCH 车道 | `search --query` | `CAPABILITY_UNSATISFIED` | 已定位 | index / searchop |
| I-09 | Hook 失败 | `dataset define` | 配方仍成功（hook 不回滚 ①） | 已定位 | `TestCatalogHookFailureDoesNotFailDefineKnowledgeSet` |
| I-10 | 两个 schema/aspect 有同名 path | 裸 path SEARCH | `USAGE_INVALID`；完整 FieldRef 可用 | 已定位 | `TestCheckSearchRejectsAmbiguousBarePath` |
| I-11 | Provider 返回 authority 中不存在或 wrong-basis CandidateRef | hydrate | `PRECONDITION_FAILED`；错误坐标时 authority 零调用 | 已定位 | `TestSearchRejectsCandidateMissingFromFixedAuthorityBasis` `TestSearchRejectsWrongCandidateCoordinatesBeforeAuthorityHydrate` |
| I-12 | Workspace 一成员不支持 query | 联邦 SEARCH | 整次 `CAPABILITY_UNSATISFIED` fail closed；不把能力缺口伪装成 partial | 已定位 | `TestWorkspaceSearchFailsClosedWhenAnyMemberCannotSatisfyQuery` |
| I-13 | schema access 含 key/summary/stored/gin/hnsw | DESCRIBE_SCHEMA | `USAGE_INVALID`，不得静默忽略 | 已定位 | `TestDescribeSchemaRejectsLegacyAndPhysicalAccessTokens` |
| I-14 | 同一 Repository basis 被多个 Workspace 引用 | 编译/查询投影 | 复用 `(repository,basis,provider,physicalDigest)`；`CompiledDoc` 不含 Workspace/PinID | 已定位 | `TestCompiledDocumentDoesNotCarryWorkspaceScope` / Workspace search tests |
| I-15 | Binding 占位 null、未 observation | 编译投影 | 不进入 `EligibleFields`，不能误报 MISSING | 已定位 | `TestProjectionCompilerRequiresObservationForBindingEligibility` |
| I-16 | Binding 成功 observation=null | 编译与 MISSING | 字段 eligible、无 cell，MISSING 可命中 | 已定位 | compiler test / `TestObservedNullProvesMissingAndFailedRefreshKeepsPublishedRevision` |
| I-17 | observation 值变化、commit 不变 | `RefreshState` | 有界 streaming warm rebuild；旧值不再命中；HEAD 不变 | 已定位 | `TestStateRefreshFindsDynamicValueWithoutChangingSnapshot` |
| I-18 | runtime refresh 失败 | `RefreshState` | `TEMPORARY_UNAVAILABLE`；已发布 revision 不被空/null 覆盖 | 已定位 | `TestObservedNullProvesMissingAndFailedRefreshKeepsPublishedRevision` |
| I-19 | State text + typed range + Snapshot filter | OpenSearch SEARCH | 同一完整 object 文档隐式 AND，并从同 revision Serving State hydrate | 已定位 | `TestLiveOpenSearchStateProjectionRefreshAndSameBasisHydrate` |
| I-20 | 动态 SEARCH | 返回 SearchView/hit | SearchView 仅含紧凑 `projectionRevisions`；逐 hit `KnowledgeVersion.Observations` 完整 | 已定位 | live OpenSearch + Docker HTTP journey / `TestDynamicProjectionPublicEnvelopesStayCompact` |
| I-21 | 受权 change notice（仓/ref/可选 Address） | Observer 只发定位，不带正文 | 控制器 pull runtime 并发布动态投影；HEAD 与 Snapshot Desire 不变 | 已定位 | `TestChangeNoticeRejectsBody` `TestProjectionNotifyHTTPRejectsObservationBody` `TestProjectionControllerNoticePullsStateWithoutChangingSnapshot` `TestProjectionNotifyPullsBoundStateWithoutChangingHEAD` |
| I-22 | 独立 runtime + OpenSearch 容器 | HTTP facade operations projection sync/search | 动态字段发现候选、同 basis hydrate、Snapshot 不变 | 已定位 | `make test-state-runtime-e2e` |
| I-23 | 固定 Workspace + 显式 KnowledgeRef 候选 | typed RERANK | 逐 Ref 授权、同 pin Canonical 回读、EvaluationProjection 后才调用 Provider；返回 SearchView 与模型/spec/candidate digest 证据 | 已定位 | `TestHTTPRerankReadsAuthorizedCanonicalCandidatesAndProjectsModelFields` |
| I-24 | Reranker 返回未知、重复、遗漏或不允许的未评判 Ref | 执行 Refine | `PRECONDITION_FAILED` / `CAPABILITY_UNSATISFIED`；Provider 不能生成知识或改写输出合同 | 已定位 | `TestExecuteRerankRejectsDishonestOrIncompleteProviderOutput` / `TestExecuteRerankFailsClosedForUnjudgedWhenContractForbidsIt` |
| I-25 | SEARCH 命中进入语义重排 | `search:rerank` | 同一 SearchView、真实 lane/originalRank 进入结果证据但不进入模型请求；只调用一次 Provider | 已定位 | `TestHTTPSearchRerankPreservesRetrievalEvidenceAndUsesOneFixedView` |
| I-26 | 单候选或候选窗超过模型可见字节预算 | RERANK | 出站前 `USAGE_INVALID`，Provider 零调用，不自动分批 | 已定位 | `TestExecuteRerankRejectsOversizedModelVisibleInputBeforeProvider` / `TestExecuteRerankRejectsOversizedAggregateWindowBeforeProvider` |
| I-27 | Responses-compatible Luna | 真实 listwise RERANK | `gpt-5.6-luna`、`reasoning=none`、strict structured output；Provider 合同与 Repository→OpenSearch→HTTP→Luna 完整链路均验证明显相关候选第一且引用守恒；默认跳过、显式付费运行 | 已定位 | `TestLiveLunaListwiseRerank` / `TestLiveHTTPSearchRerankWithLuna` |
| I-28 | RERANK 已形成投影候选窗 | 成功或 Provider 失败 | access 后 durable append `refine.jsonl`；仅投影值、完整模型输出/错误、basis/lane/prompt revision；成功响应返回 `rf_*` | 已定位 | `TestRerankEvidenceFeedbackAndTrainingSampleJourney` |
| I-29 | Agent 用候选回答，用户接受 | `answered` + `accepted` feedback | 两者按 refineEvidenceId/trace 关联；selectedRefs 必须属于候选窗；派生 `accepted-answer` 可训练样本 | 已定位 | `TestRefineEvidenceTraceAndTrainingSamplesAreRebuildable` / `TestRerankEvidenceFeedbackAndTrainingSampleJourney` |
| I-30 | 反馈是晚于 Agent 请求的独立调用 | typed feedback | 被评价 trace 与 submissionTrace 分离；候选窗外引用拒绝；Agent 自评或多答案歧义不升级，只有用户确认/纠正可形成强标签 | 已定位 | `TestTypedRefineQueryDoesNotUseCurrentRequestTraceAsFilter` / `TestRerankTrainingDoesNotPromoteSelfAcceptedOrAmbiguousAgentAnswers` / `TestRerankTrainingOnlyPromotesHumanCorrections` |
| I-31 | Provider 返回非法候选或审计窗含成功/失败混合记录 | RERANK + refine query | 非法完整输出随稳定错误落证据；evidence/trace/provider/model/outcome 过滤有效，limit 返回最新匹配记录 | 已定位 | `TestRerankEvidenceFeedbackAndTrainingSampleJourney` / `TestRefineEvidenceTraceAndTrainingSamplesAreRebuildable` |
| I-32 | SEARCH / RELATION 返回固定候选窗 | retrieval evidence | durable `rt_*` 关联 access；保存 logical request、SearchView、candidate rank/lane/value digest/observation、执行统计与错误；响应返回 retrievalEvidenceId，append 失败则成功结果不交付 | 已定位 | `TestRetrievalEvidenceQueryTraceAndTrainingAreRebuildable` / `TestHTTPSearchRerankPreservesRetrievalEvidenceAndUsesOneFixedView` / `TestRelationRepositoryWorkspaceAndHTTPUseOneExactBasisExecutor` / `TestKnowledgeSearchFailsClosedWhenRetrievalEvidenceCannotPersist` |
| I-33 | search:rerank 下游 refine 失败或用户只反馈 refine | 跨阶段关联 | 已完成 SEARCH 不继承下游失败；rf→rt 稳定关联，反馈 join key 自动传播，retrieval/rerank 训练视图均可重建 | 已定位 | `TestSearchRerankRecordsCompletedRetrievalWhenOnlyRefineFails` / `TestTypedRetrievalEvidenceQueryAndTraining` |
| I-34 | 仓已 COMMIT、Desire 未落盘 | serve 重启或 `CatchUp` | live 投影 `basis == HEAD` 且 READY；SEARCH 命中，无需手工 `projection sync` | 已定位 | `TestProjectionReconcileRecoversMissingDesire` |
| I-35 | 无 AfterSnapshot / 不挂 hook | `Start` 周期对账 | 结果与通知完整时相同：live basis=HEAD 且可搜 | 已定位 | `TestProjectionStartRecoversWithoutDesire` |
| I-36 | `READY && Applied==Desired` 但 HEAD 已前进 | `CatchUp` | 不得 skip；必须 `Desire(HEAD)` 后再 Ensure | 已定位 | `TestProjectionCatchUpDoesNotSkipWhenHeadMoved` |
| I-37 | 消费 SEARCH / 一次性 `Open()` | 读路径 | 不调用 Rebuild/Ensure/CatchUp/Start；P-01 仍成立 | 已定位 | `TestConsumerPathsDoNotMaintainProjectionOrScanAuthority` / `TestProjectionWorkerStartsOnlyFromServeFacade` / `TestLocalCLISearchDoesNotCatchUpProjection` |
| I-38 | live 仍停在旧 commit、HEAD 已前进 | SEARCH | 旧 READY commit 可搜；新 HEAD 明确未就绪，CatchUp 完成后新 HEAD 可搜 | 已定位 | `TestProjectionUnappliedHeadIsNotSearchableUntilCatchUp` / `TestOpenSearchWarmRebuildKeepsReadyGenerationQueryable` |
| I-39 | 长寿命 `kc serve` 已 Start | COMMIT 后不跑 `projection sync` | 消费 SEARCH 最终命中 published HEAD | 已定位 | `TestServeProjectionWorkerCatchesCommitWithoutSync` |
| I-40 | 真实发布的时间/整数 Schema 与长正文 | 发布、编译、查询、分页和回读 | 相邻 long 与纳秒保真；text-only 不生成整值索引 | 已定位 | `TestPublishedSchemaTypedSearchPreservesLongTextIntegersAndTime` / `TestProjectionCompilerOnlyBuildsDeclaredSlots` / `TestTemporalSchemaPublicationAndInstanceValidation` |
| I-41 | 后端 HTTP 200 但超时/分片失败，或 PIT 创建交错发布 | SEARCH / RELATIONS | 拒绝虚假完整性和混 basis；失效游标不跟新版本 | 已定位 | `TestOpenSearchIncompleteResponsesCannotProduceCandidates` / `TestOpenSearchContinuationRequiresCurrentPublication` / `TestOpenSearchPITCreationRejectsConcurrentPublication` |
| I-42 | 多批增量涉及不同分片；仅非索引内容变化 | Apply 与发布 | 全部变更可见才 READY；无变化不写；与 rebuild 一致 | 已定位 | `TestOpenSearchApplyWaitsForEveryTouchedShard` / `TestIncrementalProjectionSkipsUnchangedDocumentsAndMatchesRebuild` |
| I-43 | 候选持续被补判淘汰或调用取消 | 有界执行与续页 | 预算耗尽保留可继续位置，取消传给 provider；准备失败不发假游标 | 已定位 | `TestSearchBudgetStopsResidualPagesAndCanResume` / `TestSearchPropagatesCancellationToContextProvider` / `TestSearchTimeBudgetReturnsResumablePartial` |
| I-44 | 多仓、短预算页、批内只消费部分命中 | Workspace 归并 | 有界并发与批量；不漏不重；未知头部停止归并 | 已定位 | `TestWorkspaceBuffersAndResumesConsumedOffset` / `TestWorkspaceInitialFetchConcurrencyIsBounded` / `TestWorkspaceResumeAcrossShortBudgetBatch` / `TestWorkspacePartialUnknownHeadStopsMerge` |
| I-45 | 多个身份处于同一固定 commit | ReadMany | 只读请求身份的对象 locator，唯一 unit 不重复读，不解码全仓 manifest | 已定位 | `TestReadManyLoadsOnlyRequestedObjectLocatorsAndUnits` / `TestSingleObjectReadDecodeBytesAreIndependentOfRepositorySize` |
| I-46 | 规范词表、概念引用、关系与后续版本 | 定位词表后过滤、渐进读取 | 消费不依赖向量；证据身份和原 basis 不漂移；不冒充模型质量评测 | 已定位 | `TestControlledVocabularyAndProgressiveReadingUseFixedKnowledgeBasis` |
| I-47 | Workspace 共享预算不足，或已有批内偏移 | 首批取头部与空页续行 | 无进展明确失败；真实补判/回放仍可续；回放保留批量 I/O | 已定位 | `TestWorkspaceBudgetCannotReturnEndlessEmptyContinuation` / `TestWorkspaceBudgetFairHeadsProduceProgress` / `TestWorkspaceBudgetDefaultPageCapCannotStall101Members` / `TestWorkspaceBudgetResidualEmptyPageRetainsRealProgress` / `TestWorkspaceBudgetOffsetReplayDebtCanAdvanceOnEmptyPages` / `TestWorkspaceBudgetPrimingReplaysSavedOffsetInOneBatch` |
| I-48 | lakeFS 夹具与内存 tree 执行同一确定性 Operation 脚本 | 按步骤而非 commit ID 对齐 | 值、状态、声明、来源、历史、变化、维护分页和失败码逐观察相等 | 已定位 | `TestLakeFSMatchesTreeProviderByOperationStep` |
| I-49 | 10 与 1000 对象仓各做一次相同点写/点读；旧 locator 显式重建 | 统计 authority 读取次数、ListFiles 与解码字节 | 点写/点读成本相等，ListFiles 为 0；普通写不暗中迁移，维护入口恢复 | 已定位 | `TestSingleObjectPutCostIsIndependentOfRepositorySize` / `TestSingleObjectReadDecodeBytesAreIndependentOfRepositorySize` / `TestExplicitLocatorRebuildRecoversLegacyLayout` |
| I-50 | 变化能力报错、缺 provider 能力、unit 代数 | 增量/装配/写能力拒绝与结构检查 | 原错误失败关闭、不触发 rebuild、不 panic、不暴露 raw TreeStore；代数无文件形状 | 已定位 | `TestEnsureFailsClosedWhenIncrementalChangeLookupFails` / `TestBaseAuthorityWithoutKnowledgeCapabilitiesFailsClosed` / `TestMissingProviderCapabilitiesFailWithoutPanic` / `TestProviderNeutralUnitAlgebraHasNoFileStorageShape` |
| I-51 | command 预留后中断、提交后回执丢失、旧回执清理、ledger 读失败 | 重启、显式 resolve/abandon、保留窗口 | 不重放未知结果；PENDING 不被推断；Bolt 清理不加载历史；读失败不执行 | 已定位 | `TestCommandLogRecoversReservationBeforeCommit` / `TestCommandLogRecoversCommitBeforeReceipt` / `TestCommandLogRetentionIsBoundedAndKeepsPending` / `TestBoltCommandLogPrunesWithoutDeletingPending` / `TestCommandLogReadFailureCannotReapplyCommand` / `TestAcceptedCommitSurvivesEvidenceFailure` |
| I-52 | CLI/HTTP READ/SEARCH 与 Writer typed intent | transport wiring、命名类型和 import 可达集 | 共用 typed executor；核心无 flags/HTTP/provider 依赖；标识类型不可混 | 已定位 | `TestCLIAndHTTPUseSameTypedApplicationExecutor` / `TestApplicationCoreHasNoTransportOrProviderImports` / `TestApplicationRequestsUseOwnedIdentifierTypes` / `TestCommitExecutorPreservesTypedIntent` |
| I-53 | 同一 Operation 脚本切换 tree 与 LakeFS+对象存储 provider；并发 expected-old 发布 | shared Repository/Writer contract + 按步骤对拍 + hidden branch lock | 固定版本、Aspect、来源、历史、diff、CAS、幂等、proposal/merge、归档和失败码等价；对象字节走预签名直传；并发发布只有一个成功 | 已定位，协议级 fake 实跑 | `TestLakeFSRepositoryContract` / `TestLakeFSWriterContract` / `TestLakeFSMatchesTreeProviderByOperationStep` / `TestLakeFSPublicationLockPreventsConcurrentLostUpdate` / `TestLakeFSObjectBytesUsePresignedDataPlane` |
| I-54 | 静态 Aspect 发布后动态 Recipe/Binding observation 推进 | 同一 Controller 两条独立 lane | 静态投影推进 commit basis；动态 notice pull observation，HEAD 和静态 basis 不动 | 已定位 | `TestIngestionControllerKeepsStaticAspectAndDynamicRecipeLanesSeparate` |

[索引控制](index-control.md)的拼装 / Serving State / 同 basis hydrate 已由 I-15..I-20、I-22 与 Knowledge Serving 覆盖。
I-21 已收口 notice → 控制器 pull；I-34..I-39 仍只对账 Snapshot HEAD。真实 Observer Docker 首版（§11.3）未齐，不能把当前双容器适配器旅程记成整组 D 已通过。

### 2.9 P 授权 / Hook / Gate（facade）

本表是实现证据。P-14 / X-06 跟随 `AUTH-01`：SEARCH 发现全部 pin 成员，无 `knowledge.read` 时屏蔽正文且不是 `partial`；精确读仍 fail closed。P-21 跟随 `AUTH-03`：交付链是定位与返回之间的独立层。P-22 / P-23 的旅程束按入口节点 `_bundles.yaml` 的 `walk` 串状态目录：P-22 沿脊到 `semantic-knowledge-published` → `projection-synced` 后单仓消费；P-23 从语义知识分叉到 `knowledge-set-defined`。声明面是 `knowledge-search-granted/_probes/schema-search-enforces-declared-access.feature`；交付屏蔽与授读是该宿主上的独立 probes；身份绑定在初始化前态，Dataset 按人不继承见 `dataset-query-principals-granted/`。`"""` 任务块不是协议 Oracle。

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| P-01 | 正式 Client 缺身份 | 业务命令 | 必须显式 principal；测试夹具 owner 不能成为产品默认授权 | 已定位 | `TestRemoteCLIRejectsHomeAndMissingPrincipal` |
| P-02 | 空 allow | `--as bot put` | `FORBIDDEN`；七列不动 | 已定位 | write errors / LifecycleAndAllow |
| P-03 | 只 allow `read-workspace` | `--as` member consumption | `FORBIDDEN` | 已定位 | `TestKnowledgeSetAuthorizationCoverageIsHonest` |
| P-04 | Catalog A 的 allow | Catalog B `--as` | 不继承 | 已定位 | `TestCatalogIsolationDoesNotShareAllow` |
| P-05 | pre `writer put` hook 非 0 | `writer put` | `HOOK_DENIED`；无 commit | 已定位 | `TestPrePutDeniedLeavesNoCommit` |
| P-06 | REPLAYED | hook | 不打 | 已定位 | `TestReplayedSkipsHook` |
| P-07 | post hook | 成功写 | payload 只有指针，无正文 | 已定位 | `TestPostPutPointersOnly` |
| P-08 | post 失败 | 已 dataset define | **不**回滚登记表 | 已定位 | `TestPostDefineKnowledgeSetPointersOnlyAndFailureDoesNotRollback` |
| P-09 | `operations hook add` / `operations gate add` | CRUD | stateDir 下的 `hooks.json` / `gates.json` | 已定位 | `TestHookAndGateConfigCRUD` |
| P-10 | 仓内 `permissions` Aspect | `kc read` | **不是**闸门；GRANT 不进 `allow.json` | 已定位 | `TestUserJourneyKnowledgeGrantDoesNotAuthorizeAccess`；T8 可裁 |
| P-11 | 已 allow | 撤销授权、查询规则与当前身份（精确调用见用例） | 规则消失后 `--as` 拒绝 | 已定位 | `TestUserJourneyManageAgentAccess` |
| P-12 | `serve --config`，配置 `auth: local` | 仅 `X-Kc-As` | 与 `--as` 同一授权规则；空身份 `UNAUTHENTICATED`；`Authorization` 或 `X-Kc-On-Behalf-Of` 被拒 | 已定位 | `TestXKcAsUsesTheSameAuthorizationRulesAsCLI` / pairing tests |
| P-13 | `serve --config`，配置 `auth: gitea` | PAT / Basic → `/api/v1/user` | `gitea:<id>`；伪造 `X-Kc-As` 和管理口提权被拒 | 已定位 | `TestLiveServiceProviderConsumerJourney` / `make test-service-e2e` |
| P-14 | Dataset 两仓清单，授 Dataset `file.read`，只给一仓 `knowledge.read` | READ / pin SEARCH / `--repo` READ | Dataset 通道读清单内文件；`--repo` 无仓权 fail closed；CLI SEARCH 只列身份，HTTP SEARCH 保留 Canonical；SearchView 保留两仓 | 已定位 | `TestKnowledgeSetAuthorizationCoverageIsHonest` `TestRepoSearchDeliveryStripsUnauthorizedBody` `TestHTTPWorkspaceSearchKeepsDatasetFileReadBody` `TestDatasetFileReadDoesNotImplyRepoKnowledge` |
| P-15 | 部署未声明配置或认证模式 | 启动 | 失败关闭，不得静默采用默认认证 | 已定位 | `TestServeRequiresExplicitDeploymentConfiguration` / `TestHTTPServerOptionsFromFlags` |
| P-16 | 任意 `--auth` | 无凭证 `GET /identity/v1/auth` | 报告 `mode`、`localAssertion`、`accepts`；不是会话 | 已定位 | `TestIdentityAuthDiscovery` |
| P-17 | Server `--auth local` | `kc login --mode local --as` 后 whoami | principal 为断言主体；同一 Client 打 Taihu Server 失败关闭 | 已定位 | local login / pairing tests |
| P-18 | Server `--auth taihu` | 仅 Bearer / 再加 `X-Kc-As` | 用户 token 注入 `taihu:<username>`（工号只在 `subject`）；缺 username 失败关闭；混装 `FORBIDDEN`；仅 `X-Kc-As` `UNAUTHENTICATED` | stub 已定位 | pairing / fake introspection tests |
| P-19 | `--mode token` | 已签发 Bearer | 只发 `Authorization`，不发 `X-Kc-As`；不是第三种配对 | 已定位 | token login tests |
| P-20 | 真实 Taihu IAM | 浏览器 PAR/PKCE 或已签发 Bearer + introspection | `whoami` 为 `taihu:<username>`（或 agent/service 映射），不是工号；错配失败关闭 | gated | `scripts/live-taihu-auth.sh` / `TestLiveTaihuAuthentication` / `make test-taihu-live` |
| P-21 | 已 hydrate 的知识 ID 信封 | `delivery.Chain.Apply` | 空链原样返回正文；无读权保留 ID、清空正文；有读权保留正文；改 ID/Address `PRECONDITION_FAILED`；后续 Stage 看到前一段输出且可改写正文 | 已定位 | `TestEmptyChainReturnsHydratedBody` `TestRepositoryReadStripsUnauthorizedBodyAndKeepsID` `TestRepositoryReadKeepsAuthorizedBody` `TestChainRejectsIdentityMutation` `TestChainRunsLaterStagesOnStrippedEnvelope` `TestLaterStageMayRewriteVisibleBody` `TestFromValueRoundTripWritesOnlyBody` |
| P-22 | Schema 声明 name=`text+filter`、expression=`text`、unit=`filter`、measureKey 无 access；只授 `knowledge.search` | MATCH / EQ / field MATCH / READ，再授 `knowledge.read` | `schema.access`：只在声明面上定位；错面 `CAPABILITY_UNSATISFIED` 或零命中。`catalog.allow`：无读权命中清空正文、READ `FORBIDDEN`；授读后见 Canonical（含未编进索引的字段，作为实例证人） | 已定位 | `.data/scenes/` `knowledge-search-granted/` `knowledge-read-granted/` `TestMetricPermissionScenes` / `KC-AGENT-01` |
| P-23 | local HTTP 三种主体 `taihu:alice` / `agent:copilot` / `service:etl`；grant 按人配置 | 场景过程：whoami → SEARCH/READ → 给 etl search → 给 alice read | `identity.bind`：空凭证 `UNAUTHENTICATED`；拒自报 onBehalfOf。`catalog.allow`：授权键是 principal；他入 grant 不继承；无读权 SEARCH 屏蔽正文、READ `FORBIDDEN` | 已定位 | `.data/scenes/` `principals-granted/` `TestMetricPermissionScenes` / `KC-AGENT-01` |
| P-24 | 已认证无 `catalog.read` | 公开 Catalog show；私有 Catalog show；成员 `knowledge.read` | 公开可发现；私有仍 FORBIDDEN 至 grant；发现不等于正文读 | 已定位 | `.data/scenes/` `catalog-initialized/` `catalog-read-granted/`（私有发现为初始化宿主的 Go evidence） `repository-attached/_probes/probe-inventory-without-body.feature` `TestAuthenticatedPrincipalDiscoversPublicCatalogWithoutGrant` / `TestPrivateCatalogStillRequiresCatalogReadGrant` |
| P-25 | 已认证无仓 grant | 未声明系统仓读；声明业务仓默认可读；声明系统仓默认可读 | 未声明 FORBIDDEN；声明后可读不可写；系统仓走同一声明，不按保留 ID 放行 | 已定位 | `.data/scenes/` `repository-attached/_meta.yaml`（仓默认可读证据） `TestUndeclaredSystemRepositoryStillRequiresGrant` / `TestAuthenticatedPrincipalReadsDeclaredRepositoryWithoutGrant` / `TestDeclaredSystemRepositoryUsesAuthenticatedDefault` / `TestRuntimeWriterRefusesSystemRepository` |

### 2.10 N 入站 connector（不是 hook）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| N-01 | Observed 多于 Desired | `Preview(patch)` | 不因多余 Observed 而 REMOVE | 已定位 | `connector/preview_test.go` |
| N-02 | Desired 缺、Observed∩Scope 有 | `Preview(reconcile)` | REMOVE 那些 Address | 已定位 | 同上 |
| N-03 | Desired 超 Scope | Preview | `SCOPE_DENIED` | 已定位 | 同上 |
| N-04 | 无漂移 | Preview | `empty=true`，不强迫 COMMIT | 已定位 | 同上 |
| N-05 | 预览非空 | `Writer.Commit` | SOURCE 落盘 | 已定位 | `TestPreviewThenCommit` |
| N-06 | 任意 | `kc connector-run` | **不存在**（入站不是 CLI 插件宿主） | 已定位（负例） | walkthrough D.2 |
| N-07 | 任意 | `kc reconcile` | 不存在；外部 connector 调 kit 后提交 ChangeSet | **frozen（分层边界）** | CLI Help、[外部资源访问](resource-access.md)、T7 API |

### 2.11 S 适配器（K-23）

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| S-01 | 私有 memory fake + Reader/Writer | provider-independent 组合合同 | Snapshot 身份/CAS/历史 + LOG/DIFF/REMOVE/Archive/schema_ref/PROPOSAL | 已定位 | `TestProviderIndependentRepositoryContract` |
| S-03 | Gitea + Reader/Writer | 同一份 T12 | Adapter 无工作区且不解释知识；上层读 pinned commit | 已定位 | `TestT12GiteaContract` |
| S-04 | local profile 无 provider | SEARCH | `CAPABILITY_UNSATISFIED`；精确 READ/VFS 不受影响 | 已定位 | `TestLocalProfileHasNoSearchProjection` |
| S-05 | OpenSearch Retriever/Maintainer | 原子 SEARCH 算子 | 已实现叶子 Probe Exact（含 PREFIX/CONTAINS）；未声明或未知算子 → `CAPABILITY_UNSATISFIED` | 已定位 | `TestOpenSearchProbeTypedSubset` / `TestOpenSearchOperators` / `TestOpenSearchContainsUsesEscapedKeywordWildcard` |
| S-06 | 所有 Retriever | CandidateRef | 不返回正文/stored payload | 已定位 | engine interface + search tests |

### 2.12 F HTTP 门面

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| F-01 | serve 已起 | 接入方经 typed Writer 发布并 `--repo` 读回；治理方 compose/grant/sync 之后，消费方经 `catalog list/show` 发现入口，再 resolve/search/read | Client 与 HTTP route 调用同一应用服务；产品角色不打开 `--home`，库存 JSON 不含宿主路径或 Snapshot selector；消费 SEARCH 失败不教运维命令 | 已定位 | `TestRemoteProviderReadBackAndConsumerDiscovery` / `TestLiveServiceProviderConsumerJourney` / `make test-service-e2e` |
| F-02 | 无 allow | `X-Kc-As: bot` | `FORBIDDEN` | 已定位 | serve_test |
| F-03 | HTTP dataset define | 登记表 git | stamp 含 as / request-id | 已定位 | serve_test |
| F-04 | `kc serve` 已启动 | 旧 verb 路由或未知资源 | 404 | 已定位 | service route contract |
| F-05 | `kc serve` 已启动 | 正式 Catalog/Knowledge/Writer/Governance route | 单机与共享部署均使用同一 typed Client/HTTP 语义；HTTP 不走 CLI command table | 已定位 | `TestFormalServiceNamespacesAreExplicitAndRetiredRoutesStayMissing` / remote CLI tests / live service journey |
| F-06 | MCP | — | 未实现 | **frozen** | walkthrough D.2 |

### 2.13 O 运行可观测性

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| O-01 | `kc serve` | 产品 HTTP 请求 | OTel metric + SERVER/application span；指标无 request/repo/object 等高基数标签 | 已定位 | `internal/telemetry` / `cli/serve_telemetry_internal_test.go` |
| O-02 | OTLP logs 已配置 | 产品 HTTP 请求 | 每请求至多一条 `kc.http.request.completed`；requestId、traceId、spanId 可关联，正文/凭证/query 不入日志 | 已定位 | `TestObservedHTTPHandlerCorrelatesCompletionLogAndSuppressesManagementNoise` |
| O-03 | management 流量 | `/metrics` / `/health` / `/livez` / `/readyz*` | 保留 transport metric，不导出 completion log 和 trace，避免探针淹没业务信号 | 已定位 | 同上 |
| O-04 | Compose observability profile | 真实 SEARCH、Canonical READ、Workspace resolve | Prometheus 原始指标/rules、Tempo trace、Loki log、五个 provisioned Grafana dashboards 均可查询；同一 traceId 跨 log/trace 对账 | 已定位 | `make check-observability` / `make deploy-local-smoke` |
| O-05 | Gitea/OpenSearch/resource-access/MySQL | 跨进程调用 | 标准 CLIENT/SERVER span 与 W3C context 覆盖完整依赖图 | gap | 当前 Jaeger 依赖视图只证明 `kc-server` 内部 span，不能冒充静态系统架构 |
| O-06 | Collector/Loki/Jaeger | 生产部署 | 持久存储、备份、租户隔离、tail sampling、容量与故障演练 | gap | Compose profile 仅是 24h/内存本地验收拓扑 |
| O-07 | 30 天 SLO | SEARCH/READ/Writer 可用性与 latency good-event ratio | 有 error-budget remaining，且 `1h+5m@14.4x`、`6h+30m@6x`、`1d+2h@3x` 多窗口 burn-rate 告警可证明 firing/recovery | partial | SEARCH/READ/Writer 已有多窗口 availability burn、latency good-event、30 天 budget recording 与面板；规则专项只证明全失败、无流量等 availability 边界；完整性 eligible/profile 的原始维度、各告警 dashboard/runbook 定位与完整 firing/recovery 证据，以及真实 30 天/规模基线仍缺 |
| O-08 | Snapshot/Binding/identity provider/Hook/Gate | 真实依赖调用 | 实现 rate/error/duration/in-flight/bytes/backlog 所需的低基数原始指标和 child span | partial | 身份 provider、State Binding、Writer、Projection、Hook/outbox、Gate、VFS 与 Snapshot decorator（`kc.snapshot.*` RED/active/bytes，store=`lakefs\|gitea\|other`）已接真实边界和包测试；Gitea/OpenSearch/lakeFS 出站 HTTP 的跨进程 W3C CLIENT/SERVER 传播仍缺 |
| O-09 | OTel Collector/Jaeger/Loki/Prometheus | backend 慢、断开或队列满 | Collector accepted/refused/enqueue-failed/send-failed/queue 与 backend ingest/query/storage 自监控可见并告警 | partial | 已 scrape Collector internal metrics 并预置 unavailable/export failure/refused/queue saturation 告警；Jaeger/Loki ingest/query/storage 自监控与故障演练仍缺 |
| O-10 | 规模负载 | Workspace/Search/Writer/Projection/Evidence 放大 | 容量面板同时展示输入负载、fan-out/工作量、队列/饱和与用户延迟，并与 `scale-benchmark.md` 档位对齐 | partial | 容量/行为面板已有 operation input、Writer payload/change、Snapshot calls/bytes、READ object/unit、Projection docs/change/backlog、Evidence bytes/disk、VFS bytes/entries；缺 projection ETA 与压测基线 |
| O-11 | access/feedback/system/audit evidence | 身份与用户行为分析 | 分离采用、治理和安全视图；可聚合 DAU/WAU、委托、拒绝、仓/工作区采用、零结果/refine/feedback，不把 principal 做 metric/Loki label | partial | 原始可信 evidence、trace 查询、hitmap，以及 provider/principal-kind/delegated/authn/authz 有界聚合面板已有；缺受控高基数聚合存储/作业、权限分面、委托验证和异常规则 |
| O-12 | 专用 canary Repository | 定时 resolve→READ、commit→SEARCH、evidence reconciliation 与故障注入 | 黑盒 correctness/availability/freshness 信号与每类告警 firing/recovery 证据 | gap | `deploy-local-smoke` 只验证组件链路和查询定义，不是定时黑盒探针或告警故障演练 |
| O-13 | 发布/配置变化 | incident 调查 | service version、telemetry schema、受控 config digest 和 deployment annotation 可与 SLO/资源时序对齐 | partial | OTel Resource 已有 service/schema version；缺配置 digest 和 Grafana 发布标记 |
| O-14 | 已持久化 access 原始账 | 按时间窗、repository、principal 查询；`Get(evidenceId)`；continuation 取更旧页 | 最新匹配窗口、页内时间顺序；点查在 ack 后可见；hitmap 使用同一过滤且按聚合条目分页；非法 continuation/`since` 为 `USAGE_INVALID`；日分区热窗删除与配额/flood-stage fail-closed | 已定位 | `TestFileStoreAccessQueryByTimeRepositoryPrincipalAndContinuation` / `TestHTTPAccessLogQueryFiltersAndPages` / `TestFileStoreDatePartitionHotDeleteAndQuota` / `TestFileStoreFloodStageFailsClosed` / `TestFileStoreFailsClosedWhenPartitionPathIsAFile` |

### 2.14 D 协议已冻结、参考实现未做

这些**不要**写成正路径用例。若暴露入口，预期是 `CAPABILITY_UNSATISFIED` 或「未知命令」。

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| D-01 | 任意 | `CAPABILITIES` 独立清单 | 未暴露；缺失能力必须显式 | frozen | `TestRemovedCommandsAreRejected` |
| D-02 | 任意 | `EXPAND_RELATIONS` | 未实现 | frozen | 同上 |
| D-03 | 任意 | `WATCH_UPDATES` | 投递端是 post hook；无订阅口 | frozen | 同上 |
| D-04 | 任意 | Knowledge `LIST_TREE` 父子枚举 | 无公开 Knowledge 枚举；Workspace File Gateway 是独立宿主投影合同 | frozen | 同上 |
| D-05 | 有 Stream Binding | 普通 READ / 流 SEARCH / `tail` | 不支持普通 READ；Stream window/query 与投影仍未实现，不得伪造流结果 | frozen（负例已定位） | `TestOrdinaryReadRejectsStreamBinding` / frozen command tests |
| D-06 | Fork 发布 | 自动三方 sync（K-15） | 当前发表路径是目标仓 `governance proposal create` 新对象；自动三方 sync 未做 | 已定位当前路径 / frozen 自动 sync | `TestForkPublishDoesNotCopyPersonal` |
| D-07 | Vendor Repository | 生成只读副本（K-16） | 未做 | frozen | — |
| D-08 | 两次 OpenKnowledgeSet | ViewDiff | 未做 | frozen | — |
| D-09 | 原子查询可用 | RQL 文本语法（OR/NOT/括号） | CLI 原子子句隐式 AND；typed Client 已有 All/Any 表达式，不等于提供 RQL/NOT | frozen（文本语法） | — |
| D-10 | 上游知识更新 | 检查引用方仓 commit | 禁止跨 Repository merge；下次 ResolveKnowledgeSet 重解 | 已定位 | `TestUserJourneyUpstreamUpdateDoesNotRewriteReferencingRepository` |

---

## 3. 错误码覆盖

协议信封一律 `{error:{code,message}}`。无测试的码补齐时优先。

| code | 该出现的前置 | 现况 |
|---|---|---|
| `USAGE_INVALID` | 缺 flag、未知命令、空 changeset、search/stream 形状、未挂载仓/流、无 code 的 `fmt.Errorf` 归一 | 已定位 CLI |
| `PRECONDITION_FAILED` | IfAbsent / DERIVATION / digest / stale cursor / dirty worktree | 已定位 |
| `NON_FAST_FORWARD` | 过期 expected / merge 时 main 被推走 | 已定位 |
| `OBJECT_ID_CONFLICT` | 重复 Address / blob+aspect 混 | 已定位 |
| `IDEMPOTENCY_CONFLICT` | 同 command_id 异 digest | 已定位 |
| `EVENT_ID_CONFLICT` | 同 eventId 异 payload | 已定位 |
| `POSITION_REGRESSION` | 声明 `MONOTONIC_PER_PARTITION` 的实现才可达；Base profile=`NONE` | frozen；数仓 connector checkpoint 有独立可执行断言 |
| `WRITE_TARGET_REQUIRED` | 写未指定唯一 repository / ref（空 changeset 是 `USAGE_INVALID`） | 已定位 |
| `SURFACE_MISMATCH` | Surface 与地址不符 | frozen：当前公开请求由独立结构表达 Surface，不接受可冲突字段 |
| `SCOPE_DENIED` | connector Desired 超 Scope | 已定位 |
| `SCHEMA_UNSUPPORTED` | Domain Schema 不符合受支持的 Meta Schema | 已定位 |
| `SCHEMA_REVISION_UNRESOLVED` | 钉的 schema 不可解析 | 已定位 |
| `SCHEMA_INSTANCE_INVALID` | PUT Address/value 不符合已解析 Domain Schema | 已定位 |
| `SCHEMA_INCOMPATIBLE` | 同一 Schema object ID 的新合同会破坏既有实例或查询 | 已定位 |
| `TARGET_REPOSITORY_DENIED` | 把 Catalog id 当 Snapshot Repository 写目标 | 已定位 |
| `KNOWLEDGE_REF_UNRESOLVED` | 维护读缺对象；lookup 缺 event | 已定位 |
| `VERSION_UNRESOLVED` | 未知 commit / 不存在的 ref | 已定位 |
| `CAPABILITY_UNSATISFIED` | 未声明 SEARCH 车道；stub 未实现 | 已定位 |
| `TEMPORARY_UNAVAILABLE` | 瞬时 Backend I/O（Gitea/hook HTTP）；不是未挂载 | 已定位 |
| `CANDIDATE_MOVED` | preview 后 candidate 前进 | 已定位 |
| `VALIDATION_BASIS_MISMATCH` | 旧 PASSED 绑新 Preview | 已定位 |
| `KNOWLEDGE_SET_INVALID` | Workspace 配方不能用：无 Workspace / 重复 source / 已 retire / selector 无此 ref | 已定位 |
| `FORBIDDEN` | `--as` 未命中 allow | 已定位 |
| `CATALOG_ARCHIVED` | 归档后 dataset define | 已定位 |
| `REPOSITORY_ARCHIVED` | 归档后写 | 已定位 |
| `GATE_UNSATISFIED` | merge 缺证据 | 已定位 |
| `HOOK_DENIED` | pre 非 0 | 已定位 |

---

## 4. 交叉格（正交维里必须测的乘积）

只测这些，不要笛卡尔爆炸。

| ID | 前置 | 操作 | 预期 | 现况 | 已有测试 |
|---|---|---|---|---|---|
| X-01 | W5 candidate 存在 | 消费读 | propose 期间 `read --dataset` 仍旧 main | 已定位 | S3 |
| X-02 | W5 candidate 存在 | 观察索引 | propose 不 `AfterSnapshot` | 已定位 | I-02 |
| X-03 | W6 Preview 存在 | 读取 Catalog | Preview 不写登记表 git 配方 | 已定位 | M-02 |
| X-04 | 命令内 pin | 并发 merge | 本次结果仍旧 pin；**下次**命令见新 HEAD | 已定位 | API serving；CLI 一命令一 pin，跨命令跟已发布 Dataset 或 `--repo --commit` |
| X-05 | Binding declaration pin | Descriptor 后续更新 | 旧 pin 仍解析旧 runtime/digest | 已定位 | `TestResolveDescriptorBindingAtPinnedCommit` |
| X-06 | 联邦 Workspace | 只 allow 一仓 `knowledge.read` | 裸知识读 fail closed；SEARCH 两仓都进候选、无读权屏蔽正文、不是 `partial` | 已定位 | P-14 |
| X-07 | Catalog 已归档 | 写个人仓、define Workspace | 禁 define；个人仓仍 COMMIT | 已定位 | S6 |
| X-08 | 有 schema_ref | propose | 与 COMMIT 同一套解析 | 已定位 | `TestSchemaRefOnPropose` |
| X-09 | 已有成功 command_id | 重放带 Hook 的命令 | REPLAYED 不打 hook | 已定位 | P-06 |
| X-10 | 已配置 Gate 与 Hook | pre-merge | pre-merge 成功 ≠ 清单满足 | 已定位 | M-12 |
| X-11 | 已移除 driver 名 | 部署配置验证 | 不回流成 authority/index/cache | 已定位 | W-09 |
| X-12 | permissions Aspect 存在 | `kc admin grant add` 与 READ | 快照可读；不放行 SELECT、不当 read 闸门 | 已定位 | P-10 |

---

## 5. 补齐优先级

优先钉住静默成功、越权、错误 basis、原子性和恢复风险，再补状态观察点及 provider/transport
转换边界。具体未覆盖项留在 §2/§4 各自风险行，作者流程见 §0.2；本节不重复完成状态。

数仓业务、源客户端与真实业务规模夹具只属于墙外 suite。不要把 Projection 当 Canonical，
不要把 `permissions` Aspect 当 KC READ 闸门，也不要让 `governance preview validate` 运行场景套件。

---

## 6. 最小走通脚本（补齐时的手工对照）

自动化以 `make test` / `make test-all` 为准。Gitea 合同归其 adapter 包，lakeFS 合同由夹具 conformance 与 adapter 包承担；Gitea testkit 由使用包的 `TestMain` 回收容器。手工只用来核对「进入的状态」七列，不代替测试。

```bash
export PATH="$HOME/.local/go/bin:$PATH"
# deployment.yaml 声明独立耐久来源、既有 core Repository 与 auth: local
# bootstrapPrincipal: admin；完整配置形状见仓库 README
go run ./cmd/kc -- deployment init --config deployment.yaml
go run ./cmd/kc -- serve --config deployment.yaml  # 另一终端

export KC_SERVER_URL=http://127.0.0.1:7380
go run ./cmd/kc -- login --mode local --as admin
kc() { go run ./cmd/kc -- "$@"; }

kc catalog repo attach --repo kr://acme/public/core
kc catalog show
kc writer commit --command-id u1 --repo kr://acme/public/core --dir ./drafts
kc dataset define --dataset agent --revision 1 \
  --source kr://acme/public/core
kc operations projection sync --repo kr://acme/public/core
kc read --dataset agent --object runbooks/oncall
kc resolve --dataset agent --object runbooks/oncall
kc operations access-spec describe --dataset agent                # 治理/运维诊断，不是消费命令
# 非法
go run ./cmd/kc -- catalog repo attach --repo kr://acme/catalog    # 必须失败
kc read --dataset agent --repo kr://acme/public/core --object runbooks/oncall
```

具体业务故事由走查叶夹具维护，不并进本目录。清河茶铺材料只在 `.data/scenes/.../named-repositories-created/` 中维护。
