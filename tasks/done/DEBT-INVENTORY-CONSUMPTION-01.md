# DEBT-INVENTORY-CONSUMPTION-01 · 库存只列当前可消费的 Dataset

## 债

- 位置：`snapshot/service/inventory_http.go` 的库存 Dataset 摘要、`snapshot/service/dataset_http.go` 的 Dataset 列表。
- 信号：2026-10-15 产品书只读核对中，给调用者 `inventory.read` 与一份 `visible` Dataset 的消费许可，两个 GET 列表仍返回没有消费许可的 `hidden` Dataset；证据见 `.validation/runs/20261015T120939Z-product-claims/inventory-contract-red.log`。
- 判定：真缺口；库存访问权不等于任意交付的消费权，与发现设计“先列可消费 Dataset 再搜索”不一致。不能用正文读取会拒绝来解释列举正确，也不能隐藏所有库存绕过筛选。
- 处置：两个列表沿同一当前授权边界筛选已接受的 Dataset 摘要，保留默认可读政策和现有返回形状；建立消费许可、部分许可、撤权和公开交付的自动回归。不读取来源内容，不把来源读权推成 Dataset 许可。
- 触发条件：最终发现验收、对外发布产品书，或下次修改库存列表前。

## 目标

已获库存访问权的调用者只看到当前能完整消费的 Dataset；两个列表结果一致，默认可读交付可发现，撤权后下一次列表立即反映变化。授权存储故障明确失败，不伪装为空列表。

## 依据

