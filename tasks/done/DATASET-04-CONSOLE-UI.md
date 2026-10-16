# DATASET-04-CONSOLE-UI · 完整 lakeFS 控制台组合与 Dataset 增量（原编号 DATASET-04，与前一条重号）

- [ ] 本轮认领（2026-10-10）：在独立工作树 `codex/dataset-console-ui`、已提交基线 `2aa827f` 上继续现有改动；原目录其它会话的改动全部保留，不纳入本轮提交。

**Goal / owner：** 直接复用 vendored lakeFS v1.58.0 / Apache-2.0 的完整前端和已有 lakeFS 后端，仅新增 Dataset 与必要接线。保留 Repository、对象、提交、分支、标签、比较、仓 PR、Actions、Settings、Administration、原登录、Setup 和 PluginManager。Dataset 经 KC 完成认证、明确 Catalog 选择、Form/JSON 定义、固定来源预览后发布、历史/定义比较、文件交付和退役。owner：`doc/design/dataset.md`、`doc/design/service.md`、`doc/design/permissions.md`、`doc/design/service.md`；工程过程归 `doc/engineering/quality.md`。

**Non-Goals：** 不重做 lakeFS 原页面或后端 API、不缩减原控制台、不复刻商业 Dataset 实现、不新增发布权威或跨仓事务/隐式发权、不升级或安装依赖、不修改现有部署、不操作原工作区。独立真实后端只为本轮验证启动并清理，不代替 `make deploy-local-scenes` 等部署验收合同。

**不变量 / 接口：** `KS-01`、`KS-02`、`V-01`、`AUTH-01`–`AUTH-03`、`API-01`、`IX-01`。Dataset 使用已接受 pin，来源版本不漂移，当前授权仍逐请求生效。Catalog 留在 URL，取消/退出/身份变化使在途结果失效。接口沿用 `client.AuthDiscovery` / `BrowserLoginConfig` / `TokenRequest`、`httpsurface.Patterns()`、`catalog.KnowledgeSetSource` / `DatasetItem` / `ResolvedKnowledgeSet` 和 File Gateway。

**选定 / 否决：** 原页面直接调用 lakeFS 原 API 并使用用户原身份；Dataset 调 KC 闭合 API，两边经同源入口分流，不注入平台管理员凭证。复用原 IndexPage/Layout/TopNav/PluginManager，只增 Dataset routes。发布先校验完整定义并确认来源预览，定义变化后必须重新预览，按确认的 commit 发布；未知 JSON 字段拒绝。历史由 Catalog 的真实已接受记录提供，原详情 GET 点查/有界分页，任意真实 revision 仍可点查；不按 1..N 造历史。接口细节只由 `server/README.md`、公开实现与回归拥有。否决专用缩减外壳、重写 native API、客户端自建发布账、默认首 Catalog 和手工 localStorage 身份配置。

**清理与修复：** 移除旧 Dataset 专用外壳及其手工凭证提示；恢复完整原入口。原 API 路径与能力不重写，仅为统一登录补原用户缓存的迟到请求守卫；撤回无关 ObjectRenderer 性能重构，删除测试专用重复 `readFile` helper 和 BrowserRouter 已处理的重复 basename 归一化。原对象渲染能力保留。导航共用 NavItem 不再二次解析 href，仓内及 Administration tabs 的真实 href/点击均有回归。构建输出归 Server embed，已声明页面支持深链接，缺失资产 404，含点 ID 不按扩展名误拒。固定 CSP 不再阻断原控制台预签名、远端图片、PDF 与 SSO。旧 mock 浏览器 fixture 已关闭，其临时脚本/overlay/地址文件已删除；其导航结果不算真实后端验收。

**验证合同：** 前端类型/lint/行为/构建、受影响 Go、`make test`、`make test-contracts`、`make check-docs`、`make quality` 执行到绿并记录源码指纹。实际浏览器必须连接独立真实 lakeFS，覆盖原仓功能与 Dataset；mock 只用于隔离回归，不能作为产品验收。此前旧独立总检查中断、无最终报告，共享目录 source-changed 记录及早期进程内夹具走查均不计本次真实验收。具体结果、源码边界与浏览器证据见 [控制台验证记录](../../snapshot/console/webui/CONSOLE-VALIDATION.md)。

