# ACCEPT-HARNESS-01 · 插件验收规格在测试部署上可执行，先跑出红

## 目标

清河走查叶的 `accept-01` 到 `accept-13` 与 `repository-attached/_specs/liveaccess.yaml`，每条规格都有对应的
probe。通过一个独立的验收入口，在本机已部署的真实测试栈（lakeFS、Server、OpenSearch、`live-origin`）上
批量执行，得到逐条的红绿清单。实现开始之前，这些用例应当按预期是红的，而且红的原因是“能力还没有”，
不是用例、夹具或执行器本身有错。

## 决定

2026-10-15 用户确认：执行环境选 B，即验收只在真实测试部署上跑，走查分支不进 `make test` 的进程内执行器。

- A 要给走查分支另造“部署世界”：打开部署配置、托管池接 lakeFS 假实现、执行器以子进程起停接入方。这是绕开既有抽象的新特例。
- 清河 construct 里的 `kc invoke ... SELECT COUNT(*)` 需要真实 MySQL，即使走 A，也要拆出一部分只在部署上跑，验收会被分到两个环境里。
- 依赖时钟的规格只有 `accept-12-dataset-revisions.yaml`（时钟推进 2 小时、配额最小间隔 30s）。规格本身写的就是由部署 variant 提供测试时钟，B 下由测试部署配置提供即可。
- 结果：`make test` 的进程内执行器职责不变，仍只跑产品树协议用例，不承接清河验收。真实部署验收继续作为独立入口，与 `make deploy-local-scenes` 并列。

## 依据

- 场景树规则与执行方式：`.data/scenes/README.md`（“执行”一节说明 `goto.py --probe` 只跑单个正向走查，不是隔离批跑器）。
- 夹具与用例树：[acceptance/README.md](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_materials/acceptance/README.md)。
- [测试与验证](../../doc/engineering/testing.md)、[工程质量闭环](../../doc/engineering/quality.md)。

## 验收

1. 规格覆盖：以 `_specs/` 下的文件为准，每条规格（按规格 id）对应一个 probe，断言按规格的 `then` 写，不只断言命令成功。错误、拒绝、授权隔离类规格同样要有 probe。规格要求的命令形状以当时的 `--help` 为准；形状未定的规格先写到能红为止，并在交付里标出。
2. 验收入口：新增一个 make 目标（建议 `deploy-local-accept`），经 `scripts/deploy/deploy.sh local` 分派，与 `deploy-local-scenes`、`deploy-local-goto` 并列。入口只绑定已部署的 local 测试栈，不启动、重建或重置；栈缺失时给出明确诊断并以非零退出。
3. 结果分类：每条规格输出 `pass`、`fail` 或 `error` 三种结果之一，并附原因。`fail` 指断言不满足，即能力缺失；`error` 指夹具、部署或执行器出错。报告用机器可读格式，写到 `.validation/runs/` 下，沿用现有记录格式。本任务完成时 `error` 必须为 0。
4. 前态复用：每次运行时，清河叶的前态（含 `named-repositories-created` 的 construct）只构建一次，不对每条规格都从 `service-initialized` 重放。各 probe 之间靠命名隔离（独立仓名、Dataset 名、索引前缀），互不污染；规格要求同一世界连续操作的除外，写明理由。复用与隔离方式写进交付。
5. `live-origin`：由测试部署起停，两个实例，含 7402 端口与 `api.live-origin` 别名；`live-origin/test_origin.py` 通过。与 `repository-attached/_materials/accessor` 的分工二选一：写明各自用途（accessor 是产品树接入方的 Resource Access/Collector/Observer，`live-origin` 是验收用的外部取值源），或者合并为一个。结论写进 `acceptance/README.md`。
6. 时钟：`accept-12` 的时间类规格通过测试部署的可控时钟推进，不真实等待。当前产品还没有可控时钟，这些规格应得到 `fail`，归 `DATASET-REVISION-01`。本任务只在 probe 侧写好推进时钟的调用，并标为形状未定，不实现产品时钟。
7. 索引隔离：每次运行使用独立的索引与命名前缀。运行结束时（包括失败与中断）只清理本次前缀，不删共享索引，不提高分片上限；容量不足时入口给出明确诊断。本条覆盖部署验收路径；`make test` 共享实例的清理见下文“并入”一节（原 `DEBT-SHARED-SEARCH-CLEANUP-01`）。
8. 红绿清单写进本文件的“交付”：每条 `fail` 写明缺哪项能力、归哪个实现任务（如 `INDEX-DECLARATION-01`、`INDEX-SEARCH-SPACE-01`、`INDEX-USAGE-01`、`DATASET-REVISION-01`、`PUBLICATION-SYNC-01`）。归属已完成任务（如 `LIVEACCESS-READ-01`、`LIVEACCESS-DECLARATION-01`）的规格若仍是红，登记新的 `DEBT-` 任务，不并入本任务修。
9. 门禁：入口自身的逻辑（结果分类、前缀生成与清理、栈缺失诊断）要有不依赖部署的回归测试。场景树三项检查、场景目录守卫、`make check-docs`、`make quality` 通过，均无 skip。

