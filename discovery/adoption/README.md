# 插件共同采用合同

本包只拥有部署插件配置、作用域与说明事实，依赖 kernel 的固定来源坐标；
不登记索引或访问实现，不打开资源，不启动 worker。实际贡献消费仍分别在
`discovery/plugin` 和 `liveaccess/service`。

`PluginsConfig` 按插件名列出部署安装的实例，`PluginInstanceConfig` 给出实例解释什么：
索引实例的 `Catalog`（能力清单名），LiveAccess 实例的 `Protocol` 与可选 `Runtimes`。
没有出现在配置里的插件就是没装：它的声明是普通内容，请求明确报没有能力。
Repo 与 Dataset 都不选择实例。`Validate` 拒绝未编译的插件、空实例集与非法名称；
`CheckCatalogs` 是“一个清单名只对应一个实例”的唯一定义处，`Validate` 与索引消费侧
都调用它。`PluginSelection` 只是“哪个实例解释”的名称对，不是选择配置。
本包不定义默认插件、优先级或热加载；编译默认由宿主给出。

`PluginScope` 的 Name、Instance、Repositories、Own 由根装配给出。宿主可提供并发安全的
`ContainsRepository` 判定当前接纳的来源；它存在时以当前结果为准，未提供时采用构造时的
Repositories 固定范围。`RepositoryInventory` 给维护提供当前来源的内存快照；没有此端口时采用初始列表。宿主同时提供当前成员判定与库存时，两者来自同一接纳集合。成员判定不代替调用者授权，也不改变实例的部署配置。取得资源后立即
登记释放责任，实际释放与并发调用归各能力消费侧。`PluginAffordance` 和 `SourcePlugin`
只是静态说明与已获准固定来源的解释事实，不表示权限、支持或就绪。

`discovery/plugin` 保留同名公开别名，类型身份及 JSON 形状一致；LiveAccess 直接使用
本合同，不因共享配置事实而依赖索引登记运行时。
