# DATASET-04 · Dataset 控制台登录与管理闭环

- [ ] 本轮认领（2026-10-10）：沿用 vendored lakeFS v1.58.0 / Apache-2.0，在 `codex/dataset-console` 分支补齐 KC 登录与 Dataset 管理；保留原有工作区改动，不提交。

**Goal / owner：** 浏览器经 KC Server 完成认证发现、验证登录、续期和退出；明确选定 Catalog 后完成 Dataset 列表、来源选择、编辑发布新版、固定版本交付树与文件预览、退役。修复 Vite 输出与 Server embed 路径不一致、Catalog 上下文丢失与 File Gateway Base64/分页合同错误。owner：`doc/design/dataset.md`、`doc/design/service.md`、`doc/design/permissions.md`、`doc/design/service.md`；过程 owner：`doc/engineering/quality.md`。

**Non-Goals：** 不升级上游、不复刻 lakeFS 商业 Dataset 后端，不新增协议或服务端认证会话，不提供跨仓写入/隐式发权，不改变既有发布与撤权语义，不启动真实部署验收栈、不安装依赖、不提交。

**不变量：** `KS-01`、`KS-02`、`V-01`、`AUTH-01`–`AUTH-03`、`API-01`。同一浏览使用已接受 pin；凭证刷新不改版本；登出/身份切换清除在途结果；Catalog 明确进入 URL，页面不得静默回落首项。

**选定 / 否决：** 选定同源 KC API 适配层、浏览器本地验证登录态、现有 PAR/PKCE/token 接口、统一 Catalog 路由、来源表单复用现有发布合同；否决直连 lakeFS 或另建 Dataset 权威、前端推导权限、凭旧 pin 续权、缺省首 Catalog 与手工 localStorage 配置。

**接口 / 验证：** `client.AuthDiscovery` / `BrowserLoginConfig` / `TokenRequest`，`httpsurface.Patterns()`，`catalog.KnowledgeSetSource` / `DatasetItem` / `ResolvedKnowledgeSet`，`server` File Gateway。先以回归测试复现登录入口、Catalog 导航、Base64 读取和构建路径问题，再修复。交付前执行前端行为/类型/构建、真实浏览器回归、受影响 Go 回归、`make test`、`make test-contracts`、`make check-docs`、`make quality`；真实部署验收独立，不宣称已通过。

**选定合同补充：** 原有 Dataset 详情 GET 接受 `revision` 查询，返回已接受来源、真实发布版本列表与当前服务版；管理可见性不变，历史生命周期采用当前判定。未新增路由、Client DTO、写入口或发布权威。具体字段只由 `server/README.md`、公开实现与回归拥有。

**质量三出口：** ① 构建/资源存在性、Catalog 丢失、字节/分页、迟到发布和退役响应、默认分支缺失、名称 `new` 的页面冲突、旧来源缺少交付路径时的隐式范围扩张、文件预览错误重试均已修复，并升级为回归；开发代理共用配置，避免重复修改漏项。② 位置 `webui/tsconfig.json` 与保留上游页面，信号是整树类型检查的存量错误，判定为未接入 KC 入口的上游参考树兼容债，处置为保留并以严格 `tsconfig.console.json` 检查实际交付闭包（不关闭库检查），触发条件是任何上游页面接入路由或依赖升级，届时恢复其类型和行为检查。现有 lint 工具的 AST 弃用提示同属锁定依赖升级时处理的兼容债，控制台文件 lint 零警告仍为交付门禁。③ 位置 `webui/src/main.test.ts`，信号是原有恒真占位断言，判定为无行为证据的存量测试债，处置为不计入本轮 KC 回归证据（入口另有构建资产/深链接合同与实际浏览器走查），触发条件是上游测试或入口调整时替换占位，不以其通过作为完成证明。

**实际浏览器证据：** 临时 KC Server + lakeFS 进程内夹具；local 登录→明确选 Catalog→目录来源创建 v1→保留固定提交改布局发布 v2→选择 v1 并刷新深链接→原字节预览/下载→退役后历史元数据仍在、消费入口消失→退出/刷新要求登录；无浏览器错误，夹具终局确认 v2 已退役且原 Dataset 未改变。真实 Taihu 授权与部署验收未运行。

**验收边界：** 既有 `make test-contracts` 的 short 合同主动排除外部 live 用例；本轮不新增 skip、不削弱断言。按 AGENTS 的“全绿且无 skip”条件，此条暂不标 `[x]`，交付结果分别记录本轮有效门禁和外部验收范围。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。早于 2026-10-14 对象模型重构的认领，相关代码与设计已被后续重构改写（见 `0759640` 之后的提交），原范围按旧结构写成，不再继续。整体验收并入 `FINAL-ACCEPTANCE-01`，插件相关用例由验收规格 accept-01～13 取代。
