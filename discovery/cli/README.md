# Discovery CLI

本包同源拥有 search、relations、traverse、既有 access-spec 概览及统一 plugin 说明读取的命令、参数适配、产品输出与帮助。宿主组合 Commands()、绑定 Operands，注入认证后的 discovery/client.Client 与 Context，再采用 Shape() 做既有产品展示。

Context 的 Dataset/Pin/Definition 由宿主提供；模块不读取全局环境或保存认证。查询只接受一个命名 Dataset 的已发布修订；不带范围、按仓或临时定义的检索拒绝，先从库存选择 Dataset。条件按既有 wire 传入 Server，不另设表达式 parser、授权或固定范围权威。

plugin 的 name/task 是两个位置操作数，task 接受 help、describe、trial：help 通过 Server 读取插件说明，describe 读取已安装实例的能力清单，trial 把声明文件交给 Server 逐成员校验并报告是否支持及是否需要重建。trial 必须明确目标仓与文件，可选择固定 commit 或 ref；校验不写入对象、命令账本或索引，不启动维护。非 UTF-8 文件在传输前拒绝，避免 JSON 编码改变原材料。模块帮助可在 nil client 下本地读取。access-spec 概览原样保留逻辑合同与来源插件信息，不给 Schema JSON 添加运行状态。

不拥有具体索引实现或控制、LiveAccess、Snapshot 文件任务、安装管理、会话或全局命令框架。

搜索输出保留命中的 `basis` 和 `state`。`basis` 是对象层的耐久回读引用，可交给 `kc read --basis`；
它绑定本次 Dataset 完整候选来源与解释依赖，不从命中的实际贡献者反推范围，也不延长查询或续页
有效期。`state` 只描述完整所选 Unit 的本地状态，既有 `version`、索引原观察与 READ 观察仍分别保留。
