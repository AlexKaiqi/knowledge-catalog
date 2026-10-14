# Discovery CLI

本包同源拥有 search、relations、traverse、既有 access-spec 概览及统一 plugin 说明读取的命令、参数适配、产品输出与帮助。宿主组合 Commands()、绑定 Operands，注入认证后的 discovery/client.Client 与 Context，再采用 Shape() 做既有产品展示。

Context 的 Dataset/Pin/Definition 由宿主提供；模块不读取全局环境或保存认证。裸 search 的库存发现由宿主先经既有 Server 合同固定范围再注入 Context，InventorySearchRequested 只识别该任务。条件按既有 wire 传入 Server，不另设表达式 parser、授权或固定范围权威。

plugin 的 name/task 是两个位置操作数，task 仅接受 help；通过 Server 读取登记说明，不运行实现、来源或维护。模块帮助可在 nil client 下本地读取。access-spec 概览原样保留逻辑合同与来源插件信息，不给 Schema JSON 添加运行状态。

不拥有具体索引实现或控制、LiveAccess、Snapshot 文件任务、安装管理、会话或全局命令框架。