- [发现](../../doc/design/discovery.md)：找到 Dataset、范围先于搜索。
- [权限](../../doc/design/permissions.md)：三种资源、Dataset 默认可读、按请求重新求值。
- [Dataset](../../doc/design/dataset.md)：独立消费权限与退役。
- [测试与验证](../../doc/engineering/testing.md)、[工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- `GET /snapshot/v1/inventory` 与 `GET /snapshot/v1/datasets` 在相同主体与政策下返回相同可消费 Dataset；无权、仅管理、仅解析、仅部分来源文件许可和已退役交付不进入名单。
- 当前 Dataset 解析与文件消费分别授权；仓读权本身不授予 Dataset 消费。部署默认可读交付不要求额外显式 grant，且不因此授予来源仓读写或 Dataset 管理。
- 同一运行实例的后续请求看到消费许可撤销；`inventory.read` 自身仍是入口条件，未认证或无库存权按原合同拒绝。
- 列举不探查来源 HEAD、提交或文件正文，不触发发布/索引准备；保留既有摘要、元数据和确定顺序。后端授权错误原码返回，不当作无权或空名单。
- 相关 Snapshot service、KC Server/CLI 回归以及 `make check-docs`、`make quality` 全绿，无 skip；真实部署结果单独报告。

## 前置

无行为前置。2026-10-16 已获准在分派范围保留原未提交改动追加修复；`snapshot/service/dataset_http.go` 的原发布准备与来源冻结改动完整保留。本条冻结后，再由统一门禁和共享文件协调解锁后续重叠任务。

## 不做

不改库存读取授权的默认政策，不改管理详情、来源仓库存或消费文件合同；不扩张公开接口，不把产品审计变成行为实现，不提交或重置部署。

## 进展（2026-10-15）

- 实际触碰的设计为发现“找到 Dataset”、权限“三种资源”“范围先于搜索”和 Dataset“消费”；不改设计或公开形状。行为实现由本条独立拥有，`DEBT-PRODUCT-CLAIMS-01` 的审计表与 `FINAL-ACCEPTANCE-01` 的前置已按任务 ID 对齐。
- 开工状态与原文件保存于 `.validation/runs/20261015T121804Z-inventory-consumption/baseline/`：`inventory_http.go` 干净，`dataset_http.go` 已有他人未提交的发布准备/依赖冻结改动。因此尚未修改两份工作区生产文件；两个端点必须共用筛选，不能为绕过脏文件另造中间层特例。
- 新增 `snapshot/service/inventory_consumption_test.go`：两个列表覆盖无消费权、仅管理、仅解析、仅读文件、仅仓读、部分成员、全部固定成员、错误来源版本和退役；同一实例撤权，库存权和认证仍独立检查，授权故障保留真实错误。列举断言两个来源均无版本/正文探测、权威无变化，保留已接受摘要与元数据。
- 新增 `cli/inventory_consumption_test.go`：复用现有清河例子 Server 与进程内 lakeFS 夹具，部署只默认公开 `store-oncall`，两个 SDK 列表仅显示该交付；来源仓读权不增加名单，正式 grant 增加 `qinghe-analysis` 后可见，撤销立即隐藏，退役默认可读交付后名单为空；实际消费继续拒绝已撤权交付。
- 原工作区定向测试先红：`baseline/consumption-regression.log` 记录无权条目和退役条目仍可见、撤权未反映、授权故障未返回，以及 KC 真实列表暴露三个 Dataset 而非仅默认公开的一个。
- 拟议共享修复保存为 `consumption-filter.patch`：库存已有冻结定义先检查当前 `dataset.resolve`，复用 `authorizeCheckedFiles` 对已接受来源的实际 Repository/Commit 检查完整文件消费权；只有 `FORBIDDEN` 视作不可见，真实授权错误原样返回。退役条目排除，不解析来源 HEAD、不读文件，不走库存发现的授权捷径。
- **补丁预演通过，尚非工作区验收**：证据目录 `proposed/overlay.json` 仅替换生产文件副本，新回归见 `proposed/consumption-regression.log`；相关既有库存、默认可读 Dataset、元数据、目录交付和 CLI 消费回归见 `proposed/related-regressions.log`（Snapshot service、CLI 均绿）。本轮没有完整架构、`make test`、真实部署或共享 OpenSearch 运行，不声称这些范围通过。
- 等待：`snapshot/service/dataset_http.go` 的既有在途改动释放后应用精确补丁，再在实际工作区跑定向回归及统一 `make check-docs`、`make quality`。本条继续留在 doing，未提交、未安装依赖。

## 本轮最终统一门禁（2026-10-15）

统一证据与命令见 门禁索引（`.validation/runs/20261015T120241Z-task-orchestration/final-gates.json`）。最新 `make quality`（含 check-docs）通过；完整架构 648 pass/1 fail/0 skip（术语，归 DEBT-TERMINOLOGY-TRIAL-01）；`make test` 18 pass/2 fail/0 skip（同一 Dataset 文本能力场景及父级，归 INDEX）；`make test-contracts` 的 component 为 2349 pass/67 fail/6 skip，boundary/e2e 未启动。上述四次均源码稳定；合同失败分组见 报告（`.validation/runs/20261015T135236Z-03378168af4a/findings.md`），分别归 INDEX、库存消费债及回环 HTTP 资源调查，不由本任务替其它线修复。

最终相关定向结果及真实部署边界分别保留在任务专项证据与门禁索引：定向通过不替代完整入口，现有 local 栈 readyz 503，117 项真实验收均 error/not_started。全量门禁尚未全绿，依本轮约定保留 doing；未提交或安装依赖，未降低检查阈值或重置已有环境。


## 实现与定向验证（2026-10-16）

- 目标及实际设计依据为发现篇“找到 Dataset / U10”、权限篇“三种资源 / 范围先于搜索”、Dataset 篇“消费 / 退役”；未改设计、公开 DTO 或命令形状。
- 在保留已有未提交改动的授权下，将旧预演补丁复核后应用到实际工作区。两个列表共用 `consumableDatasets`：排除退役记录，逐请求检查 `dataset.resolve`，再复用 `authorizeCheckedFiles` 对已接受修订的全部固定 Repository/commit 检查 `file.read`。仅 `FORBIDDEN` 作为不可见，授权存储故障保留原错误；不走 `inventoryDiscovery`，不调用来源解析、HEAD、正文或发布准备。
- 变更前原文件和 diff、最小增量补丁及证据索引位于 `.validation/runs/20261016T012008Z-inventory-consumption-workspace/`。`preservation.txt` 精确核对 `dataset_http.go` 只改变列表筛选，原发布准备、选定坐标授权与冻结逻辑字节保留。
- 当前工作区先红：原实现回归（`.validation/runs/20261016T012008Z-f5fc375efc1d/report.json`） 为 7 pass / 26 fail / 0 skip，源码稳定；两个列表泄露未获完整消费许可和退役交付，撤权与授权故障未生效，真实 Server 列出三份而非仅公开的一份。
- 修复后实际工作区 消费回归（`.validation/runs/20261016T012051Z-41df0fa8f85f/report.json`） 为 37 pass / 0 fail / 0 skip，源码稳定；包含两个列表、部分/完整/错误版本成员、默认公开交付、同实例撤权、`inventory.read` 独立入口及缺身份拒绝。授权故障扩为解析与文件两阶段，并断言没有借用库存发现或临时定义授权；扩展断言对原生产代码副本的 反例（`.validation/runs/20261016T012223Z-a435db3e20de/report.json`） 为 7 fail，明确仅为原实现 overlay 反例。
- Snapshot 明确离线合同（`.validation/runs/20261016T012222Z-042761a3192e/report.json`） 为 228 pass / 0 fail / 0 skip，源码稳定；选择全部非 live 的现有 service 合同。另一次 service 全包运行 228 pass / 0 fail / 1 skip，既有 `TestLiveSnapshotPublicationJourney` 无部署而未运行，且检测到全仓源码变化，保留原始证据、不将其计入验收。
- CLI / Server / Snapshot 客户端与命令相关定向（`.validation/runs/20261016T012126Z-5b676697957d/report.json`） 的实际路径以专项 `evidence-index.json` 的 `related.path` 为准：67 pass / 0 fail / 0 skip，但运行期间全仓源码变化，不能作为一个固定工作区状态的验收；本任务运行期间未改源码。固定树复验由统一稳定窗口补充。
- 实现已落地，验收尚未完成。未跑本轮完整架构、`make quality`、`make check-docs`、`make test`、`make test-contracts` 或真实部署；未使用共享 OpenSearch、未安装依赖或提交。保持 doing，冻结拥有文件等待统一门禁。


## 交付（2026-10-16，实现与定向验证）

实现范围：两个库存列表已共用当前消费授权筛选，保留已接受摘要、元数据、排序和公开返回形状。过滤退役与没有完整消费许可的 Dataset，重新求值解析许可和全部固定来源文件许可；真实授权错误继续失败，不伪装为空名单。不探查来源 HEAD/提交/正文，不触发发布准备，不使用 `inventoryDiscovery`，不扩大来源仓读取或管理许可。

追加收口的宿主接缝：`invocation/dataset_access.go` 的 `DatasetFSMayReadRepository` 原先丢掉固定 Ref 并把 `ReadAllow` 错误转成 false；现在沿既有 `Authorize` 对明确的 as/Dataset/Repository/Ref 求值，仅 `FORBIDDEN` 返回不可消费，其他错误原样传播。文件坐标不采用 Object/Aspect 限定，保留本地直接 owner 和默认可读 Dataset 政策，没有扩张公开 API。正式新增 `invocation/inventory_file_policy_regression_test.go` 覆盖相同/不同/缺失固定版本、错误传播、本地语义及应用限定不冒充文件许可；既有范围断言保持。

真实固定成员回归：CLI 通过实际 Server 发布例子并取得已接受 pin，以每个 Repository 的确切 commit 发文件许可。两个列表现在可见该交付，实际 `files read --dataset qinghe-analysis --revision 2` 读到的内容、来源和版本与维护者控制读取完整相同；撤销任一成员后再次隐藏。初次真实读取反例为 0 pass / 1 fail（`.validation/runs/20261016T013001Z-b1406590637f/report.json`），仅是追加读取断言的 overlay，清楚暴露名单与文件消费接线的差异。`OpenFiles.mayRead` 补充已接受 pin 的 Ref 及其原精确 Query 断言补齐由 `FILE-ACCESS-01` 交付；本条只拥有库存筛选、宿主授权和 CLI 正式回归，不重复认领该文件消费实现。

已验证范围（计数为 Go 测试事件，范围有重叠，不累计）：

- 原宿主实现的正式工作区红例：13 pass / 4 fail / 0 skip（`.validation/runs/20261016T012709Z-7e45511db630/report.json`），源码稳定；包含真实 KC 固定成员许可误隐藏与后端授权故障被吞。
- 最终实际工作区短组合：54 pass / 0 fail / 0 skip（`.validation/runs/20261016T013743Z-1575b289cd89/report.json`），源码稳定。覆盖库存两个端点、当前授权与撤权、默认公开政策、完整固定成员真实读取、宿主文件授权及原拒绝前零读取合同。
- 最终 Snapshot 明确离线合同：228 pass / 0 fail / 0 skip（`.validation/runs/20261016T013743Z-8d134b2b9218/report.json`），源码稳定。选择全部非 live 的现有 service 合同，未把真实部署用例计作通过。
- 宿主函数修复后的 invocation 全包：101 pass / 0 fail / 0 skip（`.validation/runs/20261016T012803Z-c399902005fe/report.json`），该运行源码稳定。
- 较宽五包旧运行：523 pass / 1 fail / 0 skip（`.validation/runs/20261016T013549Z-f4163929b847/report.json`），运行期间全仓源码变化，保留为失败证据。唯一失败是 `TestSnapshotFileRechecksCurrentAuthorizationBeforeBytes` 原精确 Query 预期未带 Ref，归 `FILE-ACCESS-01`；补齐固定 Ref 后由上述最终短组合与 Snapshot 合同复验，原拒绝/范围/当前授权/零读取断言保留。不把旧运行记为固定树通过，也不重复宽 CLI 组。

专项目录 `.validation/runs/20261016T012008Z-inventory-consumption-workspace/` 保存原文件、最小增量补丁、原发布准备代码保留核对、冻结指纹和完整 `evidence-index.json`。实现已落地且相关定向全绿；全量验收仍等待统一稳定窗口的 `make check-docs`、`make quality` 及完整入口。真实部署未执行，不以定向替代；保持 doing，源码与任务说明冻结，未提交、未安装依赖或使用共享 OpenSearch。

- 2026-10-16 L0 固定快照门禁：索引（`.validation/runs/20261016T015828Z-l0-snapshot-gates/gate-index.md`）；本任务无新增失败归因；统一门禁仍有主线失败、文件超限及 6 项 live 未验证，保留 doing。

- 2026-10-16 分源门禁：索引（`.validation/runs/20261016-environment-recovery/gate-index.md`）；当前授权筛选及固定成员实际读取的专项证据保留；完整入口尚未形成同源全绿证明，继续 doing。

## 交付

2026-10-16 按本条自身验收收口：两个库存列表共用逐请求的完整消费授权筛选，排除退役、无权及部分许可交付；保留默认公开政策、摘要、元数据和顺序。解析与固定来源文件许可分别检查，撤权立即反映；授权存储故障原样返回，不探查来源 HEAD、提交或正文，不触发准备，也不借库存发现扩大消费权。

已验证：最终实际工作区短组合（`.validation/runs/20261016T013743Z-1575b289cd89/report.json`） 54 pass / 0 fail / 0 skip；Snapshot 明确离线合同（`.validation/runs/20261016T013743Z-8d134b2b9218/report.json`） 228 pass / 0 fail / 0 skip；宿主 invocation 全包（`.validation/runs/20261016T012803Z-c399902005fe/report.json`） 101 pass / 0 fail / 0 skip，均源码稳定，范围重叠不累计。较后的 374 来源合同报告（`.validation/runs/20261016-mvp-unified-043602Z/gates/20261016T044027Z-d94f92fb680a/report.json`） 中本条库存测试与子项共 37 个事件全部通过；六个直接相关的筛选、宿主授权与正式回归文件与当前字节一致。b16 quality（`.validation/runs/20261016-mvp-priority-closeout/gates/20261016T045451Z-9e0b9c81b729/report.json`） 包含 check-docs，稳定通过。

自身要求的相关 Server、CLI 与 Snapshot 合同均有无 skip 证据；宽报告的其它 live skip 不作为本条阻碍。固定成员的实际文件读取已核对，消费实现的补 Ref 仍由 `FILE-ACCESS-01` 交付，不重复认领。真实部署结果按本条约定单独报告，不用这些分源证据宣称最新候选全量或部署验收通过。未提交、未安装依赖；目录由统一收口处理。
