# DEBT-DATASET-VERSION-CONTRACT-01 · Dataset 新版本合同缺实现与验证入口

## 债

- 位置：`doc/design/dataset.md` 的 U16、U17，`.data/scenes/_views.yaml` 的 Dataset 产品视图。
- 信号：2026-10-20 完整契约的 `TestSceneViews/product_links` 报两条新设计用例既无实际验证关联，也无明确 gap。
- 判定：设计目标新增了服务端版本号分配、无变化定义不增号和发布版间指针对比；当前 CLI 仍要求显式 revision，不能把旧发布测试当作完整新合同证据。
- 处置：产品视图先登记精确 gap；实施前审查版本分配、重放与对比入口，再补具体规格、实现及场景关联，不削弱设计或守卫。
- 触发条件：下一次修改 Dataset 版本分配、发布幂等或历史对比入口时处理；与 Dataset 修订任务有文件重叠时按任务规则并入。

## 目标

Dataset 的版本分配、无变化发布与两版指针对比满足 U16、U17，并有对应的具体执行证据。

## 依据

- [Dataset](../../doc/design/dataset.md)：版本、元数据、U16、U17。
- [工程质量闭环](../../doc/engineering/quality.md)：质量问题三个出口。
- [场景合同](../../.data/scenes/README.md)：新增产品条目须有验证入口或明确缺口。

## 验收

- U16 的服务端分配、单调不可复用、同定义重放不增号及请求内固定版本各有可证伪回归。
- U17 比较两份已接受清单的增删改与元数据，不读来源字节；来源不可用仍返回指针差异及明确状态。
- 具体验证关联到产品条目，已证明部分再收窄 gap，不以解析或命令成功代替语义断言。
- 受影响回归、场景三项检查、`make check-docs`、`make quality`、`make test`、`make test-contracts` 全绿且无 skip。

## 前置

审查与既有 `DATASET-REVISION-01` 的范围及文件重叠，不另开同文件并行实施线。

## 不做

不改变自动跟随、检索保留、来源提交保留或紧急清除的任务归属。