## 执行要点

- 先读 `.data/scenes/README.md` 与 `acceptance/README.md`，再读 `_specs/accept-*.yaml` 与 `acceptance/variants/`。
- 从 `.data/scenes/goto.py` 出发：要么扩展出批跑模式，要么在 `.data/scenes/` 下新增批跑器并复用它的前态构建。`goto.py --probe` 现有的单 probe 语义保持不变。新增或改变的执行方式登记到 `.data/scenes/README.md` 的“执行”一节。
- probe 放在 `qinghe-object-published` 节点下，材料只通过 `$materials` 引用。不新建根目录，不另起数仓目录。
- `live-origin` 接进部署时，只改 `scripts/deploy/` 下的部署脚本与 compose，不改产品代码。
- 跑真实部署前先用 `make deploy-local-status` 确认栈在；不重置已有部署的权威数据。
- 已核实：`scripts/deploy/accessors.sh` 的夹具路径已指向 `source-repositories-configured/repository-attached/_materials/accessor`，原 `catalog-initialized/` 的问题已经不存在，只需复核。

## 前置

- `DEBT-SCENE-YAML-01`（已完成）。
- 本机已有 local 测试栈（`make deploy-local-up`）。栈不可用时，只交付入口、probe 与离线回归，并在交付里写明“未运行部署”，本任务保持 doing。
- 目录重组（`ARCH-LAYOUT-01`）排在插件重构之后，probe 届时随目录一起搬。

## 不做

- 不让走查分支进入 `make test` 或 `TestProductScenes`，不改进程内执行器的职责范围。
- 不实现任何插件、时钟、配额等产品能力；不为变绿放宽断言，不用 skip。
- 不清空共享 OpenSearch，不重置已有部署，不安装未经批准的依赖。
- 不提交。

## 进展

- 2026-10-15 执行器提速：有投影的前态冻结时，索引用 OpenSearch `_clone` 复制成写入封锁的冻结副本，每条 probe 和子状态从副本分叉，不再从根重放构建链。`TestMetricPermissionScenes` 从 56 秒降到 20 秒（`dataset-query-principals-granted` 从 32 秒降到 10 秒），结果不变，运行前后索引数相同。回归测试：`TestSceneProjectionNamespaceForkIsolatesFrozenMedia`（冻结副本不可写，两个分叉互不可见）。真实 Graveler 的 live DFS 仍按原样重放。这项提速属于进程内执行器，保留不回退。
- 2026-10-15 探查方案 A（让走查分支进进程内执行器，临时测试已删除）：
  - `kc create --name` 拒绝 `--home`，只能经 Server；服务端托管建仓要求 home 由部署配置打开（`ws.Deployment`，带 `managedStores`），而场景执行器的世界都是 InitHome 夹具，没有部署配置。
  - 清河叶 construct 里的 `kc invoke ... SELECT COUNT(*)` 需要真实 MySQL（`shop.py` 调 `mysql` 客户端），进程内无法提供；`kc access` 的 stats 部分在没有 MySQL 时可由 `access.py` 读 `source.walk.json` 返回。
  - 清河材料按红线只能放在走查分支的 `_materials/`，`$materials` 只解析到本节点，验收 probe 无法挪到产品树节点复用这些材料。
- 2026-10-15 决定选 B，理由见“决定”。

## 交付

### 实现范围（2026-10-15，本轮未完成）

