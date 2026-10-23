# 声明式索引 CLI

本包提供 `kc index`、`kc index prepare`、`kc index describe` 的命令、参数、输出和帮助。宿主装配 Commands()、注入认证后的 declarativeindex/client.Client 并采用 Shape()，不复制叶规则。插件静态说明与能力清单归 [`declarativeindex/capability`](../capability/README.md)。

本包不依赖任务服务、真实索引或访问实现，独立 CLI 的依赖闭包由架构守卫验证。

本包不持 Index、Controller、来源、后台任务或部署配置。静态任务帮助在 nil client 下本地读取；维护执行经 Server 进入原 typed 任务。统一 kc plugin <名称> help 由 Discovery 客户端读取服务登记说明，不另建插件业务命令树或安装管理。

索引运维只接受命名 Dataset 与可选修订。三条命令按用途分工：`kc index` 回答"这个修订能不能搜"，`kc index describe` 回答"没就绪卡在哪"，`kc index prepare` 是显式重试入口。`index` 与 `index describe` 按该 Dataset 的完整 file.read 准入，`index prepare` 要求 `index.manage`；公开仓通知已退役，来源内部通知与对账合同保留。

`index prepare` 与 `index describe` 共用一份报告：`members` 给每个来源的 repository/commit，`basis` 给该修订冻结的语料（对象数、逻辑声明摘要、物理摘要、代、覆盖、模式、编译字段），`ready` 给就绪答案。`index prepare` 另外带上 `observation`，即本次刷新发布的观察代；`index describe` 另外带上 `failure`，即上次构建停止的原因。报告里没有来源仓坐标：Dataset pin 才是权威依据，不把内部语料标识当作来源仓或来源提交。
