# INDEX-EXTENSION-PLAN-01 · 当前协议上的最小索引接入（2026-10-13）

- [x] 文档方案完成：按用户选择修订最小插件接入机制、Object 边界与现有计划；未开发登记 API 或插件实现。Object 未迁移完成不阻止声明式索引基于现有协议先行。
- Goal / owner：Object 自身合同、能力和跨模块迁移归 object-refactor-guideline；最小插件接入独立归 plugin-extension，以声明式索引实际消费为依据；两条工作的执行与验证路线归 object-refactor-plan。声明、就绪/对账、查询、LiveAccess 与 CLI 各沿 declarative-index / index-control / retrieval / resource-access / cli；主题归属与关系只在 docs/graph 登记。
- Non-Goals：本轮仅修改文档，不写代码、不造完整 PluginHost/动态加载/包发布/热卸载/管理 CLI，不改 LiveAccess 或当前身份/Schema 采用/观察结构的在途实现，不安装依赖、不启动服务或部署验收、不提交。
- 不变量 / 接口：保留 API-01、A-01 / L-01、V-01、W-01 / W-02、R-01 / R-02、AUTH-01 / AUTH-03、PC-01；依赖 object 固定读/维护、discovery 查询与字段合同、discovery/projection 物理端口及 discovery/service 消费口。引用随共享迁移的实际公开落点更新，不复制 Object 或索引算法。
- 选定：声明式索引是提供查询、固定 Dataset 依据准备和已接受推进三项贡献的真实实例；系统登记→按明确目标选择→实际调用，产品装配构造并关闭。OpenSearch/EngineOpener 仅插件内物理适配器，不能仅注册引擎就声称完成能力插件化。
- 否决：完整 Object 和全角色 Host 成为首批前置、注册后仍暗用固定 cx.WS.Index、预先定所有 hook/安装管理、用 SearchProjection 接口声称任意代码 Dataset 隔离。当前 context scope/budget 仅约束可信既有 Index；任意候选开放时再抽被所有实现经过的 typed 边界。
- 计划：R0～R3 按首个切面的真实分母执行；原完整 Object 迁移出口继续未完成。先构造实际实例、登记三贡献，再明确选择和调用；默认与另一具名 Index 实例验证目标范围/固定依据/失败和关闭，第二种真实能力需要共同机制时再提取。
- 质量出口：同步撤回上一版过度预设，既有审阅债按当前源码/反例及实际触碰复核，不把其它在途修正算成本轮实现。重读 owner/图、运行 make check-docs / make quality 与增量空白复核；仅说明文档和工程门禁，未验证业务或部署。
- 本轮结果：最小体系与计划已改写并独立复核，明确构造实例→登记三贡献→目标选用→实际调用/维护→关闭；推进按实例服务需求覆盖 Repo 和已接受 Dataset，不让 Dataset 跟随源 HEAD。撤回 Object 完整迁移、全角色 Host 和包/热卸载/管理 CLI 的首批前置；CLI 只按实际合同差异同步。
- 模型纠正：能力实体仅 Plugin，Affordance 是能力说明、CLI/help 是载体；系统提供注册、作用域选择、服务和 hook。撤回将 Interface 作为并列架构层的表述和预设目录；现有 Go 类型与已发布合同只作兼容约束，未改代码或命令保证。
- 本轮边界调整：保留 Object 自身要求→承诺及与索引/LiveAccess 的依赖、职责拆出和兼容验证；移出插件登记、目标选择和三贡献执行方案。Object 完成不要求首个插件接入完成，但受影响的上层合同必须验证；两条工作分别列分母，不用文档归属或路径存在宣称实现验收。仅修改文档，先重读 owner/图再运行文档与质量闸。
- 本轮审阅债：位置 knowledge.md 与 object-interface.md 的旧能力实体表述；信号是部分文字仍把 Interface 与 Plugin 并列。判定为目标术语与既有兼容合同未完全分开的存量文档问题，本轮不重设其协议或执行行为；处置为对应 owner 同步区分历史合同与目标实体，触发为能力声明/兼容分派专题下一次维护。位置 snapshot 的追加监听与索引固定服务准备；信号是无单插件撤销及维护输入可能覆盖来源整仓，判定为既有能力边界，已在 plugin-extension 明确；触发实际热卸载或限制业务代码处理材料时另行选定合同和越界回归，不以作用域选用宣称隔离。

- 模型纠正复核：目标指南、计划、CLI、全景与术语已统一；旧图仅保留历史链接，不再作为当前能力模型展示。make check-docs（32 文档 / 149 关系）与 git diff --check 通过；make quality 的格式/tidy/vet/staticcheck/复杂度/体积/克隆阶段通过，但同期源码迁移使耦合传播升至 37.32%、新逻辑单元尚未完成基线登记且 snapshot/service 漂移超阈值，整体退出 2。本轮不调整基线或削弱门禁；触发为迁移按同一政策完成真实依赖与逻辑单元复核后重跑，仍不标完成。
- 验证限制：共享工作树同期进行 object/discovery/index → object/discovery 的源码迁移。首次 make check-docs / make quality 因 repofile.Parse 在途编译未过；后续编译恢复后，检查仍被其它 owner 与历史 TASK 的旧路径 Markdown 链接阻塞。本轮已同步自己修改文档的实际 API 锚点，git diff --check 通过；未修改迁移源码或批量改其它 owner，未弱化门禁，未取得整仓 docs/quality 通过，保留本条未勾选。触发为迁移稳定并完成全图引用同步后重跑两项检查，旧版本通过记录不计本轮。


