# 调用方认证运输

本包拥有认证与会话存储端口、内存会话、逐请求认证、关联与追踪传播，以及身份发现、whoami 和
浏览器证明交换的 HTTP SDK。运输算法不选择本地配置目录或持久登录，也不组合模块 SDK。

根 [client](../README.md) 保留既有公开类型别名与调用入口，并组合各模块 SDK。
[session](../session/README.md) 通过这些端口实现本地登录和刷新；身份 SDK 不反向依赖会话、
CLI、服务端装配或具体认证 provider。协议和身份约束见 [服务](../../doc/design/service.md)
与 [权限](../../doc/design/permissions.md)。
