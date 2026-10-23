# HISTORY-01 · 有界连续历史

## 目标

对象历史可按 first-parent 链连续查询且不伪造引入点。存储提交分页、扫描预算和返回修订条数是三个
独立限制，不互相代替。只返回已证明引入点的修订；预算耗尽时保留未完成状态段与继续依据，本页可以
没有修订。成功续页必须推进扫描位置，即使仍无修订；无法取得进展时明确报错，不能无限返回相同空页。
只有证明历史终点才结束，读取失败不能伪装预算耗尽。

续页能独立恢复原对象、范围、来源绑定、固定坐标与扫描进度，不重新解析最新版本。每页重新检查当前权限。

## 依据

设计篇：[历史与连续扫描](../../doc/design/object-versioning.md#历史与连续扫描)、
[历史权限](../../doc/design/object-versioning.md#历史权限)。

## 已交付

### Reader 与耐久进度

Object Reader 新增可选 `HistoryScanner`，将检查提交预算、返回修订上限与时间预算分开。未证明引入点的
状态段随来源绑定、原始 commit、当前扫描位置和完整本地状态保存；多页无修订仍推进位置，只有真实
first-parent 起点才终结。`A→B→A`、声明、Kind、删除与真实引入点沿同一完整 Unit 状态判据。
旧 `Log` 复用这一折叠算法，不保留第二份状态折叠实现；物理窗口检查也复用一份校验。

`HistoryCheckpointStore` 持久保存服务器生成的完整选择依据与未完成源集合，与 `ReadBasis` 共用一份事务、
目录同步及只读查询机制，使用独立 bucket、版本与前缀。新进度追加不可变记录，旧页可解析；损坏拒绝且
不静默修复，缺失、取消或非法请求不创建存储。根 Home 装配该端口，借用视图共享记录，关闭请求不删除记录。

### 公开入口

`log:query`、SDK Log 与 `kc log` 已接入有界 Reader 与耐久检查点：全来源共享提交扫描预算，修订上限
分别作用于各来源；返回已证明引入点的 logs、已知 scanned 与 complete。未完成页必有已推进的
continuation，允许连续空页。

续页在授权与范围解析之前恢复原对象、Repo/Dataset 选择、ref、固定来源及扫描位置；可只提供
continuation，也可附带匹配坐标，不继承客户端默认 Dataset。每页复核全部候选（含已完成来源）的
当前历史权、源绑定、commit 与已接受发表范围。Dataset 新版及源 HEAD 推进不改原依据；撤权或退役明确拒绝。

整仓选择与额外交付别名并存时仍允许来源历史，纯文件或子目录切片继续拒绝。解释依赖不参与本地历史，
不因其失效而读最新解释。

请求期限从应用操作开始绑定；`Dataset` 新增可选 `ReadContextBound` 的只读上下文视图，来源与登记权威
I/O 均受约束。实际取消回归发现归档探测取消后缓存的 commit 存在性可使解析误报成功，已以共用取消守卫
修复解析、发布重放、可用性、登记历史和虚拟读取。

## 验收

| 场景 | 判定 |
|---|---|
| 超过 1000 单页、10000 原窗口及扫描预算耗尽 | 连续遍历不漏项、不重复；窗口边界不伪造引入点及作者时间，未知边界可识别并可继续 |
| 状态跨页持续相同；连续多个空结果页 | 空页游标携带未完成状态段且扫描位置单调推进，只有证明终点才结束 |
| `A → B → A`、删除后原样重建、合并提交 | 保留 first-parent 上实际变化；端点 diff 为空不跳过真实修订 |
| Dataset 根目录整仓交付、文件切片 | 仅有消费权均不能查询来源历史；独立来源仓授权适用于首页与每次续页 |
| 来源历史授权授予后再撤销 | 续页重新校验并拒绝，不返回部分成功 |
| 读取失败、取消、保存失败 | 不返回部分成功；读取失败不伪装预算耗尽 |

HTTP 与 CLI 分别执行"首页 → head 推进 → 仅游标续页"。覆盖 basis 与其它坐标混用、跨重启续页、
完整依赖缺失或无权时不回退最新。

## 剩余

1. 同一冻结候选上的完整混合契约。`.validation/runs/20261020T201018Z-13a57183a7fb`
   （2407 通过、2 skip）与 `.validation/runs/20261020T201018Z-b768c052f615`（219 通过、无 skip）
   是批次证据，不是冻结候选验收。
2. 真实部署验收。
3. 与索引物理回收的联合场景：查询结构回收后，读取依据和观察证据仍按各自合同可用。

## 前置

无业务前置。与 `READ-BASIS-01` 共用耐久事务与目录同步机制，但使用独立 bucket、版本与前缀；
存储机制先行，判定互不等待。

## 不做

不做区间跳过。`ChangedPaths` 的端点树差异不能证明区间未变化；任何加速须有覆盖整段且不漏变化的证据。
不做历史加速与 head 缓存。不新增批量、并发或缓存形态。

## 交付

实现与公开入口已完成，见本节；整体验收未完成，任务保持 `doing/`。

- 有界 Reader 与耐久进度基础：`.validation/runs/20261020T191848Z-f5d813a55ef6`（race 定向 29 项通过）、
  `.validation/runs/20261020T192311Z-21bbc45839a6`（1914 通过、1 skip，真实 Writer 吞吐环境项）。
- 公开有界连续历史：`.validation/runs/20261020T201018Z-13a57183a7fb`（2407 通过、2 skip）、
  `.validation/runs/20261020T201018Z-b768c052f615`（219 通过、无 skip）、
  `.validation/runs/20261020T201018Z-6834a17e66e7`（默认场景 18 通过）。三次 `sourceChanged=false`。
- 真实 CLI/HTTP 夹具通过公开读取验证 780 个无关真实提交上的连续空页、精确引入点、进度推进、
  原 cursor 可复用、服务冷重开、来源推进、默认 Dataset 隔离与撤权。用例构造无关历史通过
  Snapshot RawWriter，不是空 commit。
- 冻结候选 `.validation/runs/20261022T154324Z-93b18dd4deb1`（默认场景 18 通过、无 skip）、
  `.validation/runs/20261022T154436Z-5d9104e40a73`（5682 通过、0 失败、18 条既有环境 skip）、
  `.validation/runs/20261022T155925Z-48b3b9703db5`（真实部署专项 2 通过、无 skip），源码指纹
  `c967676f46a0a8c53b3dd3ceff862af8c7628a36f8679ce800899db629c70a0c`，三次 `sourceChanged=false`。
- 「历史首页与续页」由 `object-published` 节点 `probe-history-continuation` 在候选上执行：首页 `limit 1`
  返回 `complete=false` 与游标；仅凭游标续页可得更早修订；游标与显式 commit 混用报 `USAGE_INVALID`；
  新请求能看到最新修订。
- 与索引物理回收的联合落点：`TestHistoryBasisPreservesWholeSourceWithoutOpeningInterpretation` 确认整段
  历史只走来源范围、不因解释依赖额外取数；`TestHistoryCheckpointSurvivesReopenWithIndependentReadMetadata`
  确认检查点跨重开保留且读元数据独立；`TestBoundedHistoryServicePersistsEmptyPagesAcrossReopenAndSourceAdvance`
  确认空页进度与服务重开。索引侧的物理回收判定归 `INDEX-PHYSICAL-01`。