**API review：** `webui/DATASET-API-REVIEW.md` 对照官方文档/SDK，区分当前商业 Dataset 与锁定的 v1.58.0 前端。已复用 KC 合同补 Form/JSON、确认来源发布、真实 History/Compare。D-01–D-05 具体登记元数据 schema/description、Dataset 定义 PR、Iceberg table/namespace、逐版作者/时间/消息与 Dataset 消费协议兼容差距；包含位置、信号、判定、处置和触发条件，不伪装成可用控件。

**质量三出口：** 构建路径、资源 404、Catalog 丢失、Base64/分页、迟到响应、默认分支、合法名称冲突、旧来源隐式范围扩大、预览失效和双 basename 均当场修并升级回归。剩余债：① `webui/tsconfig.json` / 锁定第三方声明兼容：整树源码已纳入 `tsconfig.console.json`，当前 `skipLibCheck: true`，JS 保持 `allowJs: true` / `checkJs: false`；触发为获批依赖升级或声明修复后恢复库检查。② 锁定工具链 Browserslist/AST 弃用与原 renderer 大 chunk：保留版本/能力，不改阈值；触发为获批 vendor 更新或首屏预算评估。③ `main.test.ts` 恒真存量断言不计实质证据；触发为上游测试/入口调整时替换。④ 原 browser WASM 在 Node/happy-dom import 时的 worker stderr 是测试运行时兼容债，不为它改生产 renderer；实际 browser 验证该能力，触发为获批依赖或测试运行时升级。

**验收边界：** 不新增 skip、不削弱合同。既有 short contracts 排除外部 live 用例；按 AGENTS“全绿且无 skip”要求，未取得全部证据前保持 `[ ]`。生产同源配置见 `webui/KC-VENDOR.md`；配置只供审阅，不应用到现有实例。外部 Taihu/SSO、部署场景、容量与其它未执行入口不能写成通过。

**本次运行与新增登记：** 固定源码宽检查 `make test` 21 pass / 0 fail / 0 skip，`make test-contracts` 1815 pass / 0 fail / 20 个既有外部/live skip，质量（含 docs）通过；运行 ID、指纹和随后仅前端/接线/记录的差异见验证记录。真实原 API、S3、KC Server 和浏览器证明仓库/分支/标签/提交/PR/比较、原 DuckDB CSV 查询，以及 Dataset 来源推进后固定提交、历史/字节/退役。文件选择器 UI 未获得 chooser，原 API 上传与 UI 提交分开记，不冒充通过；位置 `webui/src/pages/repositories/repository/objects.jsx`，判定为验证工具缺口，待可操作 chooser 的浏览器或人工补验。原 native 登录返回丢失查询是存量债，位置 `webui/src/lib/hooks/api.jsx`，判定为原返回地址只保留 pathname；本轮记录，触发为下次 native 登录/返回地址调整时修并补回归。最终审计发现的登出截断 IdP 跳转与遗漏 OIDC 分流当场恢复原导航并补接线回归，外部 SSO 仍不宣称实测。独立服务与测试凭据在保存证据后清理，原目录和现有实例未改动。


**统一控制台登录纠偏（2026-10-10）：** 用户进入 Datasets 不应再次登录。Goal 是保留原 lakeFS 登录/SSO 与 HttpOnly 会话，Dataset 复用同一已验证用户；KC 仍逐请求独立评权（owner：Permissions 的耐久身份绑定与 AUTH-01–03、Service 的认证无 Workspace session）。Non-Goals 是前端冒充 principal、保存原访问密钥或复刻 lakeFS 登录 API、把 lakeFS 管理员自动当 KC 管理员、变更已有部署。选定最小接线：显式 lakefs 认证 adapter 向部署固定 lakeFS `/api/v1/user` 验证调用方证明，沿用 VerifiedUser/identity.Bind；组合控制台直接复用原登录页、用户菜单与退出，Cookie 请求需同源防跨站校验，401 返回原登录并保留完整目标 URL。否决 Dataset 独立登录表单、用户名转 local 断言与直接信任浏览器缓存。接口仍复用 AuthDiscovery.accepts 与 HTTPAuthenticator，不新增登录服务或 Dataset 发布权威；具体可信来源与证明边界由公开 server 实现/README 与回归拥有。原有两次登录的浏览器记录只证明旧实现，不能作为此次单次登录验收。


