# CLEANUP-03 · 退役 DSH 宿主集成（2026-10-10）

- [x] 本轮完成：按用户明确裁决彻底移除仓内 DSH 插件及专属运行、构建、CI、验收和 CLI 私有任务上下文接缝；受影响测试与契约全绿且无 skip，保留其它在途改动，未提交。
- Goal / owner：撤掉已不再维护的宿主集成，所有当前入口无需插件目录或依赖即可装配。交互与入口归 [CLI 交互](../../doc/design/affordance.md)，独立客户端与认证边界归 [服务边界](../../doc/design/service.md)，验证分母与证据归 [验证体系](../../doc/engineering/test-catalog.md) / [MVP 验收](../../doc/engineering/testing.md)，能力区域归 [能力边界组织](../../doc/architecture.md)，过程归 [工程质量闭环](../../doc/engineering/quality.md)。
- Non-Goals：不退役公开 kc / typed Client / Server / Dataset File Gateway / kcfs，不改知识协议、授权、固定版本与 Writer 语义，不裁决 webui vendor、不删耐久状态或历史原始验收资料、不安装依赖、不运行真实部署验收，不代验收 OBJECT-INTERFACES-01 / DATASET-04。
- 不变量 / 接口：沿用 API-01（共享应用执行器）、A-01（依赖方向）、I-01 / V-01（身份与固定依据）；当前形状由公开 Go API、包 README 与 Conformance 拥有。CLI 按正式参数、环境及客户端会话取坐标，不再读取插件生成的 tasks/projects context.json；通用环境默认和服务会话隔离仍保留。消费授权沿 [权限体系](../../doc/design/permissions.md) 的 AUTH-01 / AUTH-02 分层与现行 Conformance，Dataset 消费和直接仓读分别求值，不改授权语义。
- 选定 / 否决：删除完整插件源码、锁文件、打包与本地生成缓存，清理专属命令和文档要求；将插件消费者测试中的通用 Server DTO / pin / 固定读风险复核并保留在核心测试。否决只删目录留下断链、伪造 Agent 验收完成、删除或削弱核心协议/架构断言、skip 换绿、删除外部宿主安装或其它工作区。
- 验证计划：先证明现入口仍依赖插件、私有上下文仍能干预正式 CLI，再删除；补核心范围回归与插件依赖防复发检查，运行受影响测试、场景与库存合同、Server Node 测试、`make check-docs`、`make quality`；真实 Agent 运行时退役后的通用验收缺口登记为债。
- 实现范围：完整删除插件目录（43 个源文件及其本地构建/依赖缓存），源码先按轮初字节备份到仓外临时目录；移除 CLI 私有 tasks/projects 读取接缝和专属测试，保留正式环境、saved Catalog/session 与 Server 必需边界，未删除用户 Home 中的旧上下文数据。CLEANUP-01 的插件验证记录是历史，不再是当前入口要求。
- 构建与验证接线：插件/付费 Agent 目标及 CI/npm 依赖退役，核心 Server Node 测试迁为 `test-server-web` 并继续进入 CI / test-all；Gitea 入口保留纯 Go consumer 与 Catalog discovery 合同及严格 live 要求。FUSE 原插件分支改为原生 daemon-mount/stop，ready manifest、固定 pin、EROFS、停止挂载/日志清理断言保留；隔离执行生产 shell 块的回归不计作真实 FUSE 验收。
- 风险迁移：消费者合同改为无宿主的真实 typed Client/HTTP 测试，保留库存与 Repository/Dataset 来源、named/selected pin、默认 ref、HEAD 前移后的固定 commit/body；增加异主体与当前 Dataset 消费撤权拒绝。Dataset `file.read` 与直接仓读分开求值，Client README 的歧义说明按 owner/现有合同收口，未改协议授权。
- 库存与防回潮：从现有场景树的已声明 feature 提取 Agent brief，明确声明库存和未评估状态；原 DSL、Then Oracle 与 metric 材料守卫保留，移除宿主 manifest/Python 硬依赖。既有 surface 守卫拒绝插件目录/接线/私有 context 回潮，通用模型工具禁令保留；修复扫描缺路径的错误被当成无匹配放行的问题。独立复核另发现仅恢复带退役标签的测试会漏放，已先红后绿补反例，并只对插件接线检查纳入测试文件。历史迁移记录明确排除，扫描错误仍失败关闭。
- 红后绿与增量审查：私有上下文干预正式 Server 坐标的 4 个反例先红；插件缺失时旧 Docker wrapper 与 Gitea 检查仍被 npm/Node 阻断，旧守卫也漏掉无目录扫描错误和工具注册反例。新增反例现已绿；复核发现新 CLI 回归的阻塞 channel 可能挂起，已改为重复请求明确失败，定向 race 通过。显式 Catalog discovery 标签编译暴露存量未使用 import，已最小删除该 import，原合同断言保留。Client Gateway 与 MVP V7 的旧逐仓授权措辞按现有 Dataset 受限消费合同纠正，无授权行为变化。
- 最终同源证据：以下六次运行使用同一源码指纹 `ee9efa7c2feac5f2c41b32fe0e8084b07f14d8d16fa0b9ff645e0f9bd2fcf2aa`，均退出码 0、`sourceChanged=false`、fail / skip 为 0；计数是含子测试的终态事件，不是声明库存或完整项目通过率。

