# DEBT-ACCEPT-SPEC-TRUNCATION-01 · 补全三条被截断的外部取值验收规格

## 债

- 位置：`.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/accept-03-live-read.yaml` 的 `accept-live-member-failure-isolated`、`accept-live-observation-beside-value`、`accept-live-precise-range`。
- 信号：首条 given 末项止于“测试源把 table/shop.orders”；第二条 then 首项止于“observations 恰有两项，地址分别是 table/shop.orders”；第三条原文在同一行仍有 `#/runtime/row-count；第三次测试源没有新调用。`，但未加引号的 ` #` 被 YAML 当成注释，加载结果丢失这一段。前两条原始行本身已经缺少后文。
- 判定：规格不完整，不能由执行器推测原文，再把推测当作产品 Oracle。2026-10-15 的逐规格库存与证据在 `.validation/runs/20261015T120400Z-accept-harness/`。
- 处置：由规格维护者从已有来源恢复前两条缺失动作与完整地址；第三条保留原文并用 YAML 引号保护，再接入相应部署 probe；保留原有失败隔离、值与观察分离、精确读取范围三个风险，不收窄断言。
- 触发条件：`ACCEPT-HARNESS-01` 接入这三条可执行 probe 前。

## 目标

三条规格的前态、动作和可证伪预期完整；执行器不会将规格损坏报告为产品缺能力。

## 依据

- [测试与验证](../../doc/engineering/testing.md)。
- [LiveAccess](../../doc/design/liveaccess.md)。
- 场景规格规范：`.data/scenes/README.md`。

## 验收

- 恢复三处原意，明确所有被比较的单元地址、调用次数与不调用范围。
- 三条部署 probe 对完整 then 逐项断言；场景树产品关联检查、`make check-docs`、`make quality` 全绿。
- 缺少动作或地址的同类截断信号有自动回归保护。

## 前置

规格原意的权威来源，或用户裁决。

## 不做

不根据当前产品输出反写期望，不删除原规格，不以占位 fail 冒充能力红。

## 交付

### 部分实现

2026-10-16 仅对已有完整原文的 `accept-live-precise-range` then 加 YAML 双引号，保留原句全部含义，使 ` #/runtime/row-count` 与第三次不调用源的断言不被当作注释吞掉。新增执行器回归直接读取实际规格，先红后绿，见 `spec-quote-red.log`、`spec-quote-green.log`。

前两处原始行截断尚未找到权威完整原文，保持原文，没有根据实现猜预期；需要规格原意的权威来源或用户裁决。因此任务仍 doing。场景 views/product/states 检查通过只证明结构关系，不证明截断已恢复。证据目录 `.validation/runs/20261016T012005Z-accept-harness`；源码写入完成后交由主线在稳定窗口统一运行全量门禁，本线没有运行完整架构组、`make quality` 或 `make test`，没有移 done。

## 关闭

2026-10-16 并入 `ACCEPT-HARNESS-01`：位置、信号与剩余验收在该任务的“并入”一节继续推进。本文件已有的交付与证据保留为历史，不代表验收通过。