- 目标是环境 B 的独立部署入口与逐规格报告；依据为《测试与验证》《工程质量闭环》及本任务引用的场景执行合同。未改设计篇，未把清河走查接入 `make test`。
- 新增 `.data/scenes/accept.py`、`accept_world.py`、`accept_test.py`、本节点 `_probes/acceptance/probes.json`，以及 `scripts/deploy/accept.sh`、`compose.accept.yaml`。只绑定已有测试栈；前态只构造一次，每条 probe 使用独立分支、Dataset、主体和取值描述身份，Schema 按运行前缀隔离；不清空外部源调用记录。
- 当前规格共 117 条，已登记 32 条有具体产品动作和字段断言的可执行 probe，85 条仍未接入。逐规格的规格路径、probe、产品归属与执行状态见 `.validation/runs/20261015T120400Z-accept-harness/probe-coverage-check.json`；`--check` 因覆盖未完整退出 1，不把缺 probe 当能力红。
- 结果区分产品断言 `fail` 与夹具/部署/执行器 `error`；共享前态失败另列 `prestate`，下游未执行规格保留 `error/not_started`。真实脚本 `__main__` 与导入模块共享异常身份，子进程回归证明产品失败不被错分为环境错误。
- 运行中的 Dataset、仓登记及搜索资源只按本次拥有的 ID 清理。当前部署的 Repository 投影物理名称由本次唯一 Repository ID 的 SHA-256 前缀确定；清理精确物理索引及共享 control 中的本仓文档，不删除共享 control 索引。新 Dataset 索引主线形状尚未定稿，后续必须同步核对其所有权命名；本轮没有真实清理验收。
- Makefile 与场景 README 等集成必经文件开工前已有未提交修改，未直接编辑；部署分派和说明的配套改动保留在同一待合并补丁中；集成补丁 `.validation/runs/20261015T120400Z-accept-harness/accept-integration.patch` 待用户授权追加。新增文件不等于 make 入口已经接入。
- 7 条补充草案均不计可执行：`supplement-history.json` 两条（公开 Object Diff 与非空受控 proof 前置缺口）、`supplement-proof-boundary.json` 五条（local 身份没有可转发认证证明、缺完整出站/日志观测或调度能力）；具体逐 then 核对留在同证据目录。未伪造接口或以空凭证证明隔离。

### 已验证范围

- 离线执行器 14 条、共享搜索辅助器 5 条、验收搜索清理 2 条，共 21 条通过且无 skip，见 `.validation/runs/20261015T120400Z-accept-harness/final-0.log`。覆盖失败/错误分类、真实脚本入口、共享前态阻断、未执行保留、路径隔离、null 声明解析、容量不足、初始化分片计数、SIGTERM 前后窗口、忽略 TERM 子进程有界结束及精确所有权清理。
- 场景 `--check-views` 与 `--check-states` 通过；`--check-product` 当前因 INDEX 在途改动的旧 Go 用例关联未更新及 `object#U15` 无 case/gap 而失败，见 `final-1.log`。未修改该线文件。定向离线测试不能替代完整架构组、`make test` 或部署验收；全量门禁由主线在源码稳定时统一运行。
- 只读真实栈预检报告 `.validation/runs/20261015T125526Z-8f9962049123/report.json` 与 `acceptance.json`：Server `/readyz` HTTP 503，117 条全部 `error/not_started`，pass=0、产品 fail=0。没有构造前态、写仓、起停服务或清理共享资源；运行源码 `sourceChanged=false`。因此未运行产品部署验收，本任务保持 doing。
- 红绿证据：`harness-red.log` → `harness-green.log`；`prestate-review-red.log` → 对应 green；`material-null-red.log` → `material-null-green.log`；`entry-regression-green.log`；共享搜索见协同债交付。

### 质量问题去向与后续

- `DEBT-ACCEPT-SPEC-TRUNCATION-01`：两处原文截断无法从 git 历史恢复；第三处未引号 ` #` 截断已有精确 quote 补丁，未直接改原在途规格。前两处需要权威原意或用户裁决。
- `DEBT-ACCEPT-ORIGIN-HTTP-CLOSE-01`：源自测错误响应未 close 的 ResourceWarning；原文件未改，精确补丁及副本 9 条无警告回归在 `origin-response-close.patch`、`origin-close-draft-green.log`。这不是仓内原文件已修复。
- 剩余前置包括健康的既有测试部署、适用的认证证明/部署 variants/可观测控制、INDEX 主线公开命令与时钟形状、集成补丁授权及全量门禁。实现部分完成与验收通过分开记录，不移 done。

## 本轮最终统一门禁（2026-10-15）

统一证据与命令见 门禁索引（`.validation/runs/20261015T120241Z-task-orchestration/final-gates.json`）。最新 `make quality`（含 check-docs）通过；完整架构 648 pass/1 fail/0 skip（术语，归 DEBT-TERMINOLOGY-TRIAL-01）；`make test` 18 pass/2 fail/0 skip（同一 Dataset 文本能力场景及父级，归 INDEX）；`make test-contracts` 的 component 为 2349 pass/67 fail/6 skip，boundary/e2e 未启动。上述四次均源码稳定；合同失败分组见 报告（`.validation/runs/20261015T135236Z-03378168af4a/findings.md`），分别归 INDEX、库存消费债及回环 HTTP 资源调查，不由本任务替其它线修复。

最终相关定向结果及真实部署边界分别保留在任务专项证据与门禁索引：定向通过不替代完整入口，现有 local 栈 readyz 503，117 项真实验收均 error/not_started。全量门禁尚未全绿，依本轮约定保留 doing；未提交或安装依赖，未降低检查阈值或重置已有环境。

