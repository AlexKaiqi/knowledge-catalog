# server/

服务接入面：method+pattern 路由闭集、typed handler 注册、授权强制与遥测的宿主。
CAP-ORG-01 B4 先立契约（`routes.go` 与 `httpsurface.Patterns()` 双向对账）；B6 把 cli 的
serve/service_routes/handlers/授权/遥测/就绪探测搬入（B6/B7）。CLI 与 HTTP 继续运行同一批
typed application executor（API-01）——动词操作体在 `invocation` 缝（见该包 README），
本包负责 transport 形状：路由、wire DTO、授权强制、遥测注入。

公开入口：`HTTPHandler`/`HTTPHandlerWithTelemetry`（handler 装配）、
`HTTPHandlerFromConfig`/`HTTPServerOptionsFromFlags`（配置驱动）。cli 的 serve verb 以
组合根身份调用它们（`cli → server` 合法边；cmd/kc 只 import cli）。

## 边界

拥有：HTTP 路由闭集（`routes.go`）、typed handler 与授权强制（B6 已迁入）、遥测注入（B6
已迁入，`serve_facade.go`/`serve_telemetry`）、gitea/配对身份（B6 已迁入）、
就绪探测（B7 已迁入：`server/internal/readiness` 持探测/缓存/指纹与证据写探针；search 后端经 `Providers.SearchBackend` 端口注入——探测函数由装配根 `home` 的 `NewSearchBackendProbe` 绑定具体 adapter，本包与 readiness 包都不 import 具体 provider，A-01 守卫 `TestProviderAdaptersImportedOnlyAtAssembly` 钉住）。

不拥有：业务规则与执行器（`knowledgeapp`）、装配与 provider 选择（`home`）、argv 词汇与
产品输出（`cli` / `invoke`）、wire 协议语义（`client`）、声明本身（Knowledge ②）。

## 最小用法

```go
for _, r := range server.Routes() {
	_ = r // 闭集只读；handler 注册由本包内部装配完成（B5–B7 迁入）
}
```

## 依赖方向

可以依赖：`knowledgeapp`、`home`（B6 起：transport 打开 Home 装配执行载体）、`invocation`、
`delivery`、`gate`、`hook`、`identity`、`observability`、`controlplane`、`invoke`（§3.3）。
禁止 import：`kc/cli`（B6 已落 denylist，`TestForbiddenDependencies`）。`kc/client` 暂允许，
仅作为 B6 前既有 wire-shape 家（sharing/connection 负载：本地面填充 typed client 解码的同一
批 JSON 形状）；该归属必须在 B7 重新裁决（httpsurface 升级为共享 DTO 家或 server 自持形状），
在此之前不得新增形状进 `client`。

## 验证

控制台同源组合可使用 `--auth lakefs --auth-url <原 lakeFS origin>`（配置为
`auth: lakefs` 与 `authURL`），逐请求向固定 `/api/v1/user` 验证用户原会话或
Basic/Bearer proof；不使用 Snapshot adapter 的服务凭证，不继承 lakeFS Admin。
`GET /identity/v1/auth` 报告 `mode: lakefs`、`accepts: [Cookie, Authorization]`。
Cookie 仅接受唯一的 `internal_auth_session`、`oidc_auth_session` 或
`saml_auth_session`；其它 Cookie 不转发，Authorization 与任意 Cookie 混用拒绝。
Cookie 请求要求 `X-Kc-Console: 1`；兼容没有 Fetch Metadata 的客户端，若有
`Sec-Fetch-Site` 则仅接受 `same-origin` 或 `none`，拒绝 `same-site/cross-site`。
非 GET/HEAD/OPTIONS 方法要求 `application/json`。KC 不开放跨源 CORS。
验证器不带服务端 cookie jar、不追重定向，仅接受 HTTP 200 和有界的单个用户 JSON。