**认证接线的质量登记：** 原 lakeFS v1.58 `/api/v1/user` 只公开不可编辑的 `id` 与秒级 `creation_date`，没有声明删除后重建的不可重派 subject。lakefs adapter 绑定 origin + id + creation_date，普通同名重建会被现有身份账拒绝，但同秒重建无法仅凭该 API 区分。位置 `server/auth_lakefs.go` / 原 lakeFS User 合同；信号是公开身份代际精度不足；判定为依赖方身份生命周期缺口；处置是保持 DATASET-04 未完成、如实限定验收与部署账户回收要求，不宣称完整不可重派保证；触发为账户删除/重建、迁移、上游提供稳定 subject 或统一 IdP 接线时，必须显式处置旧绑定和旧授权，不能让同名新账号继承旧权。官方依据：`https://github.com/treeverse/lakeFS/blob/v1.58.0/api/swagger.yml` 与 `pkg/api/controller.go` 的 GetCurrentUser。


**身份映射边界登记：** 位置 `server/auth_lakefs.go` / `identity.CanonicalUsername`；原 lakeFS 可使用邮箱或其它 KC 不接受的账号 ID，而 KC 已选定便携用户名不能隐式改名。判定为显式身份关联缺口；本轮兼容的原账号直接复用，非法 KC 映射明确拒绝且不要求重新登录，不猜测别名。触发为部署使用邮箱/长 ID/大写账号或跨 IdP 关联时，在身份 owner 下提供显式关联与其回归；单次登录走查只证明本轮真实 compatible 账号，不宣称全部 IdP 用户类型通过。


**原登录缓存与导航的增量修复：** 只读复现发现旧 native `/user` 请求可在清缓存后迟到写回旧账号，本轮原用户缓存增加失效代次并覆盖这种请求交错；认证 401 使用同一失效点。原登录的 render 导航警告与丢失 query/hash 在此次回跳接线中当场修，恶意站外/反斜杠 next 不作为登录目的地。保留原 API 路径、SSO 和登录提交；这些必要接线不再登记为留待下轮的债。

**单次登录实测与最终闸：** 独立真实 lakeFS/MinIO/KC 环境完成一次原登录、Catalog 选择、Dataset 来源校验/发布/真实文件回读、原仓对象浏览、原退出及 query/hash 回跳；公开 API 撤 KC grant 后 UI 明确 403，而原仓读取仍 200。证据见 `webui/CONSOLE-VALIDATION.md` 与本地 `single-login-*-proof.json`，服务和一次性凭据已清理。首轮前端 105 测试、普通 21 测试、契约 1836 通过/20 既有 live skip；质量发现 `DeploymentConfig.Validate` 复杂度增量 51 超上限 50，采用同集合等价 switch 当场修、不改阈值，再对最终源码执行闸与宽回归。修改过的原 API 用户缓存纳入永久 lint 入口；最终运行指纹与完整 skip 名单归入本地 `single-login-final-checks.json` / `single-login-go-summary.json`，不把纠偏前或修复前记录拼成最终验收。

**Access Control 增量（2026-10-10，用户扩展本条范围）：** Goal 是在保留原 Administration / Users / Groups / Policies / Credentials 的前提下，补齐 KC 当前主体的权限自查、外部申请入口和既有授权规则的查看、添加、撤销，共用原登录。owner 为 Permissions 的身份、动作与范围及准入管理，Service 的共享 typed 用例；不变量为 `AUTH-01`–`AUTH-03`、`KS-02` / `K-20`、`API-01`、`O-01`。Non-Goals 是 KC 新建用户组/角色/策略权威、lakeFS 与 KC 隐式继承或同步权限、对象读 ACL、自建申请审批流、客户端推断有效权限或注入管理凭据。选定复用 `client.AdmissionResult`、`GrantRequest`、`invocation.AllowRule` 和已登记的 identity/admin HTTP 入口；自查必须完整呈现现有约束，管理列表不能泄露私有初始化账或把筛选解释成求值，最终授权始终由 Server 执行。否决把 lakeFS 商业 RBAC 功能门禁套到 KC grants、以模拟后端作为验收和新增第二套 permission evaluator。参考锁定 v1.58.0 官方 API 与当前官方访问控制说明，复用 native API 的事实及剩余差距另记 API review。实现后补回归、最终源码检查与独立真实后端证据，未验证的范围不写成通过。