## 2026-10-16 追加交付

### 实现

- 已获准在分派范围保留原在途修改追加，落地 Makefile 的 `deploy-local-accept`、`scripts/deploy/deploy.sh local accept` 分派、local-only `compose.accept.yaml` 接入及部署/场景说明；原补丁不再是集成阻塞。入口与 `make test` 保持独立，只绑定已有测试栈。
- 在 acceptance README 明确墙外 accessor 与 live-origin 分工；将逐规格验收说明放入场景 README 的执行一节。
- 新增真实 Make/分派的离线入口回归，缺栈时仍产生完整逐规格 `error/not_started` 报告，dev 拒绝执行，均不调用 Docker。

### 验证与剩余

- 本轮 Python 定向共 35 条（执行器 15、入口与错误响应关闭 3、共享搜索 6、验收清理 2、原源服务 9）全绿无 skip；Go namespace 的 3 条定向回归全绿。红证据与对应 green/final 日志见上述目录。源 9 条以 `ResourceWarning` 为错误执行。
- 场景 views/product/states 三项检查通过；Compose 合并配置只读检查通过；修改过的说明已重读，未改产品设计篇。
- 标准真实入口证据 `.validation/runs/20261016T012400Z-5999ed2e51b2/`：Server readyz 仍 HTTP 503；117 项全为 error/not_started，events=0，cleanup 为空，`sourceChanged=false`。仅预检，没有构造前态或任何部署写入。32 条可执行/85 条缺口仍按逐 ID 库存保留。
- 证据目录 `.validation/runs/20261016T012005Z-accept-harness`；源码写入完成后交由主线在稳定窗口统一运行全量门禁，本线没有运行完整架构组、`make quality` 或 `make test`，没有移 done。

## 并入（2026-10-16）

以下各条债与本任务共用验收执行器、真实测试部署和共享 OpenSearch，同一时间只能一条线使用，并入本任务。原债的交付与证据保留在 `done/` 的各条，这里只写剩余部分。

### `DEBT-LIVE-MAINTENANCE-DIGEST-01`：外部声明的维护条件摘要

- 位置与信号：条件更新只比较 null 正文的内容摘要，A→B 声明变更后同一旧 `--if-digest` 仍成功；原部署红见 `.validation/runs/20261016-environment-recovery/acceptances/20261016T030949Z-56ffe330bfe0/acceptance.json`。
- 处置：独立维护依据与内容摘要分开，由 Resolve 与条件 PUT/REMOVE 共同使用；包含复合对象删除，保持静态、内容 READ、引用和索引摘要合同。
- 剩余验收：专项红绿、受影响完整合同与修复后原 probe 实际部署。登记后立即归并，共用当前真实部署，不将关闭债条目写成产品已通过。

### `DEBT-DEPLOY-LAKEFS-MEMORY-01`：lakeFS 实测运行时软预算与异常恢复

- 位置与信号：local lakeFS 历史 OOM 退出后 restart=no；本轮真实前态的 Go managed memory 达 1,721,494,424 B，软上限仍为 MaxInt64，共享 VM 的 swap 接近耗尽。
- 已实现：设置 `GOMEMLIMIT=1536MiB` 和 local `on-failure:3`；实测保留空闲堆 449,757,184 B 高于收至软预算所需 110,881,688 B，在用内存及非堆运行时开销之上仍留约 323MiB。未改普通 GC、缓存、权威卷、kernel/VM/swap 限制；dev 保留原重启策略。
- 已验证：两个配置合同均先红后绿；真实 Compose 只读解析验证 local/dev 软预算、bootstrap 一次性语义与无新增硬限。依据与证据 `.validation/runs/20261016-lakefs-oom-recovery/`。
- 剩余验收：将同一配置显式应用已有 lakeFS 后实际复跑，采集生效预算、内存/GC/VM 余量和 OOM 状态；最终冻结门禁。不把软预算当作 RSS 硬保证。

### `DEBT-LIVEACCESS-ADMISSION-SCOPE-01`：运行中的访问插件采用当前已接纳来源

