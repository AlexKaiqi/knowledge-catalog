# 插件共同采用合同

本包只拥有既有 Repo 插件配置、选择、作用域与说明事实，依赖 kernel 的固定来源坐标；
不登记索引或访问实现，不打开资源，不启动 worker。实际贡献消费仍分别在
`discovery/plugin` 和 `liveaccess/service`。

`RepositoryPluginsKey`、`PluginSelection`、`RepositoryPluginConfig`、
`RepositoryPluginConfigs` 与 `ParseRepositoryPlugins` 保持原有 metadata 合同：
缺键与显式空数组表示无扩展；非法或不可读配置保存明确故障，不冒充未启用。
`SelectRepositoryPlugins` 从同一已读配置分出当前带类型消费侧的选择；另一种已知能力
不启用本能力，未知入口仍是该来源的故障。它不定义默认插件、优先级或热加载。

`PluginScope` 的 Name、Instance、Repositories、Own 仍由根装配给出；取得资源后立即
登记释放责任，实际释放与并发调用归各能力消费侧。`PluginAffordance` 和 `SourcePlugin`
只是静态说明与已获准固定来源的采用事实，不表示权限、支持或就绪。

`discovery/plugin` 保留全部旧同名公开别名与解析/分流薄函数，类型身份及 JSON 形状
一致；LiveAccess 直接使用本合同，不因共享配置事实而依赖索引登记运行时。
