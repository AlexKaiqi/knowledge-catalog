# LiveAccess HTTP SDK

`New(Transport)` 采用已有认证 HTTP transport；模块不解析凭证、选择 Server 或重建登录会话。
`ResolveBinding` 与 `AccessResource` 分别使用既有 bindings:resolve 和 resources:access 路由，
请求沿 `object/client.KnowledgeScope` 的 Repo/固定版本或 Dataset 依据。
`BindingRequest`、`AccessRequest` 及响应解码归本模块；根 SDK 提供薄转发与 `LiveAccessService()`。

业务读取由 Object SDK 发起，服务端采用 LiveAccess；完整原结果仍从独立审计入口授权查询。
