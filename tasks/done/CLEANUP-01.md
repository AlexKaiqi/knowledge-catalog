# CLEANUP-01 · 确定残留清理与入口防漂移（2026-10-10）

- [x] 本轮完成：清理已经复核的退役要求、失效入口、迁移说明和任务状态残留；已有工作区改动保留，受影响测试与契约通过，未提交。
- Goal / owner：让当前维护入口与实际职责、路径及执行结果一致。过程 owner 为 [工程质量闭环](../../doc/engineering/quality.md)，执行与证据 owner 为 [验证体系](../../doc/engineering/test-catalog.md)；基础验收和 CLI 归属分别由 基础重构验收、[CLI 交互](../../doc/design/affordance.md) 与现行包合同拥有。
- Non-Goals：不修改正在开发的对象 Interface / Dataset 控制台行为、协议或权限，不删除或削弱契约与守卫，不重新裁决 vendor 子集，不删除耐久状态、历史运行证据或未知用途工具，不启动真实部署验收，不安装依赖，不提交。
- 不变量：保持 API-01（共用应用执行器）与 A-01（依赖方向）；没有新增不变量。执行器的环境缺失失败、守卫可被反例击穿及同源码证据纪律沿用验证 owner 的过程合同。
- 选定 / 否决：选定最小修正、隔离临时夹具的红后绿回归、复用现有文档检查扩大维护链接覆盖；否决机械删除零引用符号、替换 Dolt 名称后外推 lakeFS 验收、大删迁移取舍、skip 换绿、放宽质量基线。
- 接口 / 验证：既有 Makefile 入口、check-surface / kcfs 脚本退出码、validation manifest.environment 与 check-docs 本地链接；精确形状仍由脚本、公开包合同及测试拥有。修改前已复现旧 Node 路径不存在、server 旧路由未被静态守卫扫描、缺 lakeFS 绑定返回成功以及退役 selector 进入验证 manifest。
- 复查补充：显式 kcfs 的平台/FUSE 缺失和容器安装/下载失败仍能返回成功，隔离反例后统一收口为失败；插件两个 HTTP 夹具仍模拟退役 workspaces 路由和字段，按正式 datasets 路由、列表身份、pin 与临时定义合同修正，原来源、撤权、旧 pin 和凭证断言保留。
- 验证结果：下列四次收口运行均为退出码 0、`sourceChanged=false`，共享源码指纹 `f819bf7e68a6416098234fd48c055ea2980e3ac1665f788c050d297e4261983b`。非 Go 结果按各工具原始摘要判读，不用验证工具的零 Go 事件冒充用例计数。既有任务状态不由本轮代勾选。

| run-id / scope | 执行范围 | 结果与边界 |
|---|---|---|
| `20261010T090433Z-8c2a9a71b725` / `cleanup:final-maintenance` | `make check-surface check-validation test-boundary` | 文档图 29 文档 / 110 关系；Python 6 + 4 项通过；Go 71 个测试终态事件 / 3 包通过，fail 0 / skip 0 |
| `20261010T090433Z-7e0f8c885816` / `cleanup:final-quality` | `make quality` | 闸层通过；26 单元，传播成本 33.23%，基线保持，无放宽 |
| `20261010T090433Z-26338056754e` / `cleanup:final-plugin` | Server Node 测试、插件 typecheck / test / build / pack:check | Node 14 项、Vitest 27 项通过，无跳过；使用已有 Node 24 和依赖，未执行 `npm ci`，不写成完整 `make test-plugin` |
| `20261010T090433Z-e33a83405461` / `cleanup:final-scenes` | `make test` | lakeFS HTTP 夹具普通/索引旅程与真实 OpenSearch；Go 21 个测试终态事件 / 1 包通过，fail 0 / skip 0 |

- 过程失败保留：`20261010T085832Z-98f162a43656` 的质量命令因 `GOPROXY=off` 无法查询已缓存工具的模块元数据而失败，后续改用本机文件代理通过，没有安装依赖；`20261010T085832Z-9c87b965a1d1` 的插件命令复现两项旧 HTTP 夹具失败，修正合同后通过。失败记录不改写成通过。
- 最后增量自检修正了新增容器模拟测试的日志隔离：先以三项失败反例证明宿主固定输出路径未进入 fixture，再只在 mock 执行接缝重定位输出，生产命令捕获与 setup 语义不变；四项回归通过。本条回写后再复跑文档、工具回归、架构与质量门禁；该增量与记录回写均不改业务源码。
- 验收边界：未执行真实 lakeFS 部署、Gitea / native provider 资格、真实 Linux/FUSE、模型或容量验收；本轮结果不代验收 OBJECT-INTERFACES-01 / DATASET-04，不把未执行记为通过。

本轮保留的候选与债（不按关键词或零引用自动删除；这是 CLEANUP-01 结束时的盘点，后续处置见 CLEANUP-02）：

| 位置 | 信号 | 判定 | 处置 | 触发条件 |
|---|---|---|---|---|
| `cli/kc_test.go` 的旧命令转换层、`doc/engineering/test-catalog.md` 的操作表与调用示例 | 多处测试仍经旧 argv/action 映射运行；部分文档仍使用退役 `catalog repo attach/create/archive` 标签或示例 | 历史接缝、内部动作与正式入口混用，须保留用例语义和断言，不能全局替换 | 保留；按风险 owner 核对正式 CLI，分清摘要标签与可执行示例，渐进迁移旅程后收窄兼容表 | 下次修改对应旅程、验证 owner 入口或专门认领测试迁移 |
| `scripts/assemble-doc.sh` | 仓内无调用，默认输出为已退役设计路径，但声明外部 Orchestrator 用途 | 外部停用无法确认 | 保留；确认仓外调用后退役或更新输出合同 | 外部工作流确认停用或重新接入组装工具 |
| `webui/src`、`package.json`、`content.go`、`go.mod`、上游 Playwright | KC 生产闭包只用参考树一部分，上游外壳与 E2E 未接当前入口 | vendor 全集有明确保留约定，子集边界待定 | 保留；先明确版本、许可、抽取及升级验证合同，再收窄 | 上游升级、接入参考页面或专门前端瘦身 |
| `webui/pub` 与 Vite 复制步骤 | 部分上游资产仅由参考页面使用，仍进入 KC 产物 | 可考虑收窄发布集合，来源资产不可直接判垃圾 | 保留；分清参考与发布资产，补浏览器和静态路由证据 | 发布体积治理或 vendor 子集改造 |
| `.validation/runs` | 本机约 490 MiB、275 次历史运行，持续追加 | 运行证据，缺保留与归档合同 | 本轮不删；按状态、时间、在途与引用关系制定保留规则 | 空间预算触发或建立自动回收入口 |
| `.session_tmps` | 临时产物混有独立 Markdown 草稿 | 独有内容是否已归档待核实 | 本轮不删；核对内容归档再回收 | 专门本地资料回收 |

设计使然与误报：vendor 参考树、公开 wire/协议形状及测试基建的零生产引用不直接等于死代码；`.kc` / `.kc-home` 是耐久状态，`node_modules` 是可重建本地依赖，均不纳入本轮源码删除。
