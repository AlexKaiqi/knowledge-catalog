# server/

服务接入面：method+pattern 路由闭集、typed handler 注册、授权强制与遥测的宿主。
CAP-ORG-01 B4 先立契约（`routes.go` 与导出金样 `routes_golden.txt` 双向对账，原 `httpsurface` 包已由 ROUTE-MANIFEST-01 退役）；B6 把 cli 的
serve/service_routes/handlers/授权/遥测/就绪探测搬入（B6/B7）。生产 CLI 经模块 SDK 请求
本服务；KC 应用的 typed handler 由 `invocation` 缝调用所属任务服务（API-01），文件任务
挂载独立 `snapshot/service`。根 CLI 仅在
`_test.go` 中保留内部操作登记及嵌入装配。本包负责路由、wire DTO、授权强制与遥测注入。

公开入口：`HTTPHandler`/`HTTPHandlerWithTelemetry`（handler 装配）、
`HTTPHandlerFromConfig`/`HTTPServerOptionsFromFlags`（配置驱动）。cli 的 serve verb 以
组合根身份调用它们（`cli → server` 合法边；cmd/kc 只 import cli）。

## 边界

拥有：HTTP 路由闭集（`routes.go`）、typed handler 与授权强制（B6 已迁入）、遥测注入（B6
已迁入，`serve_facade.go`/`serve_telemetry`）、配对身份（B6 已迁入）、
就绪探测（B7 已迁入：`server/internal/readiness` 持探测/缓存/指纹与证据写探针；search 后端经 `Providers.SearchBackend` 端口注入——探测函数由装配根 `home` 的 `NewSearchBackendProbe` 绑定具体 adapter，本包与 readiness 包都不 import 具体 provider，A-01 守卫 `TestProviderAdaptersImportedOnlyAtAssembly` 钉住）。

不拥有：业务规则与执行器（`object/service`、`discovery/service`、`discovery/datasets`、
`liveaccess/dispatch`、`liveaccess/service`、`snapshot/service` 等所属能力）、装配与 provider
选择（`home`）、模块任务的参数/帮助/产品结果（各模块 `cli`）、全局 argv 与会话（根
`cli`/`client`）、声明本身（Object ②）。请求语义沿所属模块公开合同执行。

## 最小用法

```go
for _, r := range server.Routes() {
	_ = r // 闭集只读；handler 注册由本包内部装配完成（B5–B7 迁入）
}
```

## 插件说明与运行装配

`GET /object/v1/plugins/{name}` 经当前认证主体及 `inventory.read` 返回实际登记的
静态用途、用法和任务入口。它不接受目标或 query 参数，不打开 Home 或构造来源、
Index、Controller；只有授权文件而没有运行布局时也可读取。说明不代表支持或就绪。
根 CLI 的离线帮助与此服务读取分别由所属 CLI/SDK 接入。

access-spec describe 按命名 Dataset 的 `file.read` 和已接受修订的固定来源范围授权，
在原 `AccessPlan` 根字段之外附插件简述和帮助入口，不修改 Schema 返回。
正常服务运行在激活 workers 前由根 Home 执行一次 `VerifyIndexPlugins`；自有连接的
冷管理恢复不探测插件配置。运行视图不随请求或凭据轮换热加载；变更在下一次根装配生效。

Handler 构造保持冷态，不核验自有连接的插件配置或启动维护；配置入口先恢复根 Home。
返回的 `ManagedHTTPHandler.Start()` 显式激活该根：按部署插件配置打开一次实例，绑定后台
服务身份，再启动索引与 Object 维护。`kc serve` 在 `ListenAndServe` 前调用 `Start`；
嵌入式宿主未显式启动时，第一条需要运行资源的 typed 任务激活同一个根，后续任务复用它。
静态插件说明不触发激活。重复 `Start` 不重读配置或重建实例；关闭后不能再启动。

