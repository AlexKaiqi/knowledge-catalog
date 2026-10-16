# FILE-ACCESS-01 · 从对象说明发现并下载引用文件

## 目标

Agent 读到 definition 中普通的文件路径和来源版本后，能从公开帮助找到单文件读取入口，
用既有 lakeFS 文件能力下载原始字节。对象层不解释资源字段，不自动下载，也不要求先创建 Dataset。

## 依据

- [Snapshot](../../doc/design/snapshot.md)：按仓、固定版本和路径读取字节。
- [Affordance](../../doc/design/affordance.md)：帮助、系统仓与真实入口一致。
- [对象](../../doc/design/object.md)：正文和来源依据保留，业务结构由 Schema 说明。
- [权限](../../doc/design/permissions.md)：文件读取使用当前仓权限，引用不发权。

## 验收

- 公开对象读取后，消费者用正文中的资源仓、commit、path 下载单文件，原始图片字节保持一致。
- 固定版本、范围参数、跨块来源一致性、撤权、对象读权不隐含文件读权、不覆盖本机目标均有回归。
- 从根帮助、文件帮助与系统仓可发现调用入口和参数来源，不需要阅读源码或猜真实存储仓名。
- 受影响测试与契约、`make check-docs`、`make quality` 通过且无 skip；真实部署验收另行说明。

## 前置

无。

## 不做

不新增图片类型、资源执行协议、对象字段自动解析、文件上传、Repo 目录枚举或默认最新版选择。
不修改其它进行中的插件、索引和 Agent 试跑工作；不提交。

## 交付与实际验证

已实现：现有 `files read` 增加逻辑仓、固定 commit 和仓内路径模式；复用 Snapshot 的 lakeFS
读取和唯一下载器，按当前原仓权限逐块交付，不创建 Dataset 或自动解释对象字段。根帮助、
files/repository/versions、系统仓与真实命令同步；公开响应 DTO 已登记在架构闭集。

先红后绿证据：旧 CLI 拒绝 commit 参数，旧 HTTP 拒绝 commit 字段，旧帮助缺少按仓下载说明。
新增消费者旅程通过公开 Writer/Server 读取图片 definition 后取正文坐标，再下载并逐字节核对 PNG；
对象说明与资源使用不同仓、不同版本，无索引和图片处理器。只有对象读权和文件撤权后均拒绝下载。

已通过：Snapshot CLI/SDK 与系统仓全包测试；文件服务 Repo/Dataset/native 相关合同；公开 CLI
单文件、分页/版本、并发目标保护、帮助、系统入口漂移及场景关联检查；Snapshot 架构闭集与
Server Snapshot/DatasetFile/native 合同；场景 product/views 检查；`make check-docs`。
这些验证均未使用 skip。最终 `make quality` 也已通过。真实 lakeFS 部署验收未执行。

未完成全仓验收：完整架构测试还有在途材料的术语失败，登记 `DEBT-TERMINOLOGY-TRIAL-01`。
早先质量门禁的依赖图基线阻塞由并行插件结构任务处理，本轮复跑通过，`DEBT-COUPLING-RATCHET-01`
已关闭。本任务保持 doing，不把定向通过写成全仓通过。发现的既有文件访问账接线缺口登记
`DEBT-FILE-ACCESS-EVIDENCE-01`，本轮不宣称访问审计已闭环。未提交。

本轮稳定候选门禁：`make check-docs`通过；`make quality`在 `declarativeindex/plugin/plugin_control_recovery_fixture_test.go` 的gofmt检查失败，归仍在途的 `INDEX-DECLARATION-01`，未替其它线格式化。证据 `.validation/runs/20261015T123258Z-42a42700fe5d/report.json`，`sourceChanged=false`。本任务继续doing；尚未达到统一门禁验收。

## 本轮最终统一门禁（2026-10-15）

统一证据与命令见 门禁索引（`.validation/runs/20261015T120241Z-task-orchestration/final-gates.json`）。最新 `make quality`（含 check-docs）通过；完整架构 648 pass/1 fail/0 skip（术语，归 DEBT-TERMINOLOGY-TRIAL-01）；`make test` 18 pass/2 fail/0 skip（同一 Dataset 文本能力场景及父级，归 INDEX）；`make test-contracts` 的 component 为 2349 pass/67 fail/6 skip，boundary/e2e 未启动。上述四次均源码稳定；合同失败分组见 报告（`.validation/runs/20261015T135236Z-03378168af4a/findings.md`），分别归 INDEX、库存消费债及回环 HTTP 资源调查，不由本任务替其它线修复。

