# CLEANUP-02 · 用途、替代关系与独占风险盘点（2026-10-10）

- [x] 本轮完成：按实际入口和独占风险复核上轮候选，迁移已查清的一对一旧测试命令及确定的文档残留；保留在途开发改动，受影响测试、文档与质量契约通过，未提交。
- Goal / owner：消除已被现行入口替代的维护接缝，并为其余候选补用途、消费者、替代项和退役证据。过程归 [工程质量闭环](../../doc/engineering/quality.md)，测试分类和证据归 [验证体系](../../doc/engineering/test-catalog.md)，正式交互边界归 [CLI 交互](../../doc/design/affordance.md) 与 `cliSurface` / CLI Conformance。
- Non-Goals：不改对象 Interface / Dataset 控制台在途行为、业务协议和授权，不削弱或删除独占风险断言，不全局替换不同语义的旧动作，不裁决 vendor 来源树与依赖全集，不删除耐久状态、独有草稿或未归档运行证据，不安装依赖，不运行真实部署验收，不提交。
- 不变量 / 接口：沿用 API-01（共用应用执行器）、A-01（依赖方向）；测试必须分清正式 `cli.Run`、`RunEmbeddedForTest` 应用接缝和 `fixtureHomeOperation` 前态夹具。正式 argv 由 `cli/surface.go`、包 README 与既有公开命令合同拥有，不新增形状；无调用的 `internal/gitdir.BranchName` 属于内部 Git plumbing，按 [架构总览](../../doc/architecture.md) 与包 README 收窄，不改变 Catalog authority / CAS 边界。
- 选定 / 否决：从 `catalog-show → show` 的一对一簇开始，迁移正调用、保留后态/权限/旧命令拒绝断言，确认消费者清零后退役单项映射；删除全仓无调用、非公开合同的 `BranchName` 及可重建 Finder 元数据；否决机械删除零生产引用符号、把新拼写误当真实 transport 覆盖、把 `archive-repo` 的原归档要求直接改成登记移除，以及按体积或时间删除资料。
- 验证计划：先取得旧映射仍复活退役命令的失败证据，再修正；执行受影响 CLI 回归、公开入口与覆盖合同、工具/架构守卫、`make check-docs`、`make quality`。新发现逐项当场修、登记为债或升级守卫；未执行范围不计通过。

- 清理结果：6 处正向 `catalog-show` 调用改为 `show`，删除单项测试转换；新增有效夹具上的旧命令拒绝回归，原状态、授权和失败断言保留。这些旅程仍经 `RunEmbeddedForTest`，不计作新增真实 CLI/HTTP transport 验收。验证 owner 的 16 处过时操作标签与示例改为现行 CLI 或既有 typed API，C-05 对象坐标拒绝要求保留，不把 Repository 归档误换为 detach。
- 防复发：退役 Catalog 路径在多处文档重复出现，收进既有 check-surface；只匹配 `kc` / `go run ./cmd/kc --` 的命令示例，不禁裸历史摘要或内部动作名。隔离夹具的 8 个反例修改前全部漏放，现行命令与旧 HTTP 拒绝反例保留。
- 内部代码 / 本地资料：全仓查无 `BranchName` 调用；完整内部包的临时副本仅把该函数转为私有后，staticcheck 报 U1000，确认不是类型推断或 wire 形状误报，删除该函数及 unused import。仅回收 `.session_tmps/393bfd15-b262-4490-93ec-62785dc575a7/.DS_Store`（Finder 元数据，6,148 bytes）；其余 10 个会话文件与目录集合逐字节核对保留。
- 导出审计：AST 工具列出 627 个零跨目录生产限定引用的窄候选，其中 127 个有测试限定引用；这是建议清单，不是死代码计数。同包调用、推断使用、公开 wire/Provider 合同均须另行核对；本轮只删除已证实的内部函数，未确认候选保留。
- 验证结果：下列四次运行均为退出码 0、`sourceChanged=false`，共享源码指纹 `5d08a732839427593ca87c519c2bbffd9db59e285b140a39c16cda4b50ead62f`，测试 fail / skip 均为 0。定向 `KC_E2E_RUN` 只证明选定范围，不写成完整混合验收；覆盖报告记录 `show` 的 12 项成功断言与 3 项 FORBIDDEN 边界，仍属于应用接缝证据。