LiveAccess 经同一根的部署插件配置激活。`HTTPServerOptions.LiveAccessPlugins` 提供编译入口，
`StateLookup` 只可作为所选标准入口的嵌入 adapter，不能替代采用配置。
`LiveAccessCredentialOrigins` 或部署的 `liveaccess.credentialOrigins` 固定适用证明目标；
`LiveAccessReadLimits` 或部署的 `lookupTimeout`/`readTimeout` 固定外部取值时限，随每个请求传入。
请求传本次已认证主体、证明及关联。逐值采用、目标限制和可靠原结果留证归 LiveAccess。
独立访问审计支持 `AuditQueryRequest.requestId` 筛选原请求；普通读取不附完整结果证据。

文件读取也进入同一访问账，可按 `traceId`、`requestId` 或两者同时筛选：原始文件保留真实
仓路径、仓身份及固定 commit，Dataset 读取附已接受的 pin；语义投影保留来源快照，合成路径
不作为物理文件记录。每个读取请求（包括每块）单独留证，成功字节必须等证据可靠保存后才交付。
多仓静态对象保留每个实际采用来源的 commit 与方面范围，不计入未采用或无权来源。

`ManagedHTTPHandler.BeginShutdown()` 先拒绝新工作，取消并等待后台维护，保留在途请求
所需资源。`kc serve` 收到退出信号后先调用它，再由 `http.Server.Shutdown` 排空 HTTP，
最后通过 `Close` 释放根 Home 的索引、缓存、来源资源及遥测。嵌入式 HTTP 宿主沿同一顺序
管理退出；直接 `Close` 也复用 `BeginShutdown`，等待在途业务任务后才释放资源。
排空期间保留请求自己的上下文和后续步骤所需资源；资源只释放一次。

## 依赖方向

可以依赖：所属能力的公开合同与任务服务、`home`（transport 打开 Home 装配执行载体）、
`invocation`、`delivery`、`hook`、`identity`、`observability`、`invoke`。
禁止 import：`kc/cli`，由 `TestForbiddenDependencies` 保持。`kc/client` 仅用于已有身份与
管理面共享 wire 形状；其它任务采用所属模块 SDK 合同。实际依赖闭集与既有例外由
`internal/arch` 维护，不通过 B6/B7 的历史路径恢复第二份执行器。

## 验证

认证方式来自 identity 注册表（`local`、`taihu`），装配只在 `serve_handler.go` 经
`identity.Resolve` 完成；本包不按认证方式名字分支，请求认证统一调用 `identity.Authenticate`。
lakeFS 只作存储：原 `--auth lakefs` 已退役，KC 只在认证与授权之后用服务私有凭证访问 lakeFS，
调用方的 Authorization 与 lakeFS 会话 Cookie 都不转发（`TestVerifiedCallerProofIsNeverForwardedToStorage`）。

浏览器请求检查与认证方式无关（`identity.BrowserRequestCheck`）：带 Cookie、或由浏览器经网关
到达的已验证网关身份请求，要求唯一的 `X-Kc-Console: 1`；兼容没有 Fetch Metadata 的客户端，若有
`Sec-Fetch-Site` 则仅接受 `same-origin` 或 `none`，拒绝 `same-site/cross-site`。
非 GET/HEAD/OPTIONS 方法要求 `application/json`。带 Authorization 的请求不视为浏览器环境请求。
KC 不开放跨源 CORS。

授权管理继续使用 `GET/POST /admin/v1/grants` 和
`DELETE /admin/v1/grants/{grant}`，经 CLI/HTTP 共享的 typed 执行与范围授权。
GET 始终返回 `{rules: [...]}`，不返回 `AllowFile` 的初始发权、迁移或准入账本。
可选且不可重复的 query 为 `principal/action/repository/dataset/ref/object/aspect`：
action 按既有语义动作匹配，其余按规则上的值精确过滤，object 过滤不解释 glob。
principal 与 action 同时出现也仍是列表；没有匹配项返回空数组。
未知、重复或编码错误的 query 返回 `USAGE_INVALID`。此入口没有分页参数或游标；
客户端可以对已取得的列表分页，但不能据此声称服务端已提供有界分页。

