# qinghe-knowledge-published

各仓通过 writer commit 发布清河茶铺知识。table-meta 包含库、表、列、作业与血缘；sales-semantic 包含模型、维度、度量与指标。节点 runtime 声明启动外部资源访问服务，绑定 table stats 的实时 MySQL 计数与只读 SQL 调用。原有业务回读、绑定、实时计数与 SQL 断言都保留在 construct。

两个独立用例消费这个前态：probe-sync-projections-and-search.feature 同步两仓投影并检索表、列、模型和指标；probe-publish-dataset-with-scoped-members.feature 发布限定 tables、semantic-models、metrics 路径的 qinghe-sales 并回读，作业仍被排除。Dataset 定义和 READ 不依赖 SEARCH 投影，两个用例互不继承临时后态。

人工走查由 goto.py 的明确 probe 入口执行相应用例；只重放到本状态时尚未发布 qinghe-sales 或构建检索投影。
