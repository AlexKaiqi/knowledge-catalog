# SINGLE-REGISTRY-01 · 一套服务一个登记空间（2026-10-12）

- [ ] 单登记空间实现及受影响普通检查已完成；完整契约仍有 11 项既有外部/真实环境测试跳过，按严格「全绿且无 skip」规则保留未勾选，不登记全量/真实部署验收通过。
- Goal / owner：仓登记与 Dataset 发布、固定历史由 dataset 拥有；模块边界由 core-architecture，部署恢复由 service，交互由 cli，权限由 permissions，公开词汇由 terminology 拥有；相关关系按 docs/graph 读取。
- Non-Goals：不删除仓登记、Dataset 不可变发布记录、历史、退役、当前权限、CAS 或恢复；不改 Object/Schema/Writer 写入语义，不实现多租户，不执行对象模块迁移，不安装依赖；提交按用户最新授权执行。
- 不变量：保留 KS-01/KS-02、V-01、AUTH-01～03、API-01、A-01/L-01；唯一空间不使连接等同登记、不使组合发权、不让 pin 扩大范围、不从缓存重建权威。
- 选定：部署唯一登记权威和唯一 Dataset 服务；客户端选择 Server 后直接操作 Repository/Dataset；删除 Catalog 参数、枚举、切换及整体归档。否决保留虚假默认 ID、多空间兼容分支和以删名词方式丢失状态。
- 接口：snapshot/dataset 的 Service/Registry 与状态合同、snapshot/service 的单 Datasets 资源、单 Registry 部署 binding，Client/CLI/HTTP 与 Console 同步收口；权威身份来自实际 Snapshot Repository，仅用于恢复核对。
- 验证：先增加并运行退役选择拒绝、唯一服务发布/历史/恢复与无 Catalog 公开形状的失败证据，再修改实现；运行受影响 Go/前端回归、make test、make test-contracts、make check-docs、make quality。真实部署验收按独立部署合同另行执行，未运行不得记通过。
- 增量质量问题：本轮发现的信号逐项当场修、登记债或升级守卫，不修改门禁阈值、不删弱化合同、不 skip 换绿；保留本轮开始前工作树改动。
- 实现范围：唯一 RegistryBinding、dataset.Service/Registry；删除 Catalog DTO、选择上下文、列表/切换/整体归档、请求与政策选择维度；HTTP 收到 Snapshot 模块入口，CLI 与控制台直接使用服务库存。仓登记、Dataset 不可变记录/当前版、历史、CAS、范围、退役与当前授权均保留。
- 恢复边界：旧唯一登记元信息只读恢复，首次明确管理变更迁移且不重写未变 Dataset；旧唯一治理账、供给回执和部署身份保持原历史与幂等结果。旧登记与治理账身份须匹配已装配的实际登记权威；多空间、混合/重复字段、权威不一致与遗失元信息明确拒绝；旧 Catalog 范围/动作政策不做自动扩权迁移，部署方须显式更新政策。
- 当场修与守卫：退休字段大小写导致的范围扩张、混合治理账丢历史、元信息迁移制造虚假 Dataset 历史、场景新权威指针未隔离、表格批量变换越行、系统入口旧动作分组/说明漂移、登记审计分页漏传与控制台 create 名称路由碰撞均已修并补回归；Catalog API/集合/参数/生命周期与场景表格闭集升级防回潮守卫。
- 验证范围记录：三组 Snapshot 普通/race/vet、Home/Invocation/Server（含 readiness）/HTTP、Client、155 项前端及完整控制台构建/Go 回归通过。第一轮普通/完整合同的旧接线失败与终止均保留为诊断，不计最终验收；所有已发现增量错误修复后完整复跑。未安装依赖；用户在验证结束后明确要求提交；真实部署、外部 SSO、付费模型、live runtime 与规模资格按各自环境合同，不计入夹具验收。
- 最终同源结果：源码指纹 `6666cdf2cb87989bf6cac82b588d259a88b35aa428a2a67ee99632acae193717`；`testsuite:lakefs` / `20261012T130034Z-1dd845e135ee`（make test）退出 0、21 个通过事件、0 fail/skip、sourceChanged=false；`testsuite:contracts` / `20261012T130034Z-1c10447eebed`（make test-contracts）退出 0、2162 个通过事件、11 个既有 live skip、0 fail、sourceChanged=false，CLI 57/57 成功且满足风险分层边界。通过事件含子测试，不当声明库存通过率。
- 最终门禁与材料：make check-docs（32 文档 / 143 关系）、make quality、make check-validation、git diff --check 全部退出 0；层/导出面/退休选择/HTTP 87 条闭集/文档 Test 引用/维护检查通过；场景 views/product/states、45 项场景工具测试、scale 3 项无网络请求合同与数学 self-tests、perf_tree --check、fixture 编译及 shell 语法检查通过，性能树原 history-generation/rollover gap 保留。任务记录回写后再次复跑文档/质量闸；行为源码未再修改。
- 未运行的 11 项外部合同：TestLiveHTTPStateRuntimeContainer、TestLiveHTTPRuntimeBuildsOpenSearchStateProjection、TestLiveLunaListwiseRerank、TestLiveTaihuAuthentication、TestLiveBulkApplyAgainstRealLakeFS、TestLiveHTTPSearchRerankWithLuna、TestSceneAccessorNoticeUpdatesSearchIndex、TestLiveLakeFSSceneDFS、TestLiveServiceProviderConsumerJourney、TestLiveHTTPDynamicStateSearchJourney、TestScaleProfileRepoAddLakeFS。保留其真实环境要求与原 skip，不把未运行登记成通过；独立真实部署/FUSE/模型/身份/压测验收未取得。
- 提交前复核：用户在本轮验证结束后明确要求提交；依赖的 Snapshot 在途迁移与退役接线随一致候选纳入，AGENTS、对象模块 plan/guideline 及图示/关系/旧相关任务记录、独立 Git helper 清理留在工作树。纳入此前未跟踪源文件后发现三处空白问题，已修正；155 项前端与完整控制台检查复跑通过。精确暂存候选导出后 make quality（30 文档 / 123 关系）、internal/arch、check-docs 与 validation-inventory 全部通过，暂存 diff --check 通过；无生成产物或真实密钥入提交。完整契约的同源运行仍按上列指纹和范围解释，提交前只增加空白修正、说明同步与无关改动排除，不追加真实部署验收。