最终相关定向结果及真实部署边界分别保留在任务专项证据与门禁索引：定向通过不替代完整入口，现有 local 栈 readyz 503，117 项真实验收均 error/not_started。全量门禁尚未全绿，依本轮约定保留 doing；未提交或安装依赖，未降低检查阈值或重置已有环境。

## 固定成员文件授权补齐（2026-10-16）

目标是 Dataset 固定成员的完整消费许可同时适用于目录发现与实际字节读取；实际依据为权限篇“权限每次请求重新检查”“Dataset 消费不开放来源整仓”，以及 Snapshot 固定版本读取。未改设计。

先红证据 `.validation/runs/20261016T013001Z-b1406590637f/`：完整固定成员授权能列出 Dataset，实际文件读取却因丢失 Ref 返回 FORBIDDEN。当前 `OpenFiles` 的成员 `file.read` 查询补入已接受 pin 中的固定 commit，保留当前主体、Dataset 与原仓边界，不用 HEAD 代替。此处沿本任务既有文件服务修改续修，库存任务只追加其拥有的正式 CLI 回归。

实际工作区回归 `.validation/runs/20261016T013529Z-9f161f2fe15a/` 53 pass、0 fail/skip，sourceChanged=false，核对固定成员读取与维护者同修订读取的完整字节、来源和版本相同。该组是定向范围，统一门禁另行记录。未提交。

较宽组合先暴露旧精确 Query 断言缺固定 Ref（`20261016T013549Z-f4163929b847` 的唯一失败）；保留原主体、pin、权限先于读取、零源访问等全部断言，仅补期望 `c1`。实际 Snapshot 离线合同复验 `.validation/runs/20261016T013743Z-8d134b2b9218/` 为 228 pass、0 fail/skip，固定成员及库存短组合 `.validation/runs/20261016T013743Z-1575b289cd89/` 为 54 pass、0 fail/skip，两次均 sourceChanged=false。

- 2026-10-16 L0 固定快照门禁：索引（`.validation/runs/20261016T015828Z-l0-snapshot-gates/gate-index.md`）；本任务无新增失败归因；统一门禁仍有主线失败、文件超限及 6 项 live 未验证，保留 doing。

## 清河媒体 demo 的发现与下载补齐（2026-10-16）

本轮目标是让已有文件能力在实际 demo 中形成“读对象说明、取文件路径、固定同版下载”的完整链路。依据为 Snapshot、对象、Affordance、权限及 Dataset 的既有设计，未改设计与公开协议。

`cli/example_world_test.go` 原先直接写死成片和字幕路径；现对 Dataset 修订 1/2/3 逐次公开读取媒体 definition，从其 file/cover/subtitles 取路径，再下载同修订的视频、PNG 封面和字幕，逐字节核对原文件。下载回执同时核对 Dataset、修订与路径。保留字幕更正、旧修订、vision 范围、来源仓拒绝与发布计划重放等断言；同目录 examples README 已补全固定修订的完整调用示例并重读。

实际工作区的稳定定向记录：

- `.validation/runs/20261016T033508Z-2bbaf48bd267/`：两项受影响 demo 旅程，5 pass / 0 fail / 0 skip。
- `.validation/runs/20261016T033523Z-5e156db5bf4b/`：Snapshot CLI、SDK 与系统仓全包，157 pass / 0 fail / 0 skip。
- `.validation/runs/20261016T033523Z-936540d354ea/`：Snapshot 服务显式离线合同，228 pass / 0 fail / 0 skip；真实部署旅程不在所选范围。
- `.validation/runs/20261016T033305Z-30f436cc30d1/`：相关 HTTP、正式 CLI、权限与文件访问账，65 pass / 0 fail / 0 skip。

上述四组均 sourceChanged=false。较宽 CLI 场景关联组合的 28 个事件及最终含 Schema 复用的 130 个事件均 pass，但运行期间其它源码变化，记录分别为 `.validation/runs/20261016T033523Z-f60ca45216c7/`、`.validation/runs/20261016T033654Z-fb521ba7bf2c/`；不拿这些 sourceChanged=true 的运行证明固定树验收。场景 product/views 声明检查通过，不表示执行完成。