全量库存要求不带资源约束的 `admin.grants.read`；Repository 或 Dataset
范围管理者须指定对应范围。发权按请求中的目标规则、撤权按耐久规则本身评权，
不接受 caller 替换撤权目标范围。`admin.grants.manage`
不隐含 read。Ref/Object/Aspect 受限的管理规则本轮失败关闭，避免把目标 glob
当成具体对象匹配而扩大委托；完整模式包含关系尚未选定。
无范围 bootstrap `*` 保留原行为，普通发权仍不能创建该 bootstrap 通配符。
新规则使用不可预测的 opaque ID；已存 ID 不迁移。重复旧 ID 撤权返回
`PRECONDITION_FAILED` 并保留账本，未知或已撤销 ID 返回 `USAGE_INVALID`。
可以撤销自身或最后管理规则；再次登录只验证身份，不恢复授权。

`GET /identity/v1/admission` 只返回当前主体的显式规则，不是有效权限全集；
每项保留 `dataset/ref/object/aspect` 等全部现有约束。库存默认发现和部署仓默认
消费声明不在该显式列表中。外部申请地址与管理者名单只提供查权线索，不发权、
不表示名单中的主体持有全局管理范围，也不创建申请或审批队列。
CLI 既有 `grant list --principal ... --action ...` 保留显式规则求值，要求全局
`admin.grants.read`，其结果不包含部署默认政策；scoped read 只可查询其范围库存。
HTTP 没有新增权限评价路由，前端不得把本地匹配当成服务端当前准入决定。

Dataset 管理详情仍使用现有 `GET /snapshot/v1/datasets/{dataset}`。
省略 `revision` 返回当前版；指定正整数只返回已接受的发布记录，不补造缺号。
详情在既有公开摘要之外提供 `sources`、`versions`、`servingRevision`：来源用于
编辑下一版，版本列表用于选择历史，当前服务版用于区别历史与当前。
`versions` 只返回最近的一页已接受版本（页内升序）；`limit` 省略或为零时
默认 50，硬上限 200，负数、无效数字和超上限均返回 `USAGE_INVALID`。
更早的历史经返回的 `continuation` 查询，穷尽时省略该字段；游标绑定
Dataset 和当前服务版，跨范围或发布基点变化后返回 `USAGE_INVALID`，
要求重新打开历史。`revision` 点查仍能选择任意已接受版本，不受当前页限制。
`items` 与 `sources` 即使为空也返回数组。历史记录的退役判定采用当前生命周期；管理可见性
继续走原有 `inventory.read` 授权，详情本身不授予文件消费权限，File Gateway 独立评权。
形状由 `invocation.datasetManagementView` 与
`TestDatasetConsoleManagementHistoryAndByteContract` 钉住，未新增路由或 Client DTO。

`/ui/` 挂载独立的 `snapshot/console.Handler()`；前端源码与嵌入产物分别归属
`snapshot/console/webui/` 和 `snapshot/console/webapp/`。源码、许可与操作入口见
[`Snapshot Store 控制台`](../snapshot/console/README.md) 和
[`接线与构建`](../snapshot/console/webui/KC-VENDOR.md)。先构建前端，再构建使用者二进制。
缺少入口时显示构建提示；已声明页面的深链接回到入口，缺失静态资产保持 404。
组合控制台沿用现有认证系统的用户证明，原 Repo 与 Dataset 请求都进入 Snapshot 当前授权边界。
授权通过后才由服务私有存储凭据调用未修改的私网 lakeFS；原账号管理入口指向现有认证系统。
不把原 API 或任意数据路径直接转发给 native 后端。同源入口与网络边界见上述 vendor 文档。

- `routes_anchor_test.go`：与导出金样 `routes_golden.txt` 双向对账 + 无重复 + 字面表有序；`routes_golden.go` 提供包外读取入口；
- arch：`TestEveryProductionPackageHasAnAllowedLayer`（app 层注册）、
  `TestEveryTopLevelCapabilityHasAReadme`；
- `make check-surface`、`make test-contracts`、`make test`（B5 起承载行为回归）。
