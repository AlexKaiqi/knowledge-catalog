# SNAPSHOT-CLI-01 · Snapshot CLI 文件分组与帮助闭环（2026-10-13）

- [x] 本轮完成：仅整理 Snapshot 相关 CLI 适配文件，修正描述与帮助导航，必要回归、受影响测试、`make check-docs`、`make quality` 全绿；命令、单包与业务行为保持原样。
- Goal / owner：保持现有产品调用形状和行为，收拢仓/库存、Dataset、显式授权与文件交付的 CLI 适配；交互与输入/输出保证归 `doc/design/affordance.md`，文件范围/版本交付归 `dataset.md`，服务与原生资源接入归 `service.md`，授权归 `permissions.md`。owner 和直接关系已按 `docs/graph/` 核对。
- Non-Goals：不新增命令或业务入口、不拆 Go 包、不扩公开接口、不改变发布/授权/版本/文件交付行为、不迁移对象层、不新增标签或注册框架、不安装依赖、不启动真实部署验收、不提交；保留本轮开始前全部工作树修改。
- 不变量：保留 V-01、KS-01/KS-02、AUTH-01～03、API-01 与 A-01/L-01；创建/连接、登记、Dataset 发布和发权仍分别成立；一次消费保持既定范围和版本，CLI 与 HTTP 的注册权威不合并。
- 选定：同一个 `cli` 包内按能力移动既有函数；现有命令闭集、分派、身份上下文及帮助渲染保留。Snapshot 叶子说明集中在私有帮助函数并静态合并，现有帮助组补可达路径；否决新子包、新 CommandSpec 框架、Discover 执行入口、原生 API 机械命令化及随整理修存量行为。
- 接口：`cli/surface.go` 保持公开路径/动作闭集，既有 `client.SnapshotService` / AdminService 和 `snapshot/dataset` 配方合同继续承接调用；文件移动不改函数签名或请求构造。help 的同源系统入口出版物按现有生成器刷新，仅同步描述。
- 验证：先补帮助导航/发布效果/输入规则与本机预处理的失败证据，再整理文件及帮助；运行 Snapshot CLI/Client/服务相关回归、公开命令与架构合同、文档图和工程质量门。定向范围不替代真实部署验收。
- 质量闭环：本轮帮助同源、目录可达性与文件移动的增量信号当场修或升级守卫。既有缺入口（Dataset 详情/历史与按需文件读取、仓属性管理、规则原子更新）保持后续行为任务；其它层的已登记问题不在本批修。未通过所需检查前不勾选。
- 实现范围：仓/库存/登记历史收拢到 `remote_repository.go`，Dataset 调用与配方输入分别归 `remote_dataset.go` / `dataset_recipe.go`，本机 overlay 归 `dataset_overlay.go`，显式授权归 `remote_access.go`；保留现有 connection、clone 与共享分派。`help_snapshot.go` 集中 12 个 Snapshot 叶子，总览可进入既有 composition / dataset / grant / registry 组；更正发布固定版、单仓选择、根映射回退、可选配方写回及退役保留记录。README / SURFACE 同步文件组织，现有生成器仅刷新系统入口出版物中的 3 条 Snapshot 描述。
- 范围说明（2026-10-13 纠偏）：此前同期对 kcfs 的帮助澄清及回归属于独立宿主客户端的帮助维护，不列为 Snapshot 能力交付；已有运行记录保留为事实，不由测试覆盖推导模块归属。后续 Snapshot 批次不整理或迁移 kcfs，代码保持在 Snapshot 目录之外。独立文档债：`datasetfs/README.md` 仍引用已退役 catalog / 旧授权归属且遗漏 Linux go-fuse 依赖，判定为旧边界说明漂移；本批记录，下一次独立 kcfs 文档维护时对照公开类型与架构守卫修正，不借本批扩范围。
- 增量信号处置：独立基线对照确认 8 个迁移函数签名与函数体 token 等价、公开路径闭集与 Client 形状不变，46 个非 Snapshot 叶子说明逐项不变；帮助导航、真实资源效果、kcfs 边界及重复帮助项升级为回归守卫，根映射被旧描述误排除的问题当场修正。`remote_dataset.go` 的既有内部 pin / pin check 分支原样保留，登记为路由清理债：仅在后续明确授权清理退休内部入口时按调用闭集判断并处理，本批不因文件搬迁删分支。
- 最终验证：新增帮助回归先取得失败证据再修正到绿；受影响 CLI 定向检查 61 个顶层测试通过（含子测试共 144 个 pass 事件），0 fail / 0 skip。`client`、`snapshot/dataset`、`snapshot/access`、`snapshot/service`、`internal/arch`、文档与验证库存脚本测试通过；`make check-docs`（32 文档 / 149 关系）、`make check-surface`、`make quality` 与 `git diff --check` 全绿，质量工具使用本机已有模块缓存。未安装依赖、未启动部署服务/容器、未执行真实部署验收、未提交；本条只证明上述整理及相关合同范围。
