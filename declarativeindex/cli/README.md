# 声明式索引 CLI

本包提供既有 operations projection describe/sync 的命令、参数、输出和帮助。宿主装配 Commands()、注入认证后的 declarativeindex/client.Client 并采用 Shape()，不复制叶规则。插件静态说明与能力清单归 [`declarativeindex/capability`](../capability/README.md)。

本包不依赖任务服务、真实索引或访问实现，独立 CLI 的依赖闭包由架构守卫验证。

本包不持 Index、Controller、来源、后台任务或部署配置。静态任务帮助在 nil client 下本地读取；维护执行经 Server 进入原 typed 任务。统一 kc plugin <名称> help 由 Discovery 客户端读取服务登记说明，不另建插件业务命令树或安装管理。

投影运维只接受命名 Dataset 与可选修订。describe 按该 Dataset 的完整 file.read 准入，sync 要求 dataset.manage；公开仓通知已退役，来源内部通知与对账合同保留。结果的 dataset/revision 与 members 中真实来源的 repository/commit 是权威依据，snapshot/state 是一份 Dataset 派生投影的诊断事实，不把内部语料标识当作来源仓或来源提交。
