# OBJECT-REFACTOR-IMPL-01 · 完成对象层重构及充分验证（2026-10-13）

- [x] 本轮唯一认领已完成：按 Object Guideline 的结构终态与 U1～U12 落实生产实现；承接 OBJECT-CASES-PREP-01 的规格、既有精确断言和三个红基线，以同一最终候选的 Object 必需运行证据及受影响消费兼容验收闭合。
- Goal：固定知识身份、唯一表示／组装与 Schema、真实只读 Reader、唯一 Writer、统一对象 URN＋JSON Pointer 的读／完整单元写／显式删除，以及能从根帮助理解和操作的 Object CLI；命令、校验、输出、帮助和既有命令扩展同归 `object/cli/`，KC 宿主只经公开贡献装配。
- Non-Goals：沿 Guideline，不扩任意字段 PATCH、集合／数组删除、跨 Repo 事务、组合对象整体 PUT，不重设计 Snapshot／Dataset，不把具体插件实施并入本轮。目录草稿 diff／commit 和回执仍是应用发布流程；既有入口及授权、版本、原子性保持兼容，不覆盖并发工作、不安装依赖、不提交。
- 不变量与接口：I-01、C-01、AUTH-01～03、CA-02 及 K-04 的固定身份／依据和当前权限；公开合同沿 `object/`、具名 Reader／Writer／Service／Format 和各自 Conformance，新增寻址类型在 Object 根选定，传输输入将完整引用作为请求数据交给所属服务。
- 方案：共享结构化引用与 Pointer 语义、按维护单元提前制定读取计划、所属模块贡献 CLI；否决中央 CLI 复制对象规则、第二份组装／解析器、路径充当身份、先整对象读取再裁响应及仅收窄接口名的伪只读。先修必要行为，再切换结构和新入口，逐切片测试后进行充分消费兼容、架构、文档、质量验证。
- 协同：Writer／Format 基线、Reader 实际能力边界、Object CLI 迁移由三个子任务分担；本聊天统一寻址／服务接线和全体集成验收。先复现所选红例再修复，不削弱旧断言或用 skip 换绿。

- 已落实：根 Object 类型与唯一单元代数、Writer 最终工作集／完整 Schema 校验、实际只读视图、URN＋Pointer 精确读与完整单元 PUT/REMOVE；Object CLI 和 HTTP SDK 均归自己目录，兼容入口由贡献装配。纯 Binding 声明仍属 Object，专属来源解释唯一归 LiveAccess，Dataset 逻辑 URI 读取保留观察与当前权限。
- 增量修复与守卫：根 REMOVE 迭代漏删、表示冲突、伪只读、深选先整读、复合来源丢失、根 Relation 种类丢失、URN 不能沿用 Aspect 授权、Schema 同批迁移／新增引用再删除悬空，以及原生元数据计划不核坐标；均保留精确正反例。唯一 Format.ToObjectUnits 转换取代三个字段映射；全仓 150-token 克隆检查为 0 组。
- 闸层兼容处置：完整组件暴露 Dataset 竞态用例绕过真实 Reader，已改为同一 Snapshot Registry／Reader 的实际装配，原范围／旧版／推进时机断言保留；HTTP 具名表增加两条对象维护路由，相应闭集清点 92→94，并逐条守卫。已有 Notice 来源隔离两条红例沿原 Controller 修复：通知不替换后台来源／身份，保留真实桥接反例并强化原配置测试，无新增索引插件功能。
- 耦合收紧：用同一当前工具、depth=2 政策与 macOS 平台重算改前冻结树及候选，保存前后 import 图和度量于 `.validation/object-refactor-impl-20261013/coupling/`。66→68 单元仅登记 `object/cli`／`object/client`；传播成本 16.67%→16.26%，既有单元没有恶化，`internal/httpjson` 的 Instability 0.33→0.25 来自复用认证传输；人工归为设计使然，并显式 `-write-baseline` 收紧，不改门禁阈值或排除。
- 已验收范围：URN 的四项真实 CLI→HTTP 旅程与 lost-response 真实 Writer 回执恢复通过；Object 根／Format／Writer／Reader／SystemRepo 及 Reader／LiveAccess 定向 race、所有权与库存守卫已绿。完整闸暴露的失败已逐项处置并按固定候选复验；虚拟 Bound 仍须读取固定对象类型依据，不能声称所有逻辑深选都零兄弟 I/O。真实部署为独立入口，不把夹具或带门控 skip 的运行算作部署验收。

