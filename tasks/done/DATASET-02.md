# DATASET-02 · 发布权威、消费范围与服务生命周期收口

- [ ] 本轮认领：修复架构 review 复现的 pin 扩权、跨 Aspect 越界、语义 VFS 越界、服务版索引随 HEAD 丢失与版本可覆盖问题；消费与发布编排收敛到可独立验证的公共边界。

**交接状态（2026-09-20；后经用户纠偏）：** 本轮实现已落地。当时中止了运行中的全量回归并保留待验收状态；「用户要求只做实现、由用户在其他环境测试」的表述并非用户约定，已废弃。现行标准：代码修改必须通过测试，待验收状态由全量测试与契约门重新确认。

**默认测试入口纠偏（2026-09-21，本次不执行）：** Goal：`make test` 和 testsuite 默认入口使用原有 lakeFS 场景夹具，不要求准备整套部署；owner 为 `doc/engineering/test-catalog.md`，部署验收边界见 `doc/design/service.md` §11.1。Non-Goals：不改生产 provider 默认值，不迁移全部历史夹具，不删除或削弱合同，不运行测试或启动容器。不变量为 `A-01`、`V-01`；选定运行 `TestProductScenes` 与 `TestMetricPermissionScenes`，复用显式 `KC_TEST_OPENSEARCH_URL`，未配置时沿用一次性 OpenSearch。真实部署场景保留 `make deploy-local-scenes`，也保留在显式 `test-all` 中；原 component/boundary/应用合同组合保留 `test-contracts`。否决先前将默认测试绑定 `deploy-local` 的做法，以及将场景通过宣称为完整组件、HTTP/VFS 或真实部署验收。接口沿用现有 testsuite 分组和 Go 场景入口。

**Goal：** Dataset 的不可变发布记录是消费坐标和文件范围的服务端权威；当前服务版仅在必要投影准备成功后切换，源 HEAD 前进不破坏已发布版本。owner：`doc/design/dataset.md`、`doc/design/permissions.md`、`doc/design/discovery.md`、`doc/design/service.md`，产品目标见 `doc/design/object.md`。

**Non-Goals：** 不把知识或索引放进 Catalog，不把外部 live 数据写成 Snapshot，不按 Dataset revision 复制索引，不改变 VFS 只读且受本机容量限制的边界，不扩展跨仓写事务，不降低架构守卫与既有场景断言。

**不变量：** `V-01`、`KS-01`、`KS-02`、`AUTH-01` / `AUTH-02` / `AUTH-03`、`PC-01`。pin 冻结内容不冻结权限；任何读取、候选回读、Schema/动态绑定和文件投影均不能超出同一服务版范围；发布失败保留上一服务版。

**选定与否决：** 选定 Catalog 内不可变版本记录与单独当前指针、按发布记录重建消费 pin、知识读取共享完整对象范围校验、按固定 Snapshot basis 准备和恢复派生索引、typed 应用编排。否决信任客户端 Items / 任意 commit、任一 Aspect 命中就授权整个对象、语义 VFS 整仓共享投影、消费请求临时重建索引、用 HEAD 索引冒充服务版索引。切片中不完整的知识对象不伪装成完整对象；文件访问仍保留清单内文件。

**接口与验收：** `catalog.KnowledgeSet` / `ResolvedKnowledgeSet`、`object/reader.Serving`、`knowledgeapp`、`index.Controller` 与既有 File Gateway。先把 review 的真实失败固化成回归，再实施；覆盖伪造 Items/commit、历史 pin、当前撤权、跨 Aspect、语义 VFS、源 HEAD 前进与发布失败/重启恢复，最后运行 `make check-docs`、`make test`，全绿且无 skip 才勾选。

**关系边界增补（用户已确认）：** 允许关系保存在独立图谱仓，端点引用其他仓的对象；Writer 仍只提交关系所在仓，不解析或写入端点仓，不引入跨仓事务或隐式读权。Goal 是让 `REL-01`、系统设计 §7、Provider 合同与 Dataset 跨仓一跳检索一致；Non-Goals 为跨仓写、递归图查询、自动把引用对象加入 Dataset。不变量为 `REL-01`、`W-01`、`V-01`、`AUTH-02`、`R-01`；选定结构化 KnowledgeRef 与固定 Dataset 成员投影扇出，否决端点仓等于关系存储仓的校验和 authority 扫描。接口沿用 `CanonicalRelation` / `RelationRetrieveRequest` / `DatasetRelationsExecutor`。先记录 Writer/检索拒绝跨仓的失败，再修正合法双端点 Relation 场景；保留形状、权限、同版本回读和单仓写入断言，整体验收完成前不勾选。

**Provider 差分补充：** 原生 Dolt 实测复现了更新未携带路径时被 Writer 强填默认路径的问题，破坏文件切片稳定性。沿用 `docs/reviewed/provider-contract-validation.md` 的路径提示等价合同与 `KS-01`，目标是未指定新路径的更新保留原路径；不改变显式迁移语义。选定直接传递原始 ChangeSet，由 provider 在合并现有单元后为新单元补默认路径；否决写入前无条件补路径或在测试中补显式路径掩盖差异。回归为 `TestNativeKnowledgeDoltMatchesTreeProviderByOperationStep`，包含原有路径迁移、历史、正文及新增跨仓关系步骤。

**Dolt 验证补充：** 真实 adapter 回归另捕获预期失败查询后把已存在 commit 报成不存在。按 `doc/architecture.md` 的 Snapshot 版本合同与 `V-01`，修复 Docker 会话在远端分离 stdout/stderr 后破坏语句确认顺序的问题：在容器内合并输出，再经 Docker 传输；不以重试或忽略错误掩盖版本读取失败。接口不变，保留 ref/merge 合同，并重复验证真实引擎错误后的正常查询。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。早于 2026-10-14 对象模型重构的认领，相关代码与设计已被后续重构改写（见 `0759640` 之后的提交），原范围按旧结构写成，不再继续。整体验收并入 `FINAL-ACCEPTANCE-01`，插件相关用例由验收规格 accept-01～13 取代。