**提交与合并授权（2026-10-10）：** 用户在本轮实现及真实后端验证后明确要求整理、合并、提交并推送 GitHub。交付为一条完整控制台、Dataset、共享登录和 Access Control 的聚合提交，快进进入 GitHub 默认 `main`；基线已含但尚未进入 `main` 的 38 条提交随快进保留。原共享目录其它会话的未提交文件与生成/运行证据不纳入提交；锁文件和依赖保持原样。最终实现检查为前端 144 个测试、普通 21 个测试、契约 1856 个通过/0 失败/20 个既有 live skip，类型、构建、质量（含 docs）通过，具体运行与边界见控制台验证记录。`DATASET-04` 仍保持未完成，提交授权不改变无 skip 的验收条件。

**Snapshot Store 归属纠偏（2026-10-12，沿用 DATASET-04）：** 用户确认 Dataset 以文件坐标定义，Dataset、对应权限、HTTP 接口及完整控制台应集中为可独立使用的 Snapshot Store 模块，上层 KC 调用。Goal 是将文件定义、固定版本组合、发布、原始字节交付、显式授权核心和控制台的实现权威收口到 `snapshot/dataset`、`snapshot/access`、`snapshot/service`、`snapshot/console`；owner 为 Core Architecture、Dataset、Permissions、Service 与 Capability Organization。Non-Goals 是改变公开 JSON/YAML/命令/路由、引入知识对象解释、创建第二份 Dataset 或授权账、复制两套执行器、重做原 lakeFS、升级依赖或改已有部署。不变量沿用 `A-01`、`L-01`、`API-01`、`AUTH-01`–`AUTH-03`、`KS-01`、`KS-02`、`V-01`、`CA-01`。

选定模块内部按职责分包：底层 `snapshot.Store`/文件树合同保持小；Dataset 的实现和登记历史归模块；Catalog 旧公开路径只留兼容口与外部宿主消费者；通用身份绑定继续复用 `identity`，原 KC 默认政策、Knowledge 准入、语义文件投影和观察证据由外部 adapter 提供，Snapshot 不解释它们。接口参照现有 `catalog.KnowledgeSetSource` / `DatasetItem` / 发布 pin、`invocation.AllowRule` / `GrantList`、既有 HTTP 闭集与原控制台，迁移后仅更改权威实现归属。否决只移动单个 Dataset 文件、把 Home/Invocation 当独立模块后端、权限镜像同步、静态 UI 占位替代实质能力，以及将 Knowledge 搜索/Schema 搬到存储模块。新核心不得反向依赖 `catalog`、`home`、`invocation`、`server` 或 Knowledge。

执行前已检查当前实现与依赖：Dataset 来源为仓/commit/路径，不含知识对象身份；现有文件网关混合原始文件和语义投影，抽取时必须明确分开。原契约、原路由、原登录和全部原 lakeFS 能力保留，目录守卫同变更跟进。前端类型/lint/行为/构建、对应 Go、架构、文档、质量、普通与完整契约逐步检查；实际后端验证只使用独立环境，不将此前已清理旧运行冒充新模块验收。未经最终合同证据，本条仍保持 `[ ]`。

本次迁移债：位置为 `catalog/compatibility.go` 的 `EmptyCatalogState`；信号是 Go 不能为跨包变量提供可赋值别名。旧值读取保持，新核心默认值只由 `snapshot/dataset.EmptyCatalogState` 拥有；给旧变量赋值不影响新核心，当前全仓未发现依赖该赋值的消费者。判定为历史公开可变变量的兼容边缘，处置是保留读取形状并在旧包 README 明示，不反向导入旧包或使用 unsafe。触发为外部消费者依赖重新赋值时，须显式迁移其默认状态配置，不能宣称可变别名完全兼容。

