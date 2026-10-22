# DEBT-DATASET-HISTORY-AUTH-01 · Dataset 交付范围不能代替来源历史授权

## 债

- 位置：`doc/design/object-versioning.md` 的历史分页；
  `object/reader/dataset_repository.go` 的 `Log`；
  `invocation/session_helpers.go` 的 `AllowedRepoAspectRead`。
- 信号：提案允许 Dataset 范围查询对象历史，仅明确拒绝文件切片；现有包装也仅在文件切片时拒绝，
  整仓交付则转发来源历史。Dataset 消费许可可能通过仓读取的辅助判定。
- 判定：2026-10-20 静态审查确认，固定提交的整仓文件交付并不蕴含之前提交的历史授权。
  仅交付 c2 的消费者可能看到未交付 c1 的提交坐标、存在或删除状态、内容与声明摘要；
  当前历史模型不返回正文、作者或提交消息，不将其扩大描述为正文泄露。
- 处置：设计明确 Dataset 消费授权与来源仓历史授权的区别；来源历史单独检查仓权限，
  仅有 Dataset 消费权时使用合法发布版本之间的差异比较。按采用后的规则修复调用链并增加回归。
- 触发条件：`OBJECT-VERSIONING-01` 开工时并入，或任何修改 Dataset 历史入口与授权的任务开工时并入。

## 目标

交付某个提交下的全部文件，也不会自动开放来源仓未交付的历史。

## 依据

- [对象版本与读写](../../doc/design/object-versioning.md)：历史与差异、历史分页。
- [Dataset](../../doc/design/dataset.md)：固定版本与独立消费权限。
- [权限](../../doc/design/permissions.md)：仓的内容与历史、Dataset 的交付范围。

## 验收

- 仅有 Dataset 消费权时，整仓根目录交付与文件切片均不能读取未交付的来源历史。
- 来源仓历史权限单独授予、撤销以及游标续页遵守同一授权边界。
- 两个有权消费的发布版本仍可比较，不额外要求来源整仓读权。
- 修复随受影响回归与必要授权合同一起验证，通过 `make check-docs`、`make quality`，无 skip。

## 前置

与 `OBJECT-VERSIONING-01` 的历史接口审查一并确定规则。

## 不做

不引入对象级历史权限，不将 Dataset 消费权改成来源仓读权。

## 本轮范围决定（2026-10-20）

依据既有权限设计中仓内容与历史、Dataset 固定交付的边界，先独立修复来源历史授权，不等待整个
`OBJECT-VERSIONING-01`。历史每次请求与续页均调用原仓授权检查，Dataset 消费与 object.read 不替代
object.history.read；保留文件切片拒绝来源历史的现有合同。
两版发布内容对比仍由 `DEBT-DATASET-VERSION-CONTRACT-01` 和对象版本接口审查承接，
本轮不为修复授权漏洞新增公开比较入口。

## 修复进展（2026-10-20）

- Dataset 与单仓历史查询都在每个实际来源历史读取前，经既有 `Authorize` 单独检查
  `object.history.read`；该查询不携带 Dataset 消费范围，避免消费权回退。拒绝返回 FORBIDDEN，不冒充空历史。
- 删除历史读取对 `AllowedRepoRead` 的误用：来源正文权不替代历史权，只有历史权也无需额外正文权。
- 新增真实 HTTP 回归 `TestDatasetHistoryRequiresSourceHistoryGrantOnEveryPage`，覆盖整仓/切片仅消费权拒绝、
  正文权不能替代历史权、历史权单独成功、合法分页、撤权后旧游标拒绝，以及 Dataset 正文仍可读取。
- 四项 CLI 定向回归、invocation/reader/service/object-cli 包回归与质量门禁通过。
- 原工作区的完整契约首轮在系统仓源码并行变化时触发嵌入出版物与源文件不一致；报告 sourceChanged=true。
  `TestSystemPublicationMatchesCanonicalFilesAndSchemas` 当前单独复验通过，不判为出版实现缺陷。
  原工作区 `make test` 的测试通过，但同样因 sourceChanged=true 不能作固定候选验收。
  后续在固定源码副本验证，不覆盖其它在途编辑。

## 已验证范围（2026-10-20）

固定源码副本的 `make quality`（含 check-docs）通过；`make test` 通过、sourceChanged=false。
完整 `make test-contracts` 为 5117 pass / 0 fail / 14 外部 live skip，退出 0、sourceChanged=false；
54/54 公开命令的端到端覆盖满足边界要求。新历史授权 HTTP 回归与原获权分页旅程通过，无 skip。
七个本项实现、测试与说明文件在验证结束后逐一与原工作区核对哈希一致。

证据统一保存到 `.validation/runs/20261020-history-auth/`，包括冻结清单、完整报告、质量日志与定向回归；
原工作区受并行编辑影响的运行不拼入本次通过记录。未提交。

来源历史越权缺陷已修复；任务保留 doing：外部 live 未验范围不冒充无 skip 全验收，
两份发布版对比尚由 `DEBT-DATASET-VERSION-CONTRACT-01` 承接，不在本轮授权修复中新增接口。

## 交付（2026-10-21）

实现已在工作区落地并经当前源码复核（`object/trusted_schema.go` 的一致性检查覆盖
`TrustedSchemaOperations()` 完整集合；历史授权每个来源读取前单独检查 `object.history.read`；
未应用写入证明经统一错误信封传递）。

当前源码统一门禁：`make check-docs` PASS、`make quality` 通过
（`units=86 propagation=13.69%`，ratchet held）、`make test` 0 FAIL / 0 SKIP、
`make test-contracts` 0 FAIL / 18 SKIP，e2e `53/53 successful`。

18 项 SKIP 经逐条核对全部为既有环境分级项（lakeFS 独立部署、太湖认证、Luna 重排、
HTTP state runtime 独立栈、规模与吞吐），不是本项遗留的断言缺口。
按既定判断，环境分级跳过不计入本项阻塞。