- 位置与信号：`liveaccess/plugin/entry.go` 固定保存启动时的仓列表；真实部署启动后合法 create/attach 的来源被 lookup 与 invoke 拒绝，错误为 `Repository is outside this plugin instance`。
- 已实现：宿主只为访问插件注入当前 `Store.Get` 成员判定，Entry 每次外呼前采用当前结果；未提供端口的嵌入仍保留固定范围。默认与自定义 Entry 同路，索引装配不变，不请求时重开运行时或扩张 HTTP 形状。
- 已验证：真实托管 create/attach 装配回归先红后绿；默认 HTTP Entry、自定义 Entry、移出/未接纳拒绝及并发 registry 增删回归通过，三项 race 通过，访问插件/服务及共同采用包回归通过。证据 `.validation/runs/20261016-liveaccess-current-scope/`。
- 剩余验收：真实部署重新执行且无需每次 create 后重启；最终冻结快照的文档、质量及完整受影响合同。原债登记后立即归并，不另开环境线。

### `DEBT-MANAGED-ACTIONS-RECOVERY-01`：历史托管动作回执恢复

- 位置与信号：`home/managed_ledger.go` 在真实部署进程替换后拒绝历史 READY 分配的对象动作拼写；只读备份证明原请求摘要、绑定、分配身份和初始版本有效。
- 已实现：仅 READY 回执的耐久策略校验接受固定历史动作对应的当前窄白名单，原 request/digest/actions/字节不变；新配置仍拒绝历史写法，READY 重放不创建来源或重新发权。含历史动作的未完成分配拒绝恢复，须由原版本完成或显式处置，不传递失效动作后记成完成。
- 剩余验收：真实部署恢复及统一快照门禁；红绿与只读原备份解码证据见 `.validation/runs/20261016-accept-runtime-repair/`。原债登记后立即归并，未另开并行环境线。

### `DEBT-DESCRIPTOR-READ-RESOLUTION-01`：取值描述解析的只读能力与来源故障

- 位置与信号：`object/writer/value_source.go` 对纯依赖读取要求 TreeStore，导致 runtime System 只读包装阻断业务发布；同一路径还跳过远仓 HEAD 故障或空版本，可能把多持有错误判断为唯一。
- 已实现：纯读路径改用 TreeReader，写入端 TreeStore 不变；未知 HEAD/正文失败关闭，locator 已指向但正文缺失不再吞成不存在，正常 locator 未命中及多仓歧义保持原语义。完整 System 出版物、真实只读包装、跨仓描述、预览/提交与来源故障及重复持有均有回归。
- 剩余验收：真实部署后续前态及统一快照门禁；两阶段红绿和受影响回归见 `.validation/runs/20261016-accept-runtime-repair/`。原债登记后立即归并，不另开并行修改线。

### `DEBT-SHARED-SEARCH-CLEANUP-01`：共享搜索实例容量与夹具清理

- 位置：`scripts/testsuite.sh` 的共享 OpenSearch 复用入口、`scripts/opensearch_fixture.py`、`cli/scene_projection_namespace_test.go`。
- 已完成：容量预检与本次前缀精确清理已落入 `scripts/testsuite.sh`；Go 所有权、clone 失败与包装脚本回归先红后绿（`.validation/runs/20261016T012005Z-accept-harness`）。
- 剩余验收：重复运行场景后，本次用例拥有的索引及分片恢复到运行前水平；失败与中断清理不删除其它命名空间。当前共享实例只读容量 999/1000，余 1，低于入口所需 8，受此阻断。
- 不做：不清空共享测试集群，不改变部署资源限额。

### `DEBT-ACCEPT-ORIGIN-HTTP-CLOSE-01`：外部取值源自测关闭 HTTP 错误响应

- 位置：`qinghe-object-published/_materials/acceptance/live-origin/test_origin.py` 的 `LiveOriginTest.call`。
- 已完成：错误响应放入上下文管理；原 9 条回归在 `-W error::ResourceWarning` 下通过，新增入口/close 回归 3 条通过（同上证据目录）。
- 剩余验收：随统一门禁 `make check-docs`、`make quality` 全绿。

### `DEBT-ACCEPT-SPEC-TRUNCATION-01`：三条被截断的外部取值验收规格

- 位置：`qinghe-object-published/_specs/accept-03-live-read.yaml` 的 `accept-live-member-failure-isolated`、`accept-live-observation-beside-value`、`accept-live-precise-range`。
- 已完成：`accept-live-precise-range` 的 then 加 YAML 引号，保住 ` #/runtime/row-count；第三次测试源没有新调用。`，执行器回归先红后绿。
- 剩余验收：前两条原始行缺后文（“测试源把 table/shop.orders”“observations 恰有两项，地址分别是 table/shop.orders”），git 历史无法恢复，需规格原意的权威来源或用户裁决；恢复后三条部署 probe 对完整 then 逐项断言；同类截断信号有自动回归保护。
- 不做：不根据当前产品输出反写期望，不删除原规格，不以占位 fail 冒充能力红。

## 2026-10-16 并行补充

### 实现范围