本次外部政策兼容债：位置 `server/snapshot_service.go` 的临时定义读权适配与 `invocation.RequireCompleteWorkspaceRead`；信号是旧 KC 临时配方的 Repository fallback 仍求值 `knowledge.read`，只有 Repo 级 `file.read` 的调用者在旧 KC 适配中可能被拒绝。独立 Snapshot 模块只按显式文件政策求值，不需要 Knowledge 权限。判定为外部 KC 既有政策差异，处置是保持本次迁移的公开授权语义并如实区分两个装配方，不向文件核心反向导入 Knowledge。触发为临时定义的纯文件用户接线或权限政策 review 时，须在权限 owner 下统一选择与回归，不能宣称两种默认政策相同。

本次增量审计当场修：文件接口先检查请求范围、再打开并核对 Catalog 权威、再复核实际范围；错误 provider 和空 provider 失败关闭，未授权请求不能借错误区分隐藏 Catalog 是否存在。发布准备与退役在前置 hook 之后复核 `dataset.manage`，撤权后不发生 portable 准备写或退役。移除原 Server 原始文件引擎与 dead wrappers，生命周期回归改走模块实际 HTTP 路由，借出的 Store 在所有并发请求完成前不得关闭；没有保留只供测试使用的生产适配层。

迁移后的宽检查预检当场修：验证库存继续只扫描旧 Server owner，漏计模块 19 条真实路由；库存现在同时扫描 Server 与 Snapshot Service 的 `Handle`/`HandleFunc`，保持原 88 条闭集和重复/动态注册拒绝。另一项继承问题在本轮原始文件引擎触达面发现：根 mount 的相对文件请求被错误分派为交付请求而返回 400；已用会失败回归和独立真实 lakeFS 复现，修复分派并守卫 SubPath、历史固定版本、混用坐标、父目录逃逸、伪清单与撤权，不能用增加路径特例绕过授权。


**Snapshot 模块耦合口径与棘轮收紧（2026-10-12）：** 位置 `scripts/coupling` 的顶层 unit 归组、`baseline.json`，关联 owner 为 Capability Organization 与 Quality Loop §7。首次质量检查保留失败证据：旧顶层口径传播成本 33.230769%→35.846154%，Snapshot I 0.133333→0.1875。信号的唯一新增聚合边是 `snapshot/service→identity` 被记为 `snapshot→identity`；实际 Go package 图无环，核心不消费身份，旧 `internal` 混合桶把身份加入聚合循环并制造 17 个可达格。判定为已授权目录归属调整下的逻辑单元失真，而非允许反向耦合；处置采用最小明确政策：仅 `snapshot/service` 及其子包为具名嵌套应用单元，其它单位、全部生产 imports、可达性算法和严格阈值保留，包级边界守卫不变。政策签名 `path-depth=1;named-subtree=snapshot/service;version=1` 随基线保存，旧无签名、不同政策或不同 depth 一律不可比较。

同一新工具与平台（Go/GOOS/GOARCH 见本地证据）对独立 `HEAD` archive 与当前源码重算：HEAD 26 单元、216/650=33.230769%；当前 27 单元、227/702=32.336182%，同口径收紧 0.894587pp。旧单元最大不稳定度漂移为 identity 0.333333→0.285714（4.761905pp，小于 5pp）；Snapshot 核心 0.133333→0.125。用 HEAD 的同政策基线执行 strict 仍因唯一新 unit `snapshot/service` 拒绝（exit 1），传播成本与旧单元 drift 均未失败；随后显式 `-write-baseline` 登记当前收紧值，未抬高旧阈值。先失败的命名单位回归、实际 go list imports 保留、政策不匹配、未知 unit、传播上升、漂移边界与可达性算术均纳入回归。顺带当场修复旧浮点计算把理论恰好 5pp 误判为略小于阈值的问题：仅补 1e-12 舍入容差，准确执行原 ≥5pp 硬门，不放宽阈值。触发条件：后续模块再出现新的逻辑能力边界，必须重算同政策改前/改后、保留图与漂移证据再显式收紧，不能用改名或省略 import 逃避守卫。证据在忽略目录 `.validation/coupling/20261012/`；这只证明度量与边界回归，不替代完整契约或真实后端验收。

**耦合只读代理的存量债：** 位置 `scripts/coupling/main.go` 的 `scanDecls`；信号是 interface 计数的文本切片仍包含类型名，导致 A/D 报告不能准确识别现有接口。判定为只读代理缺口，不影响本次 Ce/Ca/I、可达性和 strict 判定；处置为登记后在抽象度指标维护时修复并补接口声明回归，不借本次 unit 迁移扩张指标门禁。触发条件：A/D 指标解释、维护或升格为自动闸时必须先处置，不能把当前零代理值解释为实际无接口。


