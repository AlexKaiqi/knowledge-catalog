# 声明式索引插件

插件名为 `declarative-index`。部署把带版本的能力清单名对应到唯一运行实例。
采用 `kc/index-declaration` 的成员声明检索字段；只有选入 Dataset 的声明生效，Schema 只描述结构。
读取命中保留同一版本、Schema、范围与观察依据。

`kc plugin declarative-index help` 解释写法并列编译默认清单；`describe` 返回实际安装实例的
机器可读清单与摘要。`trial --repo <id> --file <声明文件>` 在固定目标基准上复用 Writer 的
形状和 Schema 定位符校验，再逐成员检查实例支持与重建影响，不发布对象或索引。
试运行不预留之后提交的 CAS，不保证全部物理写入前置条件，也不评估检索效果。
访问概览保留本次固定范围和解释声明的实例/清单名，清单不开放引擎地址、凭据或原始 mapping。

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
已发布代；对象写入成功不等待派生构建。维护只执行当前有效需求，旧工作账不恢复
已经停用的仓或退役 Dataset。源码、所属说明、测试和私有适配都在本模块维护。


当前实际清单只准入物理执行器确实采用的基础构建。`text` 接受至多 8 个输入：每个输入按自己的字段
各建一份文本单元，不带字段的 `--query` 在全部 text 字段上匹配；这不是把多个输入拼成一个字段。
其余 kind 仍为单输入。尚未进入执行器的拼接组合、模板、
同义词、前缀、非默认分词、关键字规范化、向量、缺值策略、截断与权重不会被假报为支持；
对应 kind/option/input 的 `unavailableReason` 或可选值/输入上限给出机器可读边界。
逐成员物理编译、成员增量与完整 Dataset 搜索空间仍须后续主线交付，不能把这些拒绝当作首批设计已完成。
对于形状合法但不支持的声明，trial 仍说明重建影响；例如权重变化本身无需重建，并不证明当前实例已执行权重。
