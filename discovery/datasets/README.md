# 可搜索 Dataset 的准备与采用

`DatasetPublisher` 依次授权、冻结候选、准备它承诺的查询能力、通过 Snapshot 的唯一权威接受新版本。准备或接受失败保留原服务版，不撤销来源提交；纯文件 Dataset 不要求索引。

此专门流程消费 Snapshot Dataset 合同与注入的准备口，Snapshot 不反向依赖 Discovery。导出形状沿 `surface_anchor_test.go`；固定服务版恢复与 Repo HEAD 恢复分别处理。

`Preparation` 对提供逐仓 `ConfiguredFor` 的贡献只准备已配置来源。未配置来源的基本发布
和固定文件/Object 读取独立；不可读或缺实现的配置仍属于必需准备，沿原错误拒绝新发布版。
显式索引准备与查询不会因此获得未配置来源的能力。

`ServingBasisDemand.Bases` 只读取当前未退役 Dataset 的固定来源版本，并去重返回需求。
它不读来源 HEAD、不执行准备；具体插件控制器按其 Repo 分区消费这些依据，分别核验、
恢复 Snapshot 与 State。旧工作账不能重新赋予已撤销的需求。