| run / scope | 正式范围 | 实际结果 |
|---|---|---|
| `20261010T095444Z-30568867218e` / `cleanup3:final-maintenance` | `make check-surface check-validation test-boundary`；场景树 views / product / states 分别检查 | 77 个 Go 通过事件、3 个包；Python 6 + 6 项通过；库存及三项场景静态合同通过，未评估模型执行 |
| `20261010T095444Z-a5c07eb9b52f` / `cleanup3:final-quality` | `make quality`（含 `make check-docs`） | 文档图 29 篇 / 110 条关系通过；格式、tidy、vet、staticcheck、复杂度、体积、克隆与耦合门禁通过，传播率 33.23% 未增 |
| `20261010T095444Z-0602e20fc84f` / `cleanup3:final-compile-race` | `make typecheck`、显式 `catalog_discovery_contract` 标签空选编译、私有上下文隔离 race | 全包与标签编译通过，未计为 live 运行；race 7 个通过事件（根测试及 6 个子例） |
| `20261010T095444Z-18de7a2e9be0` / `cleanup3:final-client` | 显式选择的消费、远程/会话、固定范围/授权、CLI surface/覆盖与场景材料回归 | 140 个 Go 通过事件、1 个包；实际 typed Client/HTTP Server 消费合同通过，默认 lakeFS 进程内夹具 |
| `20261010T095444Z-6e4fdf3389bf` / `cleanup3:final-server-web` | `make test-server-web` | Node 14 项通过，无 skip，使用已有 Node 24；无依赖安装 |
| `20261010T095444Z-876f6cca0363` / `cleanup3:final-scenes` | `make test` | 普通与索引场景 21 个 Go 通过事件、1 个包；lakeFS 进程内夹具与真实 OpenSearch，非真实 lakeFS 部署 |

- 过程失败保留：`20261010T094817Z-4755af9d2fb2` 与 `20261010T094951Z-9cc7992f023a` 因调用场景树的参数组合不合法整体失败（此前门禁断言已绿），不计通过；最后按工具公开合同逐项执行到绿。`20261010T094854Z-59721dad5a17` 因显式标签文件的未使用 import 编译失败，不计通过；最小修复后上述编译范围已绿。所有原始 report / log 保留。
- 最终增量复核：与轮初源文件备份相比，29 个已有文件变化、46 个源文件删除（其中插件 43 个、专属 CLI 接缝/旧桥测试 3 个）、2 个新核心回归文件；其余 1,788 个已有文件逐字节保留，无文件模式变化，5 个原有删除未复活，Git revision 未变。`docs/graph/` 与 DATASET-04 任务块逐字节保留；OBJECT-INTERFACES-01 的并行任务验证记录保留，未回退、未代其标完成。协议与授权生产实现未变，独立复核未发现增量阻断项；仅回写本节记录与状态，随后再跑文档和质量门禁。
- 验收边界：本轮结果只覆盖上述选择；未跑完整旧混合套件、真实 lakeFS/Gitea、真实 Linux/FUSE 或模型任务验收。现有严格外部入口和风险继续保留，不以删除插件或静态 brief 库存宣称它们通过。仅启动既有入口的 OpenSearch 验证夹具，结束后清理本轮创建的容器；其它任务的服务与用户 Home 原始数据未改。

| 位置 | 信号 | 判定 | 处置 | 触发条件 |
|---|---|---|---|---|
| MVP C8 / VALUE-01、场景 Agent briefs | 专属模型运行器退役，通用概念/入口/恢复及真实任务效果目标仍在 | 协议 Oracle 或库存声明不能证明真实模型任务通过 | 保留为未执行缺口，无替代 runner；未来按角色/任务分母、回答/trace 与独立 oracle 验证 | 恢复模型验收或交付任务效果结论 |
| MVP V9 / V11 / V14 | 原宿主侧栏/开关/更新检查与上下文生命周期已退役 | 核心固定依据与当前授权已有合同，不能代证新的宿主人用体验 | 保留原风险与退役边界，不计当前宿主执行分母或新宿主通过 | 新宿主集成或重做发现/采用交互 |
| Linux/FUSE 与 live Gitea 资格 | 验收入口去插件依赖；本轮隔离回归/默认 lakeFS HTTP 夹具可执行 | 不等于真实设备、外部 provider 或部署通过 | 保留严格独立入口，缺环境失败，不新增 skip | 对应部署资格验收 |
| `cli/remote_test.go:156` | 现有 grant 默认范围隔离测试仍设置 `KC_WORKSPACE` 并检查 `workspace` | 存量夹具未钉现行 `KC_DATASET` / `dataset`，通过不能证明对应风险；本轮逻辑及该文件未改 | 登记测试债，迁移当前变量/DTO 反例并保留拒绝继承风险，当前不追存量 | 下次修改客户端环境默认或整理该测试 |
