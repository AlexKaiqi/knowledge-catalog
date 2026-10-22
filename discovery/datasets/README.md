# 可搜索 Dataset 的准备与采用

`DatasetPublisher` 依次授权、冻结候选、准备它承诺的查询能力、通过 Snapshot 的唯一权威接受新版本。
准备失败或权威明确拒绝时保留原服务版，不撤销来源提交；结果未知不能据错误推断从未接受。
纯文件 Dataset 不要求索引。

`PrepareResources` 路径在授权后、冻结和分配准备资源前检查权威的 `PublicationCoordinator`。
准备返回固定候选和 `PublicationProtection`，纯文件或无托管资源的提供方可返回空票据。
实际票据须在权威 CAS 前，以原固定候选和实际权威基点取得耐久准入；成功接受后确认修订引用。
准入一旦尝试，错误也可能代表响应丢失，执行器不能取消保护；接受失败或确认失败均保留给权威恢复。
仅准备失败、候选被改写或准入前取消等明确尚未尝试准入的路径可以取消准备票据。
所有出口关闭本地句柄，关闭不解除耐久保护，也不让请求持有的插件实例借用泄漏。
旧 `Prepare` 路径保留原有纯文件组合合同；同时指定两条准备路径会拒绝。

此专门流程消费 Snapshot Dataset 合同与注入的准备口，Snapshot 不反向依赖 Discovery。导出形状沿 `surface_anchor_test.go`；固定服务版恢复与 Repo HEAD 恢复分别处理。

`Preparation` 先做对象身份检查：候选有两个及以上来源仓时，范围内同一对象的同一方面
（含 Schema 绑定方面）来自两个来源仓，以 `OBJECT_ID_CONFLICT` 拒绝，指出对象、方面与来源；
检查由装配方经 `Conflicts` 注入（组合根 `home` 以 `reader.Serving.CheckHoldingConflicts` 装配，Dataset 解析到读取依据的唯一转换是 `home.DatasetPin`），不依赖插件配置；多来源而未注入时拒绝而不跳过。范围按 Dataset 读取的整对象规则判定。

`Preparation` 在冲突检查之前经必填的 `Dependencies` 冻结引用依赖：选入单元引用的取值描述不在任何
选入仓时，找到唯一持有它的仓，并把用于证明唯一性的全部候选提交（包括未贡献取值描述的仓）
记入 `Dataset.Dependencies`（组合根以
`reader.Serving.FreezeDependencies` 装配）；无持有者为 `OBJECT_REF_UNRESOLVED`，多个为
`OBJECT_ID_CONFLICT`，未注入为 `CAPABILITY_UNSATISFIED`。`Execute` 返回带依赖的候选；`DatasetPublisher`
只接受依赖的变化；其余完整候选在回调之前固定摘要，SetID、修订、来源、选入条目、metadata 或退役状态
被改写都以 `PRECONDITION_FAILED` 拒绝，包括通过共享切片、map 或路径指针原地修改的情况。

`Preparation.PrepareDataset` 把包含范围、来源版本和引用依赖的固定候选一次交给注入的准备口；
组合根用 `home.DatasetPin` 转换并调用实例的 Dataset corpus 准备，不在发布执行器重复解释读取范围。
仅逐仓贡献沿既有 `Provider` 路径；对提供 `ConfiguredFor` 的贡献只准备已配置来源。未配置来源的基本发布
和固定文件/Object 读取独立；不可读或缺实现的配置仍属于必需准备，沿原错误拒绝新发布版。
显式索引准备与查询不会因此获得未配置来源的能力。

`ServingBasisDemand.Bases` 只读取当前未退役 Dataset 的固定来源版本，并去重返回需求。
它不读来源 HEAD、不执行准备；具体插件控制器按其 Repo 分区消费这些依据，分别核验、
恢复 Snapshot 与 State。旧工作账不能重新赋予已撤销的需求。

`Preparation.ExecuteProtected` 复用同一依赖冻结和冲突检查，再优先调用 `PrepareProtectedDataset`
连续完成构建与保护；Home 接入该路径，返回选定实例在构建中已登记的原票据。
未配置该回调的既有调用者仍沿 `PrepareDataset`/`ProtectDataset` 合同准备并取得已有资源保护。
Home 将选定实例的资源票据适配为本执行器消费的 `PublicationProtection`；
本包不导入具体投影提供方，不推断物理生成代。准备方返回错误时须自行清理，或同时返回已取得的票据
供执行器取消并关闭；错误不允许丢弃已分配资源的句柄。

`PublicationRecovery.Resolve` 消费已经持久保存的准入证据，每次先解析完整候选和权威绑定，
再向实际权威建立新的发布围栏。围栏须在读取该准入之后建立，不能复用早于准入的缓存检查点。
新证据同时固定本次独有准入 ID。恢复核对检查点的不可变发布收据、原权威坐标、完整候选和已接受历史；
等值候选的定义相同也不代表采用同一票据。收据指向另一准入时只解除失败尝试，赢家保护不受影响。
未知确认、绑定/ref 不符、坏收据或接受记录与收据矛盾都不给释放结论。缺少准入 ID 的旧证据无法凭等值定义
推定采用者；定义已接受但没有对应准备收据时保留未知保护。退役标记是后续政策，不改变原接受身份。
物理查询固定到该准入对应生成代，以及后续观察/查询保护，继续由索引接入。

`PublicationRecovery.Evidence` 由已接受修订的收据和完整定义重建原始准入证据，不推进权威或确认资源。
校验候选摘要与固定修订；有效旧纯文件版本可返回空证据，权威不可用或状态被抹除则失败。
组合根将该端口交给索引查询，物理提供方只消费不透明字节，不解析 Dataset 或对象协议。

`PublicationRecovery.Adopt` 连接既有不可变发布与可选准备采用端口。准备须保留完整原定义
（包括原依赖），回调原地改写也会被摘要检查拒绝；不重新冻结当前 HEAD 作为旧版本依据。
仅在取得实际托管票据后准入并提交收据，无托管票据时不改变该发布。准备失败只取消未准入票据，
准入或接受结果未知时保留耐久保护，始终关闭本地句柄；确认失败不会撤回已接受收据。

`PublicationRecovery.Referenced` 将历史接受事实与当前引用资格分开：经实际权威的只读
`PublicationInspector` 核对同一发布身份，再检查实际生命周期。未退役的保留定义仍有引用，
已知退役解除引用；缺失生命周期、坏收据或未知权威不当成退役，不写发布围栏。
检索保留到期等政策仍由其所属合同接入，不能用接受收据的存在替代完整保留判定。