- U1 独立验收：新 Agent 不继承实现上下文、不读源码／设计／测试，仅沿根帮助及四个操作叶帮助和身份帮助，在真实 CLI→Server→Writer 上完成初建 Member、nullable=false 深读、带完整摘要更新、确认省略 description 后不可读、删除后 Member 不可读；6 次帮助、9 次业务调用，日志 `.validation/object-refactor-impl-20261013/help-walk/agent-transcript.txt`。自动化预设参数旅程不代替此观察，U1 已补闭合；仅证明本次独立 Agent，不泛化为所有使用者。
- 走读信号处置：Object 叶帮助的 published 默认 ref 原未明说，已明确为 refs/heads/main；根帮助未列身份英文组、认证模式选择需要用户判断，属于应用 CLI 说明的既有缺口，不阻断本次 Object 走读，登记为后续该 owner 维护时补根导航／认证发现指引，未越界改认证模式。原生 Reader 的导出嵌入字段可取回写 Provider，判定真缺陷，三条字段能力断言先红后绿；改私有读接口嵌入，不新增转发算法。

- 最终兼容补闭环：Notice 按本次目标同步刷新，共用原算法并返回实际 basis/revision；仅在无后台 runtime 时局部借本次 Source，不保存、不执行其它仓或排队工作。已配 worker 失败不借 caller，后台上下文不继承 caller 凭证，来源查找串仓在外呼前拒绝；真实 Controller、HTTP proof、OpenSearch 与旧 CLI/HEAD/二次 revision 断言均绿。Server 只在 runtime 与已有明确 service principal 同时具备时装配后台桥；另补真实 HTTP 的无后台配置局部执行回归，并完成竞态验证。
- 旧消费兼容复验：完整合同发现的输出／帮助／未知参数矩阵／系统出版物／路由清点问题均保留原观察并修复。统一引用附维护依据，旧分列 READ／resolve 保持各自产品形状；合法命令矩阵每叶采用自己的输入，未知参数仍拒绝；最终帮助及系统入口同源生成。

- 最后环境门控补验暴露旧夹具调用已退休 --catalog；仅把 SceneAccessorNotice 前态改为现行 local init，版本／实际搜索结果／HEAD 断言保留。State 部署脚本仍引用已不存在 catalog-initialized 且只跑 CLI、不跑其两项 LiveAccess 合同，判定验收入口真缺陷；修到实际材料目录和明确的两包，未削弱门控或新增依赖。真实 Docker State＋OpenSearch 的作用域验证另存固定候选证据。

- 真实 SceneAccessor HTTP 补验又暴露“仅配置 request runtime、未配置 worker 身份却当后台源”的失败（UNAUTHENTICATED）。Server 现只在已有 ServiceIdentity 明确给出 principal 时装配后台观察，使用该身份；请求 runtime 保持可用但不隐式安装 worker。无后台身份的 notice 使用本次已验证请求，已配置 worker 仍不借 caller；不新增身份或插件 API。

- 验证环境故障处置：v4 对象／消费方 race 520 项、实际 URI/HTTP race 5 项及真实 State/OpenSearch 4 项均通过且无 skip；其后并行追加临时检索容器造成资源压力，既有 OpenSearch 于 15:37 UTC 被 OOM 杀掉（Docker State.OOMKilled=true），合同场景超时及两个追加入口中断，原失败证据保留不算验收。已清理本次临时容器并恢复原检索容器；State 入口复用既有 KC_TEST_OPENSEARCH_URL、只回收自己创建的资源，健康探测加有界超时。最终候选串行复跑依赖检索的入口，不修改业务 Oracle 或用 skip 避开。

- 完成证据（2026-10-14 北京时间）：最终 v5 冻结源码指纹 `768b41a49aca4f723b460d4d4fc4d5ab58ebc74b0bab88f04624bd5816867984`，与交付前工作树一致；各正式运行均 sourceChanged=false。证据集中于 `.validation/object-refactor-impl-20261013/final-v5/`：`make test` 21 pass／0 fail／0 skip；`make test-contracts` 3203 pass／0 fail／12 环境门控，61/61 CLI 成功且满足各自边界要求；Object／消费方 race 520 pass／0 fail／0 skip，加入 Server 后 665 pass／0 fail／1 外部 Taihu 门控；URI/HTTP race 5 pass／0 fail／0 skip；真实 Docker State＋OpenSearch／SceneAccessorNotice 4 pass／0 fail／0 skip，修正后的正式 State 入口另有 3 pass／0 fail／0 skip。上述均来自同一 v5 候选，不使用中断运行或旧候选替代。
- 必需闸与未执行分母：`make check-docs`（33 文档／174 关系）、`make quality`、`make check-validation`（48 项工具合同及 61 CLI／94 HTTP／1652 Go 声明清点）、空白检查通过；已重读变更 owner 与图，并完成独立所有权复核和 help-only 真实走读。广域合同的 12 个门控中，受影响的四个 State／通知用例已按上条单独无 skip 补验；其余八个为 Luna、Taihu、真实 lakeFS adapter／部署／规模与外部 provider-consumer，未执行、不计通过。独立 `make deploy-local-scenes` 未执行；本条完成 Object 实现及必需验收，不宣称完整外部部署验收。结果注记写入后只改变本 TASK，生产代码与冻结候选保持一致；未安装依赖、未提交。
