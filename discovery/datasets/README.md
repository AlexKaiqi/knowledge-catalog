# 可搜索 Dataset 的准备与采用

`DatasetPublisher` 依次授权、冻结候选、准备它承诺的查询能力、通过 Snapshot 的唯一权威接受新版本。准备或接受失败保留原服务版，不撤销来源提交；纯文件 Dataset 不要求索引。

此专门流程消费 Snapshot Dataset 合同与注入的准备口，Snapshot 不反向依赖 Discovery。导出形状沿 `surface_anchor_test.go`；固定服务版恢复与 Repo HEAD 恢复分别处理。

`Preparation` 先做对象身份检查：候选有两个及以上来源仓时，范围内同一对象的同一方面
（含 Schema 绑定方面）来自两个来源仓，以 `OBJECT_ID_CONFLICT` 拒绝，指出对象、方面与来源；
检查由装配方经 `Conflicts` 注入（组合根 `home` 以 `reader.Serving.CheckHoldingConflicts` 装配，Dataset 解析到读取依据的唯一转换是 `home.DatasetPin`），不依赖插件配置；多来源而未注入时拒绝而不跳过。范围按 Dataset 读取的整对象规则判定。

`Preparation` 在冲突检查之前经必填的 `Dependencies` 冻结引用依赖：选入单元引用的取值描述不在任何
选入仓时，找到唯一持有它的仓并把其当时的提交记入 `Dataset.Dependencies`（组合根以
`reader.Serving.FreezeDependencies` 装配）；无持有者为 `OBJECT_REF_UNRESOLVED`，多个为
`OBJECT_ID_CONFLICT`，未注入为 `CAPABILITY_UNSATISFIED`。`Execute` 返回带依赖的候选；`DatasetPublisher`
只接受依赖的变化，SetID、修订与来源被改写时以 `PRECONDITION_FAILED` 拒绝。

`Preparation.PrepareDataset` 把包含范围、来源版本和引用依赖的固定候选一次交给注入的准备口；
组合根用 `home.DatasetPin` 转换并调用实例的 Dataset corpus 准备，不在发布执行器重复解释读取范围。
仅逐仓贡献沿既有 `Provider` 路径；对提供 `ConfiguredFor` 的贡献只准备已配置来源。未配置来源的基本发布
和固定文件/Object 读取独立；不可读或缺实现的配置仍属于必需准备，沿原错误拒绝新发布版。
显式索引准备与查询不会因此获得未配置来源的能力。

`ServingBasisDemand.Bases` 只读取当前未退役 Dataset 的固定来源版本，并去重返回需求。
它不读来源 HEAD、不执行准备；具体插件控制器按其 Repo 分区消费这些依据，分别核验、
恢复 Snapshot 与 State。旧工作账不能重新赋予已撤销的需求。