- 只读诊断现有 local 栈：lakeFS `OOMKilled=true`、退出 137，导致登记权威不可用；共享 OpenSearch 是 999 主分片加 1 配置副本，按合同已占 1000/1000，并非尚余 1。`kc-proj-*` 占 992；逐前缀和恢复所需授权见 诊断（`.validation/runs/20261016-parallel-accept-diagnostics/findings.md`）。没有起停、重建、重置或清理共享环境。
- 新接能力清单、Trial/Writer 一致性及取值 Schema 旧路径退役三个 probe；已有 35/117 条、剩余 82 条，逐 ID 库存保留在 增量证据（`.validation/runs/20261016-l3-probes/index.md`）。索引声明任务所需 5 条现在均有 probe，仍未执行部署验收。
- 试运行原材料上传、同分支固定判断、完整错误包装比较，以及 HEAD/owned generation/owned control 写入版本不变均有断言。真实材料合同修正原两个 probe 的错误码漂移；结构执行器拒绝具名父测试下的子 skip/fail，未改或削弱源守卫。

### 已验证与剩余

- 开发期离线 5 条新增回归及真实材料 Trial/Writer 合同先红后绿；最终快照、受影响回归及文档/质量门禁以证据目录 `final-validation.json` 为准，不把开发期定向绿写成完整验收。
- 本轮 HTTP 资源结论见 L0 门禁索引：历史 31 个连接失败叶本轮通过，未复现地址耗尽，因此没有依据修改 `scripts/testsuite.sh` 并发或系统限制。
- 部署恢复、共享容量、主线公开合同和隔离变体仍阻断相应验收；两条规格原文仍待裁决。本任务保持 doing，未提交、未安装依赖、未用 skip。

## 2026-10-16 MVP 优先级复核

- 现有 local 栈已由前轮恢复，无需重复起停：Server `/readyz` 为 `ready/all`，lakeFS 健康，实际容器已生效 `GOMEMLIMIT=1536MiB` 与 `on-failure:3`，`OOMKilled=false`。本轮只读指标中的 Go 管理内存为 1,530,063,736 B，RSS 为 1,437,605,888 B；这证明当前真实前态后的预算生效，不替代最终源码部署复跑或长期容量资格。
- 共享 OpenSearch 的旧满额阻塞已经解除。前轮按已核实测试归属清单、control 条件版本和索引 UUID 精确删除 238 个测试残留索引，保留未确权资源和共享 control 索引，见 `.validation/runs/20261016-environment-recovery/verified-residue-cleanup.json`。本轮核验为 876/1000 已用分片、124 空闲，未改变分片上限。
- 在该共享实例连续执行两次成功、一次失败（退出 73）、一次 SIGTERM（退出 143）的拥有前缀夹具，均恢复原 763 个索引及 876 个分片；各次前后全部其它索引 UUID 相同。只创建和删除本次 `kc-test-<随机值>-` 完整前缀资源，不清理其它残留。证据 `.validation/runs/20261016-mvp-environment-current/environment-proof.json` 与同目录实际内存采样。
- 本轮未修改产品行为，未安装依赖、提交、重置权威或起停容器。历史托管回执与只读取值描述的修复沿用已有红绿回归；最终源码部署复跑及文档、质量、完整受影响合同由主线在稳定窗口统一执行，任务仍保留 doing。规格覆盖和两条原文截断的缺口保持原记录。

- 执行器、原材料与 Oracle 校正的实现、各批红绿回归及源码 SHA-256 见 `.validation/runs/20261016-accept-runtime-repair/index.json`；同仓 Schema 切片、显式材料路径、公开诊断与失败位置修复均已定向验证，外部索引增量的范围另列；维护声明 CAS 的真实红与修复单独归档，固定版本/凭证新规格仅保留未登记草案；最终源码部署复跑和快照门禁仍待同次归档，不据此收口。

- 并入的 `DEBT-SHARED-SEARCH-CLEANUP-01` 补充信号：全局 `preflight` 对可能只读的原 35 条也固定预留 8 分片；选择性容量与“本次无写”双重证明方案仅归档 `.validation/runs/20261016-accept-runtime-repair/search-capacity-readonly-proposal.json`，尚未实现或验收。未知/索引发布用例仍须原容量，现轮优先正确资源配置的独占门禁；128MiB heap 的 429 单列为环境污染。

### 本轮资源合同补齐（2026-10-16 MVP 并行修复）

