# PLUGIN-MINIMAL-EXAMPLES-01 · 插件重构合同与过程示例（2026-10-13）

- [x] 本轮按新版插件目标修正完成：逐个扩展点给出索引/LiveAccess 的实际需求和消费方；移除无需求支撑的通用返回改写点，补端到端固定依据、透明逐值取值/失败隔离、适用身份自动传递及独立访问证据，并同步索引查询/控制/CLI 分工，保持重构文档职责。
- Goal / owner：插件作者接入方式归 `plugin-extension.md`；索引真实候选与同依据回读沿 `declarative-index.md` / `retrieval.md`，访问观察沿 `resource-access.md`，最终交付沿 `permissions.md` / `service.md`。已核对 docs/graph 所有权、直接关系和现有公开调用/交付代码。
- Non-Goals / 不变量：只改文档、阅读入口和图主题登记，不改 Go、测试、协议、CLI 或其他任务，不安装依赖、不启动服务、不提交；保持 W-01/W-02、V-01/KS-01、C-01/R-01/R-02、AUTH-01～03/KS-02、P-01/PC-01、L-01/API-01。
- 选定 / 否决：由最新版两个 owner 的承诺反推接点，每项必须有具名使用需求、payload、消费方及失败语义；平台固定回读、权限、结果组装和凭证目标限制作为强制职责，不混成可替换 Hook。LiveAccess 单值失败说明是已选定行为变化，公开表示与实现仍须选定并取证；否决以当前 `(value,error)` 限制新目标、预建通用返回点和把透明读取局部失败外推为完整动态搜索。
- 验证与质量出口：独立按完整重构分母及两个例子复核，重读 owner 和文档图，运行 `make check-docs`、`make quality`、本轮空白检查；登记、全部任务、维护及资源的真实接线差距在正文明确，首个实施切片触发时取证。返回接点保留现有合同，不新增通用返回框架要求，不以文档闸宣称运行验收。
- 新版对齐证据：已重读两插件 owner、共享 owner 和文档图；独立复核确认需求矩阵、强制系统阶段、局部失败/严格查询、模块分工及独立留证正确，例子与验收同步。`make check-docs` 通过（33 文档 / 169 关系），`make quality` 退出 0，日志 `/tmp/kc-plugin-requirements-derived-quality-20261013.log`，增量及新增文件空白检查通过。本轮只改共享插件篇、阅读入口、该篇图主题和本条记录，未改两个插件 owner 或 Go、运行测试及部署。
- 实施差距出口：`liveaccess/service/hydrate.go` 的整次错误上返、`object/read.go` 缺逐值失败表示、单仓业务 READ 未透明取值、原观察公开选择及真实保留未齐，判定为新版目标待实现；LiveAccess 接入时先选公开形状，再补逐值/对象拒绝/严格投影回归。`caller_credentials.go` 透传不证明目标适用/重定向强制，按权限 owner 补边界；`observability/access.go` 现有坐标/指针/整体错误及 RequestID 不证明原值/逐值失败或请求查询，独立审计目标落地时补原结果关联、可靠保留及授权查询。索引 control/CLI 仍是目标归属，按声明式索引 owner 分工迁移并验真实调用，不将文档绿色写成这些行为已完成。
- 本轮证据：独立复核确认文档职责及完整重构合同恢复，精确选择、固定准备/恢复、后台身份、物理隔离、旧资源与续页、失败/退出保证保留；例子只辅助说明。`make check-docs` 通过（33 文档 / 168 关系），`make quality` 退出 0，日志 `/tmp/kc-plugin-refactor-contract-quality-20261013.log`，增量及新增文件空白检查通过；owner 与图已重读。仍仅文档、阅读入口和图主题登记，未改 Go 或执行运行/部署验收。
- 交付证据：独立复核确认两例、现有注入口、Schema/观察核验、返回采用和 SEARCH 专属交付范围准确；补回 Dataset 不继承来源选择、一次任务同实例与遍历贡献。owner 与图已重读，`make check-docs` 通过（33 文档 / 168 关系），`make quality` 退出 0，日志 `/tmp/kc-plugin-minimal-examples-quality-20261013.log`；增量与新增文件空白检查通过。未修改 Go、运行行为测试、部署或提交；通用登记和返回阶段注入明确待实施，没有以任意字段改写或伪造观察示范新政策。
