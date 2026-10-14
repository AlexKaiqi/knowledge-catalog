# Discovery HTTP SDK

本包拥有查询、关系、遍历、访问概览和插件说明读取的客户端请求与现有 HTTP 路径。调用者注入已经认证的运输；本包不保存会话、不宣告主体、不打开来源、不选择插件或启动维护。

Search 的既有 flat 条件与 typed expression 保持原 wire；客户端不另解析查询语言。Repo/Dataset 坐标复用 Object 客户端的公开范围信封；服务端仍拥有授权、范围与固定版本。插件说明 GET 只传登记名称，结果和访问概览原样交付，不在客户端建立第二份说明类型或修改 Schema。

依赖 Discovery 逻辑合同、Object 坐标、共享 HTTP codec 及标准库；不依赖中央 KC client/CLI、Home、Invocation 或插件实现。
