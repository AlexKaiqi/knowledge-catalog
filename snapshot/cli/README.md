# snapshot/cli

本包提供 Snapshot Store 的可组合命令适配。`Commands()` 是命令路径、动作、help、操作数、
帮助组与叶子 flag 限制的唯一声明；KC 从该声明装配公开命令表、输入绑定和帮助。
`New(client, Context)` 返回 `Executor`；`ValidateFlags` 只检查参数范围与形状，不打开输入
文件；`Validate` 可在请求前检查具体输入，`Execute` 返回产品结果，由宿主统一编码与展示。

命令包括库存、仓登记与登记历史、Dataset 定义/历史/退役、便携配方合成、交付树 clone、
文件分页与字节下载、显式授权管理。`dataset overlay` 只读写本机配方，不需要 HTTP client。
其余命令只调用 `snapshot/client` 的 typed SDK，不打开本机 Store、服务权威或 KC Home。

`HelpTopics()` 以字符串映射贡献静态概念页和分组引导，返回独立副本；宿主将分组正文
与 `Commands()` 的所属叶组合展示，其余键仅供帮助访问，不登记为业务命令。
它说明 Repository/commit/ref、Dataset/revision/pin、选源输入与原生来源浏览边界，
并指向完整配方、元数据管理与文件消费路径；不查询目标状态、读取输入或建立另一份权威。

宿主提供已认证的 SDK 和当前主体上下文；上下文用于缺省主体的显式规则查询，不是身份
证明或有效权限权威。本包不保存登录、凭证和会话，不复制发布、授权或服务端文件路由实现。
仓供给与连接、恢复、默认政策、部署和跨模块流程仍由宿主负责。

仅依赖 `kernel`、Snapshot 根/具名一级能力和标准库；没有 KC `cli` / `client` / `invocation`
或 `home` 依赖。按仓文件读取显式固定 commit；Dataset 文件范围与版本由服务返回的 pin
固定，每次 SDK 访问仍受当前权限约束。普通对象正文中的文件地址由调用方按业务说明选取，
命令不解释图片类型或自动沿用对象说明的版本。

## 叶子帮助

`Command.Usage` 先给该叶主调用式，再按用途、用法与参数、条件、结果、后果、风险与成本、
失败恢复、示例说明一个实际任务。文本由本包 `help_registry.go`、`help_dataset.go`、`help_files.go`、
`help_access.go` 的私有常量持有，`commands.go` 只引用同源帮助并保留路径、动作与 flags 的
静态声明；宿主无需另写叶子说明。

这是当前命令的静态可供性说明。读到说明不等于当前目标已就绪、已获授权或某插件已装配。
命名发布分别需要 `dataset.manage` 与每来源的 `repository.dataset.publish`；普通读取
不能代替来源许可。来源许可在检查与准备前、最终接受前按当前规则核对，目标 Dataset
的管理者不能用自己的规则管理权自授私有来源。SDK 的显式 commit 按固定坐标求值；
本 CLI 配方选择 ref，再由服务冻结，`baseRev` 仍只核对选择结果。
`Execute(..., {"help": true}, ...)` 返回同源 `Description` 与 `Usage` 的完整展示，不发送
HTTP 请求、不读取配方或规则文件、不写本机目标。实际执行仍按相应服务检查当前范围、
权限和发布依据。

帮助区分三个本机写入保证：单文件下载在完整读取后发布新目标且拒绝覆盖；整树 clone
不承诺整体原子完成，失败可留部分材料；本机 overlay 的 `--out` 可以覆盖已有文件，
写入不提供原子替换。Dataset 发布也明确区分独立配方写回、宿主准备与最终接受，准备
失败不抹去已经完成的配方写回。相应保证由现有实现与行为回归拥有，帮助不另造执行机制。