- 同类容量信号再次出现：普通 Home/CLI Go 夹具只收到 `KC_TEST_OPENSEARCH_RUN_PREFIX`，不会采用场景 transport 的物理 namespace；本轮共享节点再次达到 1000/1000。原四种退出路径的前缀夹具证明仅覆盖采用该前缀的子进程，不能外推为普通 Go 夹具已经具备清理合同。
- 已升级自动守卫：`scripts/opensearch_fixture.py` 在 HTTP 与子进程之前，只接纳仓内正式 `scripts/testsuite.sh lakefs` 或 `./cli` 中有限且完整锚定的 SceneNamespace 回归选择。普通 Go 夹具、宽正则、未锚定选择、第二个 selector 覆盖、shell 包裹与其它测试包均拒绝；普通夹具改用本轮专属 OpenSearch。正式 `make test` 的 lakeFS 自调用入口保留。两阶段红绿、8 项离线回归见 `wrapper-scope-red.log`、`wrapper-selector-anchor-red.log` 与 `wrapper-scope-green.log`。
- 部署验收清理显式接入本轮 Dataset 集合，识别来源的存储摘要限定身份与 `dataset-index/<本轮已登记 Dataset>/<完整 scope 摘要>` 的静态/观察 corpus。控制记录的 SHA-256、物理 namespace、CAS 与 generation UUID 均核对后，先条件领取控制记录，再逐个核对 UUID 并精确删除 generation；不删除共享控制索引，不领取未登记 Dataset。只读资源快照使用同一集合，因此 Trial 不变断言覆盖 corpus。伪造 ID、控制变更、UUID 替换和 CAS 失败均保留 generation。6 项离线清理回归与 38 项执行器回归通过。
- 真实资源清理合同在本轮专属节点验证：仅删除 3 个自建 corpus/source generation；同前缀但未登记的 Dataset 保留，原有 153 个索引 UUID 不变，全部证明用资源随后按确权名字回收。证据 `accept-resource-live-proof.json`。
- 对共享节点另做只读归属清单：已知本轮测试 Repository ID、全部持久对象的 `cN` lakeFSFake 来源版本、control 摘要与精确 generation 坐标共同证明 110 个测试 generation；创建时间仅为辅证。执行前再次核对完整来源证明、控制 CAS 和 UUID，精确删除 110 个 generation 及 103 条控制文档，将分片恢复至 890/1000；其余 776 个索引 UUID 及共享控制索引均保留，未确权空 generation、System 与真实来源未删除。清单与执行证据 `mvp-fixture-residue-manifest.json`、`mvp-fixture-residue-cleanup.json`。
- 上述证据均在 `.validation/runs/20261016-mvp-environment-current/`。本线没有启动或替换服务、安装依赖、提交或重置权威；源码部署、统一门禁和未覆盖规格由根主线在最终稳定源码统一验收，本任务仍为 doing。

- 2026-10-16 冻结原 35 条的只读搜索变体实跑为 27 pass / 8 fail / 0 error，恢复原部署端点后的代理末检查仍为 denied=0、upstreamErrors=0；随后修正五条 Dataset 夹具的 Schema 来源/交付坐标，正式解析、发布及文件回读回归先红后绿，38 项执行器回归通过；该增量尚待真实部署复跑与统一门禁，见 `.validation/runs/20261016-accept-runtime-repair/index.json` 的 `readonlyOriginal35Actual` 与 `schemaMountCoordinateCorrection`。

### `DEBT-TEST-OPENSEARCH-LEASE-01`：自动搜索夹具跨进程资源协调

- 原债位置：`tasks/done/DEBT-TEST-OPENSEARCH-LEASE-01.md`；自动临时搜索入口各自创建 512MiB heap 实例，实测约 900MiB 以上，独立合同并行启动导致全局 VM 余量突降并中断本轮门禁。证据 `.validation/runs/20261016-owned-final-opensearch/`。
- 处置范围：同 Docker daemon 的跨进程租约、真实 VM 启动预算及自动 adapter 共用实例；租约覆盖测试和清理，显式 endpoint 与普通 Go 测试包并发保持原合同，不操作既有其它实例。
- 已验证：最终冻结源 `6118fb4c…` 的 10 项离线生命周期回归、既有 15 项回归、build、文档、质量与定向 boundary 全绿无 skip；不确定创建结果持续阻断后续实例，见 `.validation/runs/20261016-auto-opensearch-lease/index.md`。剩余为最终合并源统一门禁与真实部署验收，本版未启动真实 Java 实例。
- 并入 `DEBT-TEST-OPENSEARCH-TIMING-01`（2026-10-16）：客户端打包后的完整合同中，离线
  `test_sigterm_interrupts_a_waiter_without_starting_another_instance` 首个 fixture 的 8 秒就绪等待失败，
  后续 3 秒子进程清理也超时；首次失败（`.validation/client-package-contracts.log`）与
  同源孤立通过（`.validation/client-package-isolated-lifecycle.log`）分别保留。根因未确定，
  不把复测通过视为修复，也不先归因为并行负载；剩余为就绪/清理阻塞归因和完整 boundary 的稳定执行，
  保留原 SIGTERM、互斥、所有权清理与退出码断言。本信号未改自动夹具实现或原测试。

