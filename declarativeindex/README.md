# 声明式索引插件

插件名为 `declarative-index`。Repo 原生配置选择它的运行实例，Schema 的 `access` 声明
决定可用检索操作。Dataset 沿自己的固定来源使用各仓已选贡献，读取命中时保留同一
版本、Schema、范围与观察依据。帮助通过 `kc plugin declarative-index help` 读取，
Schema JSON 保持知识内容。

| 目录 | 责任 |
|---|---|
| [`plugin`](plugin/README.md) | 构造真实贡献，接入 Repo 分区、资源所有权和固定 Dataset 需求 |
| [`projection`](projection/README.md) | 实际 Index、声明编译、候选检索、固定正文回读及原子派生发布 |
| [`control`](control/README.md) | 当前需求、HEAD 与固定版本独立恢复、通知、失败与启停 |
| [`opensearch`](opensearch/README.md) | 实际物理适配；协议实现同归本模块内部目录 |
| [`cli`](cli/README.md) | 插件命令、参数、输出、帮助和例子 |
| [`client`](client/README.md) | 所属命令的认证传输 |

通用检索值和组合归 `discovery/`，提供方端口、预算和范围合同归
[`discovery/projection`](../discovery/projection/README.md)。查询实现不依赖控制器；
控制器通过实际维护接口调用 Index。Object 的正文缓存及维护保持独立。

接入不证明查询已支持或索引就绪。准备失败保留 Dataset 旧服务版，物理构建失败保留
已发布代；知识写入成功不等待派生构建。维护只执行当前有效需求，旧工作账不恢复
已经停用的仓或退役 Dataset。源码、所属说明、测试和私有适配都在本模块维护。
