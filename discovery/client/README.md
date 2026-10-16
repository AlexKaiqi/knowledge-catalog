# Discovery HTTP SDK

本包拥有查询、关系、遍历、访问概览和插件说明读取的客户端请求与现有 HTTP 路径。调用者注入已经认证的运输；本包不保存会话、不宣告主体、不打开来源、不选择插件或启动维护。

Search 的既有 flat 条件与 typed expression 保持原 wire；客户端不另解析查询语言。Repo/Dataset 坐标复用 Object 客户端的公开范围信封；服务端仍拥有授权、范围与固定版本。插件说明 GET 只传登记名称，结果和访问概览原样交付，不在客户端建立第二份说明类型或修改 Schema。

依赖 Discovery 逻辑合同、Object 坐标、共享 HTTP codec 及标准库；不依赖中央 KC client/CLI、Home、Invocation 或插件实现。


`PluginCapabilities` 通过 `GET /object/v1/plugins/{name}/capabilities` 返回实际实例清单；
`TrialDeclarations` 通过 `POST /object/v1/plugins/{name}/declarations:trial` 发送目标基准及声明材料。
`documents[].path` 只是相对诊断坐标，`content` 才是请求材料；服务不打开调用者文件路径。
试运行结果逐成员区分结构无效、实例不支持和支持，以及固定基准下的重建影响。
两项都不启动维护、不修改对象、Writer 命令账本或索引；服务正常访问/审计证据另按既有规则记录。

生产部署已装配运行根时，这两项仅借用该根；未装配的旧惰性 Handler 明确返回
`PRECONDITION_FAILED`，不会借试运行隐式启动维护。静态 `help` 仍可在没有运行根时读取，
已有运行根时则采用该 Entry 的配置清单。冷开公共阶段的重构另按宿主文件的任务顺序收口。