工作区阶段的统一门禁未通过：当时 `make quality` 中 check-docs 与常规质量项通过，随后耦合门禁失败；typed 架构合同与验证库存也有检索迁移接缝。已由 `DEBT-INDEX-CONTRACT-GATES-01` 并入 `INDEX-SEARCH-SPACE-01`。完整架构宽运行在 typed 失败后继续扫描未入库的本机 realcase 旧材料，已主动中止并保留诊断，由 `DEBT-ARCH-LOCAL-DUMPS-01` 承接扫描边界，不改守卫换绿。首轮 U1000 不再出现在后续 quality 中。

本轮仅修改 demo 回归、示例说明与任务记录；未提交、未安装依赖，真实 lakeFS 部署验收未执行。本任务继续 doing，不将可用能力和定向通过写成统一验收完成。

### 固定源码最终复验

为避免并行改动混入结果，复制 tracked/non-ignored 文件并核对原工作区复制前后及副本的源码指纹完全一致。记录 `.validation/runs/20261016-file-access-demo-final/snapshot.json`；本轮两个 demo 文件与副本的内容摘要另行复核一致。副本沿用相同本机依赖与测试环境，不含被忽略的 realcase 导入数据。

- `.validation/runs/20261016T033922Z-604f0aa5c332/`：公开 PNG 资源下载、清河媒体与工具旅程、共享下载 helper 的 Schema 复用回归，130 pass / 0 fail / 0 skip。
- `.validation/runs/20261016T033945Z-7e8f2b04894e/`：`make quality` 完整通过，包含 `make check-docs`、常规质量项与耦合门禁。检索任务已在复制前收紧并修复基线，本任务未改基线。首次副本检查漏带三项历史证据链接，是副本准备错误；补入同一原文件的只读引用后复验通过，原失败记录保留。
- `.validation/runs/20261016T034017Z-c90b588d0fb7/`：完整架构 670 pass / 3 fail / 0 skip。失败为旧 typed 搜索执行器要求、检索帮助中的业务例，以及 I-21 已失效测试引用；均已在 `INDEX-SEARCH-SPACE-01` 的“对象发布收尾发现的门禁信号”中承接。验证库存检查同样只报 I-21 的两个失效引用。

上述三次最终记录均 sourceChanged=false。本条专项与 quality 已通过；完整架构仍未全绿，保持 doing。未执行真实部署验收。

- 2026-10-16 分源门禁：索引（`.validation/runs/20261016-environment-recovery/gate-index.md`）；bf99未包含随后补强的媒体同修订下载回归，保留其独立固定源码证据；统一完整门禁未齐，继续 doing。

## 交付

2026-10-16 按本条自身验收收口：公开对象说明可提供资源仓、固定 commit 与路径，再经 Server 下载原始字节；固定修订、范围、分块来源一致性、撤权、跨仓权限及不覆盖本机目标均有回归。根帮助、模块帮助与系统仓说明已同步；清河媒体从每版 definition 取得视频、封面和字幕路径，再下载同修订文件。

已验证：固定源码媒体与公开文件旅程（`.validation/runs/20261016T033922Z-604f0aa5c332/report.json`） 130 pass / 0 fail / 0 skip；Snapshot CLI、SDK 与系统仓全包（`.validation/runs/20261016T033523Z-5e156db5bf4b/report.json`） 157 pass / 0 fail / 0 skip；Snapshot 显式离线合同（`.validation/runs/20261016T033523Z-936540d354ea/report.json`） 228 pass / 0 fail / 0 skip，均 sourceChanged=false。较后的 374 来源合同报告（`.validation/runs/20261016-mvp-unified-043602Z/gates/20261016T044027Z-d94f92fb680a/report.json`） 中本条文件读取、下载、帮助和媒体回归也均通过；复核其文件实现与媒体回归字节保持，新增的发布重放和拒空身份测试不改变本条文件能力。b16 来源 quality（`.validation/runs/20261016-mvp-priority-closeout/gates/20261016T045451Z-9e0b9c81b729/report.json`） 包含 check-docs，稳定通过。

这些是各自明确来源的专项及增量证据，不组合宣称新候选全仓全绿。自身相关合同无 skip；旧宽报告的其它 live skip 不计成本条通过或阻碍。真实 lakeFS 部署验收仍单独进行，本条未据此宣称部署通过。未提交、未安装依赖；本节仅补齐交付事实，目录由统一收口处理。
