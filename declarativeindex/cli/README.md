# 声明式索引 CLI

本包提供真实 declarative-index 插件可复用的静态 Affordance，以及既有 operations projection describe/sync/notice 的命令、参数、输出和帮助。插件入口登记 Affordance()；宿主装配 Commands()、注入认证后的 declarativeindex/client.Client 并采用 Shape()，不复制叶规则。

`Affordance()` 返回本包的纯数据 `PluginAffordance`：Name、Summary、Usage、Tasks。
它与说明文字、任务入口同源；插件 Entry 映射到消费方的登记合同，不复制内容。
本包不依赖任务服务、真实索引或访问实现，独立 CLI 的依赖闭包由架构守卫验证。

本包不持 Index、Controller、来源、后台任务或部署配置。静态任务帮助在 nil client 下本地读取；维护执行经 Server 进入原 typed 任务。统一 kc plugin <名称> help 由 Discovery 客户端读取服务登记说明，不另建插件业务命令树或安装管理。
