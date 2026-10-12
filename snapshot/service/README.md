# snapshot/service/

Snapshot Store 的独立 HTTP 适配：文件 Dataset 的定义、固定来源解析、发布历史和原始
字节交付，以及同一显式授权账的自查与管理。完整控制台由相邻 `snapshot/console` 提供。

## 边界

核心调用 `snapshot/dataset`、`snapshot/access` 和 `snapshot.Store` 的文件能力。
来源坐标是 Repository、commit 与文件路径/目录前缀；文件内容始终是不解释的字节，
不以 Knowledge `object_id` 定位来源。公开类型仍保留历史名称，迁移不改变 wire 形状。

调用方通过 `Config.Acquire` 借出一次请求的 `Resources`，并负责释放租约、关闭顺序和
授权账的原子持久化。`CatalogProvider` 打开同一登记权威，`Sources` 提供已打开的文件仓，
`access.RuleStore` 对授权检查与更新提供同一串行边界；不得以客户端断言代替已验证身份。
服务不提供第二份 Dataset、授权或身份账，不要求 KC Home、Invocation、Knowledge 或索引。

`IdentityOptions` 使用显式配置的验证器；未配置不会自动接受 local 身份。
原 lakeFS 登录证明由 `NewLakeFSAuthenticator` 向固定的 native origin 校验。使用者先显式
初始化通用 `identity` 绑定权威，再以 `identity.WrapAuthenticator` 包装验证器。
lakeFS native 管理员身份不产生本模块的管理授权；退出也不删除耐久身份绑定或恢复 grant。
native 用户身份的秒级代际及兼容用户名限制与原接线相同，见控制台 API review 和 TASK 登记。

## 最小用法

```go
import (
    "net/http"
    "kc/snapshot/service"
)

// config 提供已配置的验证器、Catalog/文件仓及持久授权账。
func mountSnapshot(mux *http.ServeMux, config service.Config) {
    service.NewHandler(config).Register(mux)
}
```

`Register` 挂载已有身份发现/自查、授权规则、Catalog/来源库存、Dataset 和文件交付路由，
以及 `/ui/` 静态控制台。路由和 DTO 由公开 Go 类型、实现与回归拥有。
控制台仍使用原 lakeFS API；同源网关需要将 native API、登录/退出和 S3 路径交给真实 lakeFS，
将本模块 API 交给此 handler。完整可审阅配置见
[控制台接线](../console/webui/KC-VENDOR.md)，模块不复刻 lakeFS 后端。

## 外部扩展与依赖方向

KC Server 通过窄回调提供既有默认政策、便携 recipe 准备、Knowledge 准入、语义投影及观察
证据。独立文件服务不需要这些回调。发布仍由 Dataset 核心执行，授权先于领域操作；
`Around` 只可拒绝前置执行或观察真实结果，不能重复执行、伪造发布或覆盖领域错误。
来源库存的外部描述不能更改登记身份。请求元数据只供诊断，不能用于发权。

允许依赖 Snapshot 子包、通用 `identity` 与 `kernel`；禁止直接或传递依赖 `catalog`、
`home`、`invocation`、`invoke`、`server`、`cli`、`client`、Knowledge、Index、Retrieval。
语义文件视图的解释和应用生命周期留在挂载方，原始文件算法只在本模块拥有。

## 验证

`snapshot/service` 回归覆盖身份/CSRF、完整 HTTP 形状、原子授权管理、固定文件读取、
真实历史、范围拒绝、分页及 hooks 至多一次执行；Server 的并发关闭测试通过本模块真实
HTTP 路由借用资源。架构守卫
`TestSnapshotStoreFileModuleHasIndependentOwners` 与
`TestLegacyCatalogDoesNotRetainDatasetAuthority` 检查所有权及依赖方向。
包内夹具回归不等于真实 lakeFS 验收；独立部署与浏览器证据单独记录。

`GET /catalog/v1/catalogs/{catalog}/repositories` 是控制台发布页的来源选择入口。
通过 `catalog.read` 后，只返回该 Catalog 权威已登记的仓身份，维持已有
`catalogId` / `repositories: [{id, ...}]` 响应；不枚举全部已连接 Store，不读取来源字节。
库存可见不产生来源消费或 Dataset 发布权。`Resources.DescribeRepository` 可为一个
已选定来源添加应用拥有的可选元数据（KC 沿用既有 schema count）；该扩展不能修改仓 ID，
默认模块只返回 `{id}`，库存与文件发布均不要求 Knowledge 或索引实现。