## MVP 验收中断恢复（2026-10-16，待统一验证）

- 已开始的 probe 中断后保留实际开始时间并标记 interrupted；未调度及后续规格为 not_started，仍沿 pass/fail/error 分类。取消在动作调用前检查，不再把可能已写入的 probe 误记为从未开始。
- 首个信号落在清理或报告阶段时记录取消并完成既有有界的一次清理，不重试整次清理；普通清理错误仍保存。报告临时写入后原子替换，信号处理覆盖报告发布；已完成结果不会因清理窗口首个 SIGTERM 丢失。
- 新增 `.data/scenes/accept_interruption_test.py` 的 8 项实际子进程信号回归并登记离线入口，均不访问部署。没有改 World、搜索夹具、规格预期、部署或共享搜索；按用户要求在全部实现完成后统一验证。

## 本轮 MVP 增量统一验证（2026-10-16）

8 项真实子进程信号回归、39 项执行器回归、10 项搜索辅助回归、3 项正式入口回归和三项场景结构检查已通过；未访问部署。

统一冻结源 `109295d7d0ed28077b13a358419c6a66a434f8e4737bcb018af07ad1b6feb81a`：144 个 Go 顶层测试（含子项 299 条通过事件）、60 项 Python 测试、场景 views/states/product 检查及 `make quality`（含 `make check-docs`）均通过、无 skip，各报告 `sourceChanged=false`。源码、增量 diff 与逐项证据见本轮证据索引（`.validation/runs/20261016-mvp-isolated-fixes/summary.json`）。旧实现的失败对照在全部改动后的验证阶段执行，不冒充改前先红。

本节仅交付上述独立 MVP 修复；未运行本轮源码的完整 `make test`、`make test-contracts` 或真实部署验收。任务原有全目标和缺口继续保留，目录保持 doing；未提交、安装依赖或操作其它任务的环境。追加本节后单独复查文档链接，已验证实现、测试与 README 内容保持冻结源一致。

## 真实场景前态复用（2026-10-16）

- 位置与信号：`TestLiveLakeFSSceneDFS` 原为每个节点和 probe 重放祖先链；上轮 16 个节点、63 条 probe 共构造 271 段、初始化 79 次，DFS 本身耗时 600.23 秒。该问题在本轮直接修复并增加自动回归，不另开重复任务。
- 实现：真实 lakeFS 夹具只对本次创建且存储命名空间一致的仓保存检查点；顺序恢复全部分支头、仓属性及本地配置，清除新增分支和未提交写入，保留不可变提交与 Dataset pin。节点只构建一次；索引仍从冻结副本隔离，HTTP 与会话重新准备。真实 DFS 初始化次数不为 1 时直接失败。
- 隔离回归覆盖先修改再观察、反向顺序、单独执行和子状态；检查原 HEAD、历史、权限、仓属性、工作分支和暂存内容。恢复拒绝未拥有的来源、存储身份变化和失败的分支清单，不把失败当空状态。
- 实跑暴露了场景中的退役本机命令误走测试专用入口；改由公开 CLI 分派，并补充默认目录不被写入的回归。试跑写入仓库根旧 `.kc` 的唯一测试授权已精确撤销，审计保留；原文件缺失与空规则无法区分，因此保留空规则文件，不声称字节级恢复。修正与前后摘要保存在本轮证据中。
- 验证：同一场景树 16 个节点、63 条 probe 保留，DFS 97.33 秒，构造 16 段、初始化 1 次；17 个通过事件、0 失败，原有两条独立走查仍由 `goto.py` 执行。最终受影响场景及检查点回归 28 个通过事件、无 skip；夹具及 CLI 边界 5 项通过，质量与文档门禁通过。部署后仅把物理默认分支的字面值改为从 `snapshot.DefaultRef` 推导，行为值相同，随后定向 4 项及质量门禁再次通过。
- 环境：使用调用拥有的专属 OpenSearch 与已有真实 lakeFS/MinIO/PostgreSQL；923 个原有仓的命名空间清单前后相同，无新增或删除仓残留，专属搜索和测试容器已移除。本轮没有降低容量上限、删减断言、增加 skip、安装依赖或提交。
- 实现与逐入口证据见本轮记录（`.validation/runs/20261016-scene-checkpoints/summary.json`）。计时来自不同候选及编译器的两轮运行，作为实际等待时间记录，不作为严格同工具链压测。该交付只覆盖执行器优化，本任务其它规格缺口和验收要求仍保留。