- [ ] SNAPSHOT-INTEGRATION-01（2026-10-12，当前认领）：保留 Desktop 未提交增量，整合并完成 Snapshot Store 公开边界重构。

**Goal：** lakeFS 是唯一生产 Snapshot 后端，Store、Dataset、Access、HTTP、Console 的实际目录、契约和实现各归其主。外部只 import 根包及 dataset/access/service/console 四个具名一级包。Owner 为 Core Architecture、Dataset、Permissions、Service、Capability Organization、Quality Loop。

**Non-Goals：** 不进行 Knowledge→Object 名称/API 迁移，不补 Dataset 商业扩展，不升级依赖，不改既有部署，不自动提交或推送；不删除独立 Gitea 身份认证。Desktop 其它主题增量、删除和索引状态保留。

**不变量：** A-01、L-01、API-01、AUTH-01–03、KS-01–02、V-01、CA-01。固定文件版本不冻结授权；文件组合不产生 Object 身份；同一发布、授权、幂等账继续承担权威。

**选定与否决：** 每个模块包含 contract.go、同包实现与测试，按需使用 owner-local internal。lakeFS 私有实现只消费存储合同。必要 RawWriter/CommandLedger 公开接口归根包，不暴露具体 Writer/Ledger/Repository/Catalog/Registry 或内部 journal；保留合法能力接口断言。退役 Gitea/filegit 存储、旧 catalog 转发和无正式消费者的 RootDir Git checkout。否决仅搬目录而保留深层 import、具体实现别名、函数变量直通、反向依赖或重复权威。接口由 snapshot/store.go、write_contract.go、command_contract.go 及四模块 contract.go/README/Conformance 选定。

**验证与回写：** 先保留边界失败证据，再收口实现；检查包含测试与嵌套模块。对同一候选源码运行受影响回归、前端、check-docs、普通/契约、quality，再以独立真实 lakeFS/MinIO 验证完整控制台。仅通过后逐文件回写 Desktop，每个写入先核对冻结快照与当前值，避免覆盖并发改动。旧结果不计本次验收；既有 live skip 如实列明，本项满足全绿且无 skip 前保持未完成。逐文件源证据和范围见 .validation/reconciliation/20261012T064756Z/REFACTOR-TARGET.md。

**本项控制台纠偏（2026-10-12）：** Goal 是复用原控制台差异展示与 Repository Compare，让 Dataset 首页直接列出获准 Catalog 的数据集，Catalog 作为可选过滤和明确的创建归属；列表、导航、版本工具栏和文件浏览沿用原控制台组件与样式。Owner 为 Dataset、Service 与 Core Architecture，不变量 V-01、KS-01–02、AUTH-02。Non-Goals：不把组合树当成单仓，不新增本地 Git 权威，不改变 Catalog 身份、消费范围和权限，不升级依赖或修改现有部署。选定共享原文本 diff、显式跳转原仓固定 commit 比较，来源入口仅依据受信同实例坐标；否决手写 item 对比、猜测 kr 标识对应原仓名及自动用 Dataset 授权读取整仓。合同仍为 snapshot/dataset/contract.go、snapshot/service/contract.go；独立真实后端与浏览器验收另记 .validation/console-ux/20261012/。

**本项 Access Control 纠偏（2026-10-12）：** Goal 为我的权限与授权管理共用规则表单，字段和值分栏，补齐有管理权的规则编辑，恢复原 My Credentials/Users/Groups 的可进入页面。Owner 为 Permissions、Service、Snapshot access/service/console 合同；不变量 AUTH-01–03、KS-02、V-01。Non-Goals：不另建 KC 用户/组权威，不改变现有部署或认证模式，不因个人权限页面而授予管理权，不升级依赖。选定同一规则组件的查看/编辑/新增模式；PUT 原子替换现有规则，在 RuleStore 同一序列化边界校验旧范围、新范围与 expected 旧规则，保持 ID 和来源，冲突及失败不改授权账。分享生成规则继续通过分享管理修改。否决先撤销再新增、客户端求值自我发权、仅取消禁用后把社区后端不支持的管理写成可用。接口由 snapshot/access/contract.go、snapshot/service/README.md 与 httpsurface 注册表承接；先红后绿的回归、真实原 API 与浏览器验证记录在 .validation/access-ux/20261012/。

