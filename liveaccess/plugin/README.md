# LiveAccess 插件入口

`Entry(Options)` 登记静态说明和真实取值构造。默认使用 `HTTPStateLookup`，请求的固定
Schema/Binding 决定来源地址。嵌入方可用 `OpenLookup(PluginScope)` 构造兼容 adapter；
每取得资源便通过 `scope.Own` 登记释放责任，实例与 Repo 范围来自已有仓配置。默认
adapter 的凭证适用目标由 `CredentialOrigins` 指定，知识声明不能自行扩充可信目标。

取值是必需贡献；源操作及已有 InterfaceRegistry 为可选贡献，不要求只读插件实现写入。
消费登记及逐仓选择归 `liveaccess/service`。其 `Close` 拒绝新调用、等待已接受调用后
逆序释放资源一次。单个实例构造失败会立即释放其部分资源，并为相关 Repo 保存原故障；
独立实例仍可登记。没有后台维护 worker。

静态说明来自 `liveaccess/cli`；读取 Entry 的 Affordance 不执行 Open、读取仓配置或访问来源。
登记不发权；固定声明、当前请求授权和结果核验仍沿原消费合同。
