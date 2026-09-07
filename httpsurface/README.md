# httpsurface/

公开 HTTP 路由表的闭集：只含 method + pattern。

typed handler 在 `cli/`，调用与 local CLI 同一组应用操作。本包不得 import `cli`、`client`、`home` 或协议层。生产 CLI argv 表不得 import 本包；测试可以把 `Patterns()` 与 mux 对账。加一条 CLI 命令不能自动长出 HTTP 路由。
