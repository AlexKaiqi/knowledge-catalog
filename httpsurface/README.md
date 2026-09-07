# httpsurface/

公开 HTTP 路由表的闭集：只含 method + pattern。

typed handler 在 `cli/`，调用 Server 内的应用操作；业务 CLI 通过 typed Client 访问这些路由。本包不得 import `cli`、`client`、`home` 或协议层。生产 CLI argv 表不得 import 本包；测试可以把 `Patterns()` 与 mux 对账。加一条 CLI 命令不能自动长出 HTTP 路由。

`POST /catalog/v1/catalogs/{catalog}/repositories:create` 是平台托管仓的申请资源，接受 `repository` 与 `commandId`，执行独立的 `catalog.repositories.create` 授权。原 `POST …/repositories` 仍是既有源接入；两者不得通过请求里的 driver、路径或身份字段互相切换。