经验证的原 `user.id` 必须满足既有 KC 规范用户名合同，不做大小写/邮箱别名转换；
不兼容的有效账号返回 `FORBIDDEN` 并要求显式身份关联，当前未提供该关联功能。
缺失 ID 或非正整数 `creation_date` 失败。`identity.Bind` 耐久绑定
provider/issuer/`id:creation_date`，创建时间变化的同名账号不能继承旧授权。
原 [v1.58 用户 API](https://github.com/treeverse/lakeFS/blob/v1.58.0/api/swagger.yml)
只暴露秒精度创建时间，同名同秒重建无法据此区分；部署必须显式处理账号回收与旧授权，
当前接线不宣称完整身份生命周期的不可重派保证。KC Catalog/Dataset 授权仍单独判定。

授权管理继续使用 `GET/POST /admin/v1/grants` 和
`DELETE /admin/v1/grants/{grant}`，经 CLI/HTTP 共享的 typed 执行与范围授权。
GET 始终返回 `{rules: [...]}`，不返回 `AllowFile` 的初始发权、迁移或准入账本。
可选且不可重复的 query 为 `principal/action/repository/catalog/dataset/ref/object/aspect`：
action 按既有语义动作匹配，其余按规则上的值精确过滤，object 过滤不解释 glob。
principal 与 action 同时出现也仍是列表；没有匹配项返回空数组。
未知、重复或编码错误的 query 返回 `USAGE_INVALID`。此入口没有分页参数或游标；
客户端可以对已取得的列表分页，但不能据此声称服务端已提供有界分页。

全量库存要求不带资源约束的 `admin.grants.read`；Repository、Catalog 或 Dataset
范围管理者须指定对应范围。发权按请求中的目标规则、撤权按耐久规则本身评权，
不采用首个 Catalog，也不接受 caller 替换撤权目标范围。`admin.grants.manage`
不隐含 read。Ref/Object/Aspect 受限的管理规则本轮失败关闭，避免把目标 glob
当成具体对象匹配而扩大委托；完整模式包含关系尚未选定。
无范围 bootstrap `*` 保留原行为，普通发权仍不能创建该 bootstrap 通配符。
新规则使用不可预测的 opaque ID；已存 ID 不迁移。重复旧 ID 撤权返回
`PRECONDITION_FAILED` 并保留账本，未知或已撤销 ID 返回 `USAGE_INVALID`。
可以撤销自身或最后管理规则；再次登录只验证身份，不恢复授权。

`GET /identity/v1/admission` 只返回当前主体的显式规则，不是有效权限全集；
每项保留 `dataset/ref/object/aspect` 等全部现有约束。公开 Catalog 和部署仓默认
消费声明不在该显式列表中。外部申请地址与管理者名单只提供查权线索，不发权、
不表示名单中的主体持有全局管理范围，也不创建申请或审批队列。
CLI 既有 `grant list --principal ... --action ...` 保留显式规则求值，要求全局
`admin.grants.read`，其结果不包含部署默认政策；scoped read 只可查询其范围库存。
HTTP 没有新增权限评价路由，前端不得把本地匹配当成服务端当前准入决定。

Dataset 管理详情仍使用现有 `GET /catalog/v1/catalogs/{catalog}/datasets/{dataset}`。
省略 `revision` 返回当前版；指定正整数只返回已接受的发布记录，不补造缺号。
详情在既有公开摘要之外提供 `sources`、`versions`、`servingRevision`：来源用于
编辑下一版，版本列表用于选择历史，当前服务版用于区别历史与当前。
`versions` 只返回最近的一页已接受版本（页内升序）；`limit` 省略或为零时
默认 50，硬上限 200，负数、无效数字和超上限均返回 `USAGE_INVALID`。
更早的历史经返回的 `continuation` 查询，穷尽时省略该字段；游标绑定
Catalog、Dataset 和当前服务版，跨范围或发布基点变化后返回 `USAGE_INVALID`，
要求重新打开历史。`revision` 点查仍能选择任意已接受版本，不受当前页限制。
`items` 与 `sources` 即使为空也返回数组。历史记录的退役判定采用当前生命周期；管理可见性
继续走原有 `catalog.read` 授权，详情本身不授予文件消费权限，File Gateway 独立评权。
形状由 `invocation.datasetManagementView` 与
`TestDatasetConsoleManagementHistoryAndByteContract` 钉住，未新增路由或 Client DTO。

`/ui/` 嵌入本包的 `webapp/`，由 `webui` 构建生成；源码、许可与操作入口见
[`webui/KC-VENDOR.md`](../webui/KC-VENDOR.md)。先构建前端，再构建 Server。
缺少入口时显示构建提示；已声明页面的深链接回到入口，缺失静态资产保持 404。
部署入口直接转发原页面的 API 与 S3 请求给已有 lakeFS 后端，Dataset 请求给 KC；
同源分流配置见上述 vendor 文档，原页面使用 lakeFS 用户凭证。

- `routes_anchor_test.go`：与 `httpsurface.Patterns()` 双向对账 + 无重复 + 字面表有序；
- arch：`TestEveryProductionPackageHasAnAllowedLayer`（app 层注册）、
  `TestEveryTopLevelCapabilityHasAReadme`；
- `make check-surface`、`make test-contracts`、`make test`（B5 起承载行为回归）。