| run-id / scope | 执行范围 | 结果与边界 |
|---|---|---|
| `20261010T092842Z-23c9ccf7112d` / `cleanup2:maintenance` | `make check-surface check-validation test-boundary`；`go test ./internal/gitdir ./catalog` | 文档图 29 文档 / 110 关系；Python 6 + 4 项通过；Go 231 个测试终态事件 / 5 包通过 |
| `20261010T092842Z-dade3f18cc14` / `cleanup2:quality` | `make quality` | gofmt / tidy / vet / staticcheck / 复杂度、体积、克隆门禁通过；26 单元，传播成本 33.23%，基线保持 |
| `20261010T092842Z-d00e8b79cd9e` / `cleanup2:affected-e2e` | 5 条受影响旅程、旧映射拒绝、正式命令面、内部操作/阶段、usage 与 HTTP 对象坐标拒绝合同 | 15 个顶层用例，Go 56 个测试终态事件 / 1 包通过；复用测试执行器临时 OpenSearch，结束后回收 |
| `20261010T092937Z-a858d28457c6` / `cleanup2:coverage-contracts` | 覆盖证据维度、语义风险要求与完整命令面报告合同 | 3 项通过，无 skip；不把报告形状检查外推成全部命令动态覆盖 |

- 最终增量复核：轮初快照相比仅 11 个已有源文件变更，无新增或删除源文件；其余 1,852 个文件逐字节保留，OBJECT-INTERFACES-01 / DATASET-04 任务块与 5 个既有删除保持原状。独立复核确认原断言保留，无协议形状扩张或新特例；本次仅回写验收记录和任务状态，回写后再跑文档与质量门禁。

本轮保留的候选与债：

| 位置 | 信号 | 判定 | 处置 | 触发条件 |
|---|---|---|---|---|
| `cli/kc_test.go` 其余旧动作映射 | 仍有存量旅程和动态 argv 使用 | 不能由新拼写推定同语义或真实 transport 覆盖 | 保留；逐簇核对消费者、后态和独占风险再迁移 | 修改对应旅程或专门认领下一簇测试迁移 |
| `doc/engineering/test-catalog.md` C-08 与旧 `archive-repo` 接缝 | 测试接缝映射到 detach，不能证明 Repository 归档后的 COMMIT/PROPOSE 拒绝 | 用例要求仍有效，已有接缝的证据不足 | 保留 `REPOSITORY_ARCHIVED` 要求和现况；补真正 ArchiveRepository 与两类写拒绝证据后再裁决转换 | 触及仓归档生命周期或 C-08 旅程 |
| 同文 C-13、`audit --dataset` 接缝 | 当前 Catalog Audit typed client 不传 Dataset，行摘要仍写配方历史 | Dataset 配方历史和 Catalog authority 历史的证据混用待澄清 | 保留风险；按 owner/API 分清各自历史与独立断言，不在清理中改业务语义 | 触及 C-13 或 audit 旅程 |
| `webui/pub/{getting-started.png,logo.png,logo.svg,logo192.png,trophy.svg}` | 当前生产源码闭包及既有构建文本无引用，但 Vite 仍复制发布 | 可考虑收窄发布集合；外部直链与 vendor 来源约定未确认，既有构建也不是当前 WIP 的构建验收 | 保留原件，先选发布集合并补当前构建、浏览器与静态路由证据 | 发布体积治理或 vendor 子集改造 |
| `.validation/runs` | 盘点时 287 次运行，至少 14 个被源码文档引用，1 个未完成最终报告 | 原始运行输出无法由当前代码重建；没有保留/归档合同 | 全部保留，按引用、在途状态和归档规则回收 | 建立证据保留入口或空间预算触发 |
| `.session_tmps` 两份 Markdown、8 个 rejected architecture 侧车 | 独有草稿/审查资料，没有已归档或可重建证明 | 尚不能判为缓存残留 | 保留；核对原始输入、生成器与独有结论归档 | 专门本地资料回收 |
| `scripts/assemble-doc.sh` | 仓内无调用，声明外部 WorkSurface/Orchestrator 用途 | 仓外消费者未确认停用 | 保留；确认外部使用后更新合同或退役 | 外部工作流确认或重新接入 |
| AST 导出候选、webui vendor / 依赖全集 | 低引用或不在 KC 当前生产闭包 | 尚无逐符号退役证明或 vendor 子集合同 | 保留；公开合同、同包调用和升级/许可边界另审，不安装或卸载依赖 | 对应 API 退役、上游升级或专门瘦身 |

- 验收边界：本轮不代验收 OBJECT-INTERFACES-01 / DATASET-04，不运行真实 lakeFS/Gitea/native/FUSE 或模型部署验收；新建服务仅用于既有测试入口的 OpenSearch 验证夹具，不改环境供给合同。完整旧混合套件和未逐项复核的候选不计通过。