本轮登记的存量债（未借单空间改造追存量）：

| 位置 | 信号 / 判定 | 处置 | 触发条件 |
|---|---|---|---|
| `.data/scale/runner/smoke.py`、`ladder.py` 与 `scenes/env.example.yaml` | 默认 scratch `kc-perf-*` 使用保留前缀，run stamp 大写 T 不满足 lakeFS 名称；原 runner 漏洞，与本轮选择维度无关 | 保留并登记，后续统一合法 scratch 生成并补真实 Client 校验的无网络回归 | 下一次运行 named managed scratch 压测前 |
| `scripts/testsuite.sh` taihu-live 分组 | 选择 ./cli，但 TestLiveTaihuAuthentication 实际在 ./server；原 live 入口可产生无测试成功 | 独立修正真实包与存在性守卫；本轮不把此入口算 live 证据 | 下次真实 Taihu 验收前 |
| `scripts/e2e-kcfs-linux.sh` 临时部署 | 原脚本用新 stateDir 绑定共享且已初始化的 kc-catalog 权威，按恢复合同应拒绝；原有验收环境接线问题 | 保留单空间参数迁移，后续每次 run 绑定独立物理登记权威并清理，不重置共享权威 | 下一次真实 FUSE 验收前 |
| `.data/scale/runner/smoke.py`、`ladder.py` 环境合同 | 已声明 credentialsEnv/setupAllowCreate=false，但 Client 仅发 X-Kc-As 且始终 create；存量 runner 尚未实现该环境能力 | 明确不以本轮请求接线回归证明真实环境资格，后续补认证与既有仓路径合同 | 第一次非 local-auth 或禁止创建环境的真实压测前 |

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。早于 2026-10-14 对象模型重构的认领，相关代码与设计已被后续重构改写（见 `0759640` 之后的提交），原范围按旧结构写成，不再继续。整体验收并入 `FINAL-ACCEPTANCE-01`，插件相关用例由验收规格 accept-01～13 取代。
