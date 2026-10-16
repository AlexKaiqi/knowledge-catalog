# DEBT-CLI-SURFACE-CONTRACT-01 · 同步 CLI 操作说明中的旧对象与检索合同

## 债

- 位置：`cli/SURFACE.md` 的上下文与操作数、Object、发布及运维段落。
- 信号：对象发布验收复核发现该文仍称 URN 指定来源仓、基本对象命令拒绝 `--repo`、Schema 浏览只支持仓、搜索支持仓范围，以及 Schema 携带访问声明和 origin；这些叙述与当前对象引用、Dataset 检索及结构 Schema 合同不一致。
- 判定：已有开发者操作说明漂移；本轮公开 help、系统出版物与对象发布合同已另行验证，不以扩大帮助改写替代三项债的验收。
- 处置：归入现有 CLI 检索迁移线，按设计和公开代码核对后同步旧段落，并保留公开例子和输入合同的自动检查。
- 触发条件：`INDEX-SEARCH-SPACE-01` 的 CLI 迁移及统一交付前；该任务已在 doing，登记后立即并入。

## 目标

CLI 操作说明与不带仓的对象身份、显式维护目标、Dataset 固定修订检索和结构 Schema 一致。

## 依据

- [对象](../../doc/design/object.md)、[发现](../../doc/design/discovery.md)、[Affordance](../../doc/design/affordance.md)。
- [工程质量闭环](../../doc/engineering/quality.md)。

## 验收

- 对照当前公开命令、类型和 Conformance 修正文中互斥规则，不恢复退休入口。
- 相关公开文档与命令合同、`make check-docs`、`make quality` 通过。

## 前置

与 `INDEX-SEARCH-SPACE-01` 的 CLI 迁移同批处理。

## 不做

不新增命令、参数、发布流程、help 篇幅或陌生 Agent 试跑。

## 关闭

本债未实现，按同批文件归并规则并入正在进行的 `INDEX-SEARCH-SPACE-01`；剩余说明同步和回归由该任务承接，不作为三项对象发布债本轮新增实施范围。