- 本轮边界调整结果：Object guideline 已聚焦要求→承诺、公开依赖和跨模块职责拆出；plugin-extension 独立承接真实索引三贡献及目标选用；计划分别列 Object 自身、消费兼容和索引接入分母；索引与 LiveAccess 具体设计沿各自 owner。新文档及主题/直接关系、总览/CLI/组织引用已同步并重读，独立复核通过；未改代码、测试、命令或出版物。
- 接入设计补实：上一版仅约束登记/选择，仍把实际机制留给实现，不能作为开发依据。本轮选定静态 Go 插件入口和系统生成的登记上下文、一次性接纳贡献组合、消费侧只读实例目录、启动冻结的 Repo/Dataset 精确选择表、系统需求视图、HEAD/固定 Dataset 两路维护、实例资源 namespace 与续页执行身份，并列出全部既有调用的改接点；只改文档，不把目标登记 API 或多实例隔离写成已实现能力。具体字段与 Go 签名归实施时的公开代码，本文不编造网络协议或管理命令。

- 边界调整初次验证记录：make check-docs 通过（33 文档 / 163 关系），git diff --check 通过。quality 首次停在共享迁移新增文件检查，直接复核后通过；随后发现并修正文档的在途路径断链。最终 quality 的文档及格式/tidy阶段通过，但 go vet 因 CLI 引用迁移中已不存在的 kcclient.WorkspaceFileCoordinate、WorkspaceFilesService、SnapshotService 等类型而退出 2；其后静态/复杂度/体积/克隆/耦合阶段未执行，不能记为通过。日志 /tmp/kc-object-doc-boundary-quality-final.log；处置为保留待验，触发为 Snapshot/Client 迁移恢复编译后重跑质量闸，不修改源码或弱化守卫换绿。


- 接入设计补实结果：已明确插件入口与登记上下文、完整实例接纳、冻结的精确目标规则、HEAD/固定 Dataset/State 需求及实际路由、固定恢复脱离 HEAD 门槛、逐 basis 独立就绪、所有索引任务改接、额外实例物理 namespace 与多层续页身份。旧资源接纳者的固定身份与部署默认选择分开；默认指针变动不转移资源或旧 token。来源上下文不由请求改写，Stop 与资源账各自唯一归属，首次需求失败拒绝激活；独立具体机制复核后的问题已修正文档。
- 最终文档与闸证据：重新读取改动 owner 与图，make check-docs 通过（33 篇 / 165 关系），git diff --check 通过，make quality 全绿（格式/tidy/vet/staticcheck/复杂度/体积/克隆/耦合）；日志 /tmp/kc-plugin-concrete-quality-final.log。先前静态检查中的 U1000 / ST1005 由并行源码工作恢复，修复和耦合基线变化不计本轮实现；本轮仅修改 Markdown/文档图，未修改 Go、测试、配置或出版物，不将质量闸记为新插件的业务验收，所有接入实现仍待开发。

## 本条保留的 LiveAccess 历史审阅信号

下列信号按现有访问、观察与权限 owner 的保证保留。触碰相应 LiveAccess 路径时先复核
当前工作树和反例，确认缺口后在对应专项切片独立修复；不是 Object 目录迁移或首个静态索引接入的无条件前置，
也不能把注册或路径移动当作风险已经消失。

| 位置 | 信号 | 判定与处置 | 触发条件 |
|---|---|---|---|
| `liveaccess/live_access.go`、`serving.go` 与 `state_lookup.go` | 通用 LiveAccess、Bound State 与直接 access 的值和 basis 核验分散，完整包络不等于采用结构及固定声明关联已核验 | 统一合法观察依据、实际采用值结构与声明关联的核验；补伪造/不匹配依据及结构不合规值的拒绝反例，保留合法空值语义，不预设新字段 | LiveAccess 注册、来源返回值及按观察依据回读切换前 |
| `invocation/verbs_resource.go` 的 `access` 与访问证据编排 | 直接返回 bindings/observations 的取值路径未完整进入与 invoke 对齐的实际访问证据链 | 在所属任务服务记录真实主体、固定声明、实际观察及成功/失败结果；保持公开 access 返回形状，不以请求完成日志替代访问证据 | access 服务迁移和默认 binding 切换前 |
| `liveaccess/state_lookup.go` 的 HTTP 出站装配与调用方证明转发 | 声明 origin 和通用 HTTP client 不足以证明目标信任、证明适用与重定向后的委托边界 | 由宿主注入目标/证明政策，来源适配执行目标与重定向检查；补新地址、跨目标重定向及源拒绝反例，不回退旧观察，不把凭证写入知识或证据 | LiveAccess HTTP adapter 归属切换或改变出站证明装配前 |

另纠正此前一项判断：`ResourceLookup` 会返回注入的 lookup 或构造 HTTP 适配器，本身
不访问外部源；`WithLiveAccess` 优先匹配已安装 Handler。当前已有独立 Echo 的正式
调用测试。因此通用调用中出现 State 适配器属于结构耦合，不能直接判作“普通能力无法
调用”的行为缺陷。无需来源访问的契约可以提前补强，但授权动作不因拆包自动改变。
