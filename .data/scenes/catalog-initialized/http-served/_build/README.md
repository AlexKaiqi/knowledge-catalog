# http-served

从父部署启动测试用 typed HTTP Server。匿名与已认证身份请求、login/whoami/logout 会话、admission 查询三个独立用例复用可用 Server 地址；每个用例的服务实例与客户端会话仍分别隔离。

正式 Server 发布发现、认证方式和访问证据的具名 Go 测试使用各自 setup。仅观察 access/hitmap 来源的 embedded 用例挂在 catalog-initialized，不要求 HTTP 前态。
