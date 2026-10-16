# DEBT-INDEX-PLUGIN-CLI-DOCS-01 · 插件 CLI 包说明遗漏清单与试运行任务

## 债

- 位置：`discovery/cli/README.md` 的 plugin 任务说明。
- 信号：2026-10-16 阅读包合同时发现“task 仅接受 help”，但公开验证器与执行器已支持 `help`、`describe`、`trial`，能力清单包 README 也已指向后两者。
- 判定：包说明与公开 CLI 合同不一致；不涉及 Dataset 搜索与消费方式的文案裁决。
- 处置：并入 `INDEX-DECLARATION-01` 文档收尾；本轮 L2 代码文件范围不含该 README，未直接修改。
- 触发条件：`INDEX-DECLARATION-01` 说明入口收尾；该任务当前已在 `doing/`，立即归并。

## 目标

Discovery CLI 包说明准确区分插件说明、实例清单与只读试运行入口，和公开合同一致。

## 依据

- [Affordance](../../doc/design/affordance.md)：帮助与真实调用入口一致。
- [发现](../../doc/design/discovery.md)：实例清单与试运行。

## 验收

- 重读包说明与实际 plugin 命令、验证器、执行器，三种任务及试运行的只读边界一致。
- `make check-docs` 与 `make quality` 通过。

## 前置

并入 `INDEX-DECLARATION-01`。

## 不做

不提前修改 Dataset 搜索和消费方式的产品说明。

## 关闭

2026-10-16 并入 `INDEX-DECLARATION-01` 的文档收尾。此关闭仅表示归并，包说明尚未修正。