**Repo / Dataset 授权接线（2026-10-12，本项续作）：** Goal 为复用现有认证器与耐久身份绑定，把 Repo、Dataset 产品授权归 snapshot/access；snapshot/service 对完整原控制台 Repo API 强制检查当前授权后，以调用者原证明转发 lakeFS。Owner：Permissions、Service、Snapshot Store；不变量 API-01、AUTH-01–03、KS-02、V-01、CA-01。Non-Goals：不重做账号、组、凭据或登录，不建立第二份政策，不升级依赖、修改现有部署或自动提交推送。以 Desktop 已提交 fa52d18 的 Registry / Dataset 服务合同为准，不恢复 Catalog API。选定闭集原 API 操作映射、真实同实例仓坐标、当前规则与已有默认政策、库存过滤后分页、原始流交付；否决客户端藏按钮、方法粗分权限、平台服务凭据注入、Dataset 权限开放来源整仓。接口见 snapshot/access/contract.go、snapshot/service/contract.go。新增原调用方扫描守卫防止再次写错 PR 合并方法；真实 v1.86 后端已证实原 v1.58 控制台使用 PUT 合并，未定义路由移除。

**验收与剩余边界：** 2026-10-13 已自建独立真实 Gitea 1.26.3 认证及未修改的 lakeFS 1.86.0，两个真实主体的 API/CLI 初轮 82 项检查通过；最终浏览器、撤权与源码绑定检查以 .validation/shared-auth/20261013/ 最终报告为准。此前单主体证据不计此次多用户验收。snapshot/service/native_api.go 物理 Repo DELETE 尚无部署绑定、托管恢复及 Dataset 来源的完整耐久解绑生命周期，当前先授权再明确拒绝；完整删除需求触发实现。后端失败审计债已当场修复：原结果与 HTTP 状态归为实际失败，私有存储认证失败为服务不可用；回归覆盖 401/403/404/500，不伪造成功。锁定库声明仍 skipLibCheck:true；既有 live skip 单独披露，本项不标完成。旧证据与逐文件整合清单放 .validation/repo-access-current/。

原 API 写操作另需尊重宿主已挂载 Store 的字面文件写能力与归档状态；不按保留仓名隐式发权。独立真实 system 仓的引用创建曾绕过只读挂载，已取得红证据并恢复测试分支；补 generic capability 检查与宽授权不能绕过只读仓的回归。

**现有认证与私网存储接线（2026-10-13，用户确认后实施）：** Goal 是复用现有 Taihu / Gitea 认证器与耐久身份绑定，在 Snapshot 模块统一执行 Repo、Dataset 授权，授权后由服务端私有存储凭据调用未修改的 lakeFS 1.86。Owner 为 Permissions、Deployment Auth、Service；不变量 AUTH-01–03、API-01、KS-02、V-01、CA-01。Non-Goals：不重做账号、组、登录权威，不修改 lakeFS 源码、升级依赖、恢复 Catalog 合同或改动已有部署，不覆盖 Desktop 的其它修改。选定统一业务入口、原 Repo 操作闭集、真实仓绑定、当前授权账及受控后端客户端；替换此前只适用于 lakeFS 用户证明的原样转发接线。底层账号/凭据/初始化 API 不得随存储凭据对外转发；原控制台账号入口使用已有认证提供方，操作审计区分实际调用者与存储账号。接口由 snapshot/service/contract.go、identity/http.go 和既有 Client 认证合同承接，Server 只装配。不采用旧版本或模拟认证作为验收；独立真实 Gitea 多用户认证 + lakeFS 1.86 验证控制台、CLI、撤权与绕过，Taihu 验签/授权码合同另按其既有验证边界记录。逐文件基线、红绿证据与清理记录在 .validation/shared-auth/20261013/。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。早于 2026-10-14 对象模型重构的认领，相关代码与设计已被后续重构改写（见 `0759640` 之后的提交），原范围按旧结构写成，不再继续。整体验收并入 `FINAL-ACCEPTANCE-01`，插件相关用例由验收规格 accept-01～13 取代。
