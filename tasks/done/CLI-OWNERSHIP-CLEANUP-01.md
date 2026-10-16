# CLI-OWNERSHIP-CLEANUP-01 · 根 CLI 所有权收尾（2026-10-14）

- [ ] 本轮认领：按实际 owner 归位代码和细节测试，删除空壳与私有迁移转发，把旧嵌入分派及文件挂载夹具接线归测试装配；根保留全局身份、部署、宿主管理、静态模块装配与跨模块任务，保留既有公开入口和全部行为断言，不提交。
- 触碰 owner：CLI 的模块任务与全局上下文、Service 的共享业务入口、Capability Organization 的模块边界、Quality Loop 的信号闭环；设计目标沿既有合同，不新增协议、命令框架或公开业务形状。
- 用户追加的设计书范围：最终主产物为 `doc/README.md` 产品设计与使用书。Core Architecture 更新对象环境愿景、分层职责及面向对象的注册／环境管理思路，Object System 说明供给和使用闭环，Plugin Extension 说明作用域扩展及两处 Affordance，Terminology 同步规范名词；Wield 保持未来方向，申请访问衔接外部 owner／既有业务流程。Spring 容器配置、装配与管理的类比经官方资料核对，KC 的业务对象延伸明确作为设计选择，知识发布与运行装配分别负责。结构图保留可编辑 SVG 与 PNG 并内嵌 HTML；文档阅读入口补图片显示，不改变产品协议。
- 设计自检出口：图中狭义 Store 的“幂等回执”误归属及外部操作的普遍“可恢复执行”已当场纠正，回执沿实际写入账、恢复沿源端合同，并显式区分结果未知；资产与派生手册同步，不把图示扩大为已实现承诺。原 owner 不变量、作用域／发权边界与正文合同保留。
- 验证顺序：先用新增所有权守卫取得当前残留的失败证据，再迁移；执行受影响模块与宿主回归、架构/公开入口合同、文档检查及质量闸。真实部署与外部环境验收独立记录，不将跳过用例写为验收通过。
- 质量出口：无符号文件、恒空登记、测试专用生产转发与 owner 单元测试留根当场修；重复残留升级为 AST 所有权守卫。现存应用目录发布、治理、Hook/Gate 与证据任务按当前 owner 保留，不能仅按动词名称搬进 Object 基本 CLI。
- 已落实：根生产文件从 59 个降为 35 个；移除三组旧别名、空文件、恒空登记和未使用会话函数，产品分派不再带嵌入开关，kcfs 的本地夹具及隐藏 pin 任务仅在测试编译。公开命令与 11 个导出符号保持；49 个原测试名归实际 owner，另三组矩阵拆分／重命名，原断言保留。公开 kcfs 已拒绝的私有 pin 回放分支也已清理，其固定 pin 断言改走实际文件网关。
- 守卫：AST 检查空壳、私有原样转发、嵌入执行、根单元测试归属、私有函数生产引用，并保护既有公开类型；原 typed executor、非扫描及非维护断言沿真实 owner 路径保留。测试支持里的 Legacy OAuth 材料只保存历史断言，现行登录由实际 broker／刷新／会话回归验证，不计作生产兼容入口。
- 证据：改前快照、红守卫、各 owner 全包与宿主定向绿、自检与同口径耦合前后报告保存在 `.validation/cli-cleanup-20261014/`。Go 1.23.6 普通场景 21 pass／0 fail／0 skip；质量闸与文档检查通过。其完整混合合同仅 UI 重定向断言失败，且运行期间另一聊天增加对象引用测试，sourceChanged=true，不能作为冻结验收。专项 Taihu 入口及示例按真实 Server owner 归位，并增加实际脚本选择／失败传播回归；最终稳定候选使用项目原环境 Go 1.26 重跑普通场景与完整合同，质量工具使用 PATH 同步的 Go 1.23.6，不混用标准库。
- 存量债：`cli/webapp_embed_internal_test.go` 的 UI 补斜杠断言期待 307，Go 1.23.6 标准 ServeMux 返回 301，Go 1.26.0 返回 307；原测试逐字未变，独立 ServeMux 同样复现。判定为 Server HTTP 路由的存量跨工具链兼容差异，不以修改断言处理；下一次维护该路由／支持工具链合同时明确并固定状态码，跨支持版本验收，证据在 `toolchain-redirect/`。`README.md` 共享服务示例仍称 Gitea 主体为 `gitea:<user-id>`，判定为身份资料漂移，本轮不扩到身份行为，下一次维护身份登录资料时同步并检查引用。`object/README.md` 的 System 出版说明仍称“空 LakeFS/Gitea Snapshot”，旧存储名与当前唯一 lakeFS 不符；判定为原包用法存量漂移，登记留在下次该出版／部署说明维护时同步，身份认证的 Gitea 使用不受此债影响。
- 场景接线出口：v2 普通场景的六个叶失败及两个父失败指向同一旧数组断言；7 个 feature 的 10 个单对象 READ 断言块按已选定返回形状改为精确字段比较，命令、locator、权限、版本输入、每行字段与预期值逐字节保留。原 matcher 与合法数组断言保留；重复信号升级为场景解析守卫，先红后绿，证据在 `scene-read-shape/`。不改 Object 行为。
- HTTP 场景接线出口：v3 的普通 ProductScenes 已绿，唯一叶失败来自授权隔离 probe 的四个旧 object 输入；迁到同一业务对象的完整根引用，Dataset、三主体、发权操作及全部权限／完整 Canonical 断言逐字节保留。全树确认没有其它基本 HTTP PUT／REMOVE 表残留；快速契约复用公开 Object DTO 严格解码和完整引用解析，显式非法输入 probe 保留，授权 probe 要先具备合法请求。先红后绿证据在 `scene-http-reference/`，不改执行器映射、服务或正文 Oracle。
- 新静态测试归位：v4 的普通场景 21／0／0 与闸层同源通过；完整合同的组件 2866 pass 后，原根单元测试归属守卫正确拒绝新增 HTTP 夹具测试中的纯模块调用。通用严格 JSON 夹具校验归现有 `internal/testkit`，生产只用标准库且目标类型／语义校验由调用方注入；9 个请求与引用变体归其测试，4 个场景选择／错误用途变体留 CLI 验证选择及失败传播，原 13 变体语义完整保留。全树仍用真实公开 DTO／Parser，原所有权守卫及其单位／集成证据矩阵未变且已绿，没有加白名单或虚假宿主调用。证据在 `http-guard-ownership/`。
- 完整入口收尾：v5 闸层、验证合同 50／0／0、普通场景 21／0／0 同源通过；完整合同 3997 pass／18 fail（含父项）／12 个既有环境门控跳过，源码未变。失败揭示旧接线：根全局 catalog／pin 拒绝被模块闭集校验抢先；HTTP 证据重复认领 Object PUT、遗漏实际 Writer commit；系统能力入口派生文件未刷新；混合基点负例的迁移误删显式 repo；消费帮助、场景静态守卫仍匹配旧 argv／数组。均按实际 owner 修正，95 路由及 46＋50 证据断言保留，真实提交旅程认领 commit，不用人工回执冒充生产成功。原范围外 READ 是无正文的空 union，三个迁移新增的 FORBIDDEN 期望修正为精确引用未解析并保留无泄漏断言；真正授权拒绝仍为原 FORBIDDEN。跨模块旧参数拒绝补实际 Run／HTTP 不可达回归，先红后绿，证据在 `final-contract-wiring/`。
- CAS 出口：原 `TestCatalogRepoWriteFlow` 的 expected-only 过期 CAS 保留输入及 NON_FAST_FORWARD 断言；迁移接到 MaintenanceExecutor 后暴露空 Base 被当前 Head 抢填的默认依据错误。按既有 Writer.CommitIntent 合同修到 Object service：账本重放之后，空 Base 先沿显式 Expected，再退到当前 Head；不在 CLI 补业务默认值。新增真实 lakeFS HTTP 夹具的 PUT／REMOVE 过期预期回归，先红后绿，并核对发布 Head 和内容未变，包 README 同步；不扩到其它对象迁移或部署工作。
- 当前验收边界：CLI 清理、场景接线、owner 设计、两图及最终 HTML 已稳定，用户最新注册对象主线、书名与导航已通过独立语义自检及实际显示。图片阅读的原断言与新增实际 GET／HEAD、转义、代码隔离、路径拒绝回归先红后绿；新资产路径同时暴露既有 symlink 仓外逃逸，已修复并保留红例。旧 v1～v5 只为前态；最终 v6 候选与正式结果统一保存在 `.validation/cli-cleanup-20261014/final-joint-v6/`，普通场景、完整合同、文档与质量闸须绑定同一源码状态，不以源码变化或环境跳过冒称完整验收。对象 owner 已按用户指示停止并交接 dsh，临时引擎保留且运行验证已停止；本聊天完成 CLI／设计书范围的检查，不代替其尚未闭合的对象专项、race、部署或 controlplane 裁决。暂不勾选完整联合验收。

## 关闭

2026-10-15 拆分 `TASK.md` 时关闭，未按原条目验收。实现已落地；整体验收并入 `FINAL-ACCEPTANCE-01`，剩余目录归位并入 `ARCH-LAYOUT-01`。登记的存量债：UI 补斜杠状态码 → `DEBT-UI-REDIRECT-STATUS-01`；README 与 `object/README.md` 的 Gitea 资料漂移 → `GITEA-LOGIN-RETIRE-01`。
