# DATASET-PERMISSION-TESTS-01 · Dataset 权限测试与用例优化（2026-10-13）

- [ ] 本轮认领：将此前的充分性与必要性审计落实到测试和用例，补足失效反例与状态断言，归并同义重复，保持所有独立授权、范围与传输边界；不提交。
- Goal / owner：Permissions 的身份、资源隔离、当前授权与耐久决定，Dataset 的可信发布范围和固定依据，Test Catalog 的最低责任层与独立风险规则，Quality Loop 的反例与质量闭环。所有权及直接关系按 docs/graph 读取。
- Non-Goals：不引入对象或字段 ACL，不修改公开动作、DTO、授权政策或底层存储，不把动态源共享和发布选源的未选定 authority 编成测试假设，不调整质量阈值、不安装依赖，不覆盖其它聊天的在途修改。
- 不变量：保留 KS-01、KS-02、AUTH-01～03、V-01、API-01；pin 不授权，清单不扩仓，失败不污染授权账或已接受发布，业务请求仍经已认证 Server。
- 选定：先让正反用例确实到达目标检查；授权更新覆盖 old/new 四格、精确错误码与深快照；补临时多来源的第二阶段授权、真实授权管理到 file.read 重判、准确可见集合及落盘边界。纯参数留最低层表驱动，各 transport/provider 仅保留自身转换和公开组合风险。
- 否决：用测试数量或通过事件宣称充分、用另一道前门拒绝冒充目标检查、只验任意 error/数量/秘密 marker、为减少数量删除独立合同、用当前实现反向收窄设计。
- 接口：沿 snapshot/access、snapshot/dataset、snapshot/service、snapshot/client 的既有合同和应用装配；本轮主要修改测试、场景与必要的证据说明。发现行为缺陷先保留反例，再按相同 owner 合同修复。
- 验证：定向合同与公开 CLI/HTTP 旅程、必要 race、反例故障注入，以及 make check-docs、make quality、普通场景与完整 contracts；分别保存源码绑定结果，不合并历史执行或把 skip 当 pass。仅完整所需合同全绿且无 skip 后勾选。
- 质量出口：未闭合的发布选源/分享 authority、最后准备回调撤权窗口、动态源消费/共享/目标信任按原 owner 保持债，不造虚假通过。每项本轮发现的问题记录位置、信号、判定、处置、触发条件；同类弱断言以表驱动状态断言和反例回归固定，不再依赖人工计数。
- 实施结果：规则坐标逐维反例与 Add/Revoke/Update 账状态使用独立深快照，更新覆盖旧/新范围四格、准确冲突码、并发唯一赢家及私有迁移回执；临时多来源、具名与同名临时对象、metadata、三个既定 hook 窗口均补正确前态及精确后态。文件范围钉准确路径、来源提交和字节；cursor 纯参数矩阵归组件，HTTP 留转换与撤权见证，重复 hook 操作由已有最低层合同拥有。
- 公开旅程：新增只撤 file.read 而保留 dataset.resolve 的真实耐久授权更新，以及 Repo×Dataset 部分来源授权到目录、读取和正式 clone 的路径与落盘字节核验。二者保持完整接受 pin 不变，另一已认证主体仍能读取原数据，不以不存在的文件或前门拒绝冒充文件授权。
- 当场修复：`invocation/dataset_access.go` 的文件适配将 Repo 与 Dataset 分开求值，Repo×Dataset 交集规则丢失一个坐标；新公开旅程和组件反例在原代码 overlay 上真实报红。私有 helper 传入实际 Repo 与 Dataset 后转绿，保留原 Match、授权政策和公开形状；独立增量审查无阻断问题。
- 门禁差距登记：同期资源访问设计新增 U10～U12，产品覆盖检查报缺入口/缺口；在 `.data/scenes/_views.yaml` 如实登记实例选择、替换与兼容执行仍缺的旅程，保留检查器和既有证据。后续实现这些新合同即须补实际场景，不能用本轮 Dataset 固定文件证据替代。
- 保留债：`snapshot/service/dataset_http.go` 的发布选源/分享 authority 尚无已选定独立动作，判定为 owner 合同待选，触发为公开发权方案落定；PreparePublication 内发生撤权的最终提交边界尚无请求期授权/租约合同，判定为待审窗口，触发为该提交边界合同选定；动态来源的消费授权、观察共享与目标信任差距继续归 `resource-access` U6/U7，不把固定文件绿外推。历史真实 clone 脚本检查旧输出目录的失效 oracle 仅留历史审计记录，当前正式 clone 旅程检查当次唯一 TempDir 的精确落盘集合，后续采用该脚本前必须修复并复跑。
- 测试闭环修正：`scripts/maintenance_test.py` 的 surface 守卫隔离夹具缺同期迁移后的 object / snapshot/cli / snapshot/client 目录及两份 README，基线真实报红；仅同步前态后六项回归全绿，缺扫描路径必须失败和全部退役反例仍保留。Go 测试保存基线核对为 18 份既有文件修改、1 份新文件、10 项真正新增、4 项改名；6 个独立故障有红/绿见证，未将编译失败或历史变异计入。
- 证据合同：共享工作树仍有并行迁移，普通场景断言虽绿但运行期修改套件脚本导致 shell 尾部读偏移，不能将该入口退出失败写成通过；以当前文件及原 HEAD 建立无提交的隔离验证副本，复制前后指纹一致后重跑质量、库存、普通场景与完整合同。最终同源结果另见 `.validation/test-opt/20261013/`；既有外部/live skip 和未执行真实部署单列，不勾选为完整无跳过验收。
- 完整合同发现与修复：第一份稳定副本的各测试断言通过，但 TestMain 的逐命令边界门禁以 61/61 成功、57/61 达标报红；`cli/snapshot_consumer_commands_test.go` 的四个迁移后命令缺可计入且精确的失败断言。复用既有三条旅程，补未发布版详情、异版 cursor、无管理权更新，强化撤权/下载目标/过期观察的准确错误与完整状态不变。定向三项无 skip 全绿，四命令成功与边界全部达标，未改门禁或增加顶层测试；本轮现为 20 份 Go 测试文件、10 项真正新增，受影响文件内 66 个顶层 Test。随后以新稳定副本重跑完整合同，不用拼接两次覆盖报告替代套件通过。
- 全仓合同阻断出口：第二份稳定副本的组件合同被同期新增的 `object/writer/representation_consistency_test.go`、`object/reader/readonly_view_test.go`、`invocation/direct_access_contract_test.go`、`discovery/service/notice_background_source_test.go` 反例阻断，信号为表示冲突、具体只读返回值仍携写能力、直接访问核验/证据和后台来源隔离失败。判定与处置沿本文件 `OBJECT-BOUNDARY-REGRESSIONS-01` 的明确用户边界：本轮只补反例，等待在途重构后再修实现；保持红证据，不删除、skip、放宽或代修这些领域行为。触发为该重构切片修复后重跑完整 contracts；本轮另独立执行完整 CLI 合同确认新增边界，不将受影响测试和质量闸绿写成整仓合同全绿。
- 最终执行范围：稳定副本 `frozen-v2` 的本轮 66 个顶层 Test / 121 个叶全部通过、0 fail / 0 skip，授权并发两项 race 通过；`make quality`（含 check-docs）、`make check-validation` 与普通/索引场景通过。独立完整 `make test-e2e` 退出 0，61/61 公开命令有已断言成功，61/61 满足风险对应的失败边界要求；495 顶层 pass / 6 既有外部/live skip，不能把 skip 记为通过。完整 mixed contracts 因上述另一任务六个顶层反例失败而退出，保持本条未勾选；各最终 run 的 sourceChanged=false，代码/测试与当前工作树相同，记录性 TASK 后补。结果、分层目的、去重及反例依据保留于 `.validation/test-opt/20261013/RESULT.md`，未执行真实部署、未提交。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。早于 2026-10-14 对象模型重构的认领，相关代码与设计已被后续重构改写（见 `0759640` 之后的提交），原范围按旧结构写成，不再继续。整体验收并入 `FINAL-ACCEPTANCE-01`，插件相关用例由验收规格 accept-01～13 取代。
