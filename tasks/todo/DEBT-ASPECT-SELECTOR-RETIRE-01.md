# DEBT-ASPECT-SELECTOR-RETIRE-01 · 先整读再裁剪的方面选择器退场

## 债

- 位置：`object/read.go` 的 `AspectSelector` / `SelectAspects`；`object/reader/reader.go` 的 `Reader.Read`、
  `object/reader/serving.go` 的 `Serving.Read`；`liveaccess/service/hydrate.go` 的 `Service.Read`、`Hydrate`、
  `HydrateRepositoryRead` 与 `selected`；`liveaccess/service/repository_read.go` 的 `Selector` 字段；
  `discovery/service/dataset_search.go` 的 `Hydrate` 端口。
- 信号：这些入口先组装整个对象，再按方面名删掉不要的部分。按 GraphQL 形状读取（OBJECT-SHAPE-READ-01）
  已经改成先按坐标规划、只读所选单元，读取通路不再需要整读后裁剪。2026-10-16 核对：生产代码里没有调用方传入
  非空选择器，`--include` / `--exclude` 在引用读取入口已经拒绝，唯一的 flag 转换函数 `AspectSelectorFrom`
  已删除；剩余的非空用法只出现在测试里。
- 判定：选择器参数只剩形式上的存在，但签名同时被 `snapshot/dataset`、`server`、`home`、`discovery`、
  `liveaccess` 等几十个调用方使用，其中一部分文件正被并行任务修改。按“改动重叠后开的先停”的规则，
  本轮不改签名。
- 处置：删除 `AspectSelector`、`SelectAspects` 和上述签名中的 selector 参数；测试里改用形状读取或
  按 Pointer 的引用读取表达原有断言（只取部分方面、未选方面不外部取值），不削弱断言。
- 触发条件：相关调用方文件上的并行改动落定后，或下一次修改 `Reader.Read` / `Service.Read` 签名时。

## 目标

对象读取只剩两种通路：整对象读取，以及按引用或形状先规划再只读所选单元；不再存在先整读再裁剪的选择器。

## 依据

- [对象](../../doc/design/object.md)：读取、形状（只读需要的部分）、进度表中“先整读再裁剪的旧方面选择器退场”。
- [工程质量闭环](../../doc/engineering/quality.md)：增量不欠与质量问题三个出口。

## 验收

- `rg 'AspectSelector|SelectAspects'` 在生产代码与测试中均无结果。
- 原选择器测试表达的风险（只返回部分方面、未选 Binding 不外部取值、证据只覆盖所选方面）改由形状或引用读取回归承接并通过。
- 受影响完整回归、`make check-docs`、`make quality`、`make test`、`make test-contracts` 全绿且无 skip。

## 前置

- OBJECT-SHAPE-READ-01 已交付。
- 涉及文件上的并行改动已落定。

## 不做

不改变整对象读取与引用读取的返回形状，不新增读取参数。
