# Object CLI

本包是 Object Affordance 的唯一定义：命令、参数校验、位置参数绑定、帮助、任务执行和产品结果视图。宿主组合 `Commands()` 与本地 `ModelHelp()`，调用 `BindOperands()`，注入认证后的 `object/client.Client` 和可选 Dataset 默认值，再做通用 JSON 编码。HTTP 与 CLI 业务规则仍由 Object 服务和 Writer 执行。

目标依据 `docs/reviewed/object-refactor-guideline.md` 的 U1～U12 与 Object CLI 合同：从根帮助理解模型，统一引用进入读写，保留固定版本、维护 digest 和实际来源，独立于插件采用。读定位与维护粒度分离；公共引用完整传入服务端，CLI 不按 locator 深度生成写 Address。

不负责目录 desired 对照或批量出版、Dataset 发布、Discovery、LiveAccess、治理与认证会话。否决将叶规则复制到中央宿主、导入 invocation/home 复用完整应用、引入通用插件命令框架。对象专属扩展也由本包贡献。

`read`、`writer put/remove` 的目标只用位置参数传入对象 URN＋locator，不接受 `--reference` 或旧的 `--repo/--object/--aspect/--member/--include/--exclude`。`BindOperands()` 按模块声明绑定位置参数，使用私有类型保留其来源；`ValidateFlags()` 拒绝另行提供的目标 flag，执行器只将完整引用字符串写入 SDK 请求。不适配旧输入，也不拼装另一套 Address。读取保留显式 Dataset 消费范围及 commit/ref 版本限定；写入保留维护类型、Schema、来源与并发条件。读取选择由根 `object.Reference` 和 Reader 决定，写入维护范围由服务端与 Writer 决定。所有业务请求经 Server 和显式认证传输；帮助可本地使用。

读取输出保留统一引用、实际 locator、维护坐标、摘要和逐单元来源依据。逻辑 State 结果保留 observations，来源观察不冒充固定知识正文或可写摘要。Schema describe 的完整 Definition 在 Schema 项内，不混入读取信封。resolve、log、provenance 与 Schema 命令按各自既有任务定义输入；本轮不改这些任务。
