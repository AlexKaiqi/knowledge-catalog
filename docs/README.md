# 文档地图

本目录解释设计问题、第一性原理、调研依据与已作决策。具体协议以 Go 类型、包 README 和 Conformance 测试为准；命令操作以根 `README.md` 与 `WALKTHROUGH_v5.1.md` 为准。

这条分工是刻意的：设计理由通常比字段和命令稳定，不应让文档复制代码后逐渐漂移。

## 1. 设计文档

| 文档 | 回答的问题 |
|---|---|
| `KNOWLEDGE_CATALOG_DESIGN.md` | 为什么需要 Catalog；身份、来源、写边界与维护闭环怎样推出 |
| `LAYERS.md` | ⓪–③ 各自知道什么；哪些依赖方向必须禁止 |
| `TERMINOLOGY.md` | 公开文档、CLI、JSON 与 Go 导出注释统一使用哪些名词 |
| `COMPOSITION.md` | 多 Repository 为什么由 Workspace 组合，而不是复制或覆盖 |
| `SERVICE_ARCHITECTURE.md` | Catalog/Knowledge 服务、统一客户端、远程 VFS 与接入写面怎样保持协议分层 |
| `ASPECT_ACCESS.md` | Aspect 写单元、读形态与检索声明怎样分离 |
| `LIVE_MATERIALIZATION.md` | Aspect State/Stream Binding 怎样由墙外产品物化并进入统一检索 |
| `CONNECTORS.md` | 外部权威怎样被访问，以及怎样显式进入知识仓 |
| `PERMISSIONS.md` | 能力授权、仓边界和权限知识为什么必须分层 |
| `STORE_ADAPTERS.md` | 权威、索引、缓存、投影的介质职责怎样划分 |
| `HOOKS.md` | 为什么需要薄的出站扩展点，以及它不能做什么 |
| `GATES.md` | 治理跃迁为什么必须检查绑定精确候选的证据 |

设计文档可以写概念模型和非约束性示意，但不维护以下内容：

- Go 字段、完整 JSON/YAML schema、错误码全集；
- CLI 参数表、本机配置文件格式、环境变量清单；
- “已做/未做”开发台账和按时间追加的历史记录；
- 可以由测试直接证明的实现状态。

重大选择写成 ADR/K 决策；被替代的结论直接修改正文，并由 git 历史保留演变。

## 2. 操作与验证文档

| 文档 | 职责 |
|---|---|
| 根 `README.md` | 当前能力、目录入口、运行方法 |
| `WALKTHROUGH_v5.1.md` | 用当前 CLI 走通操作流程 |
| `MVP_ACCEPTANCE.md` | 验收范围和证据 |
| `TEST_CATALOG.md` | 测试覆盖与待补测试；允许记录实现状态 |
| 各包 `README.md` | 该包的具体契约、用法和实现边界 |

知识提供方的实体/Aspect/关系草稿、源系统字段、Connector 和业务验收不回写成通用设计。数仓材料暂放 gitignored 的 `.data/data-warehouse/`，稳定后迁为独立 integration repo。

## 3. 维护规则

修改协议时按以下顺序维护：

1. 若问题、原则或决策改变，更新相应设计文档。
2. 让代码类型与 Conformance 表达具体协议。
3. 当前操作方式改变时，更新包 README 或 Walkthrough。
4. 测试状态改变时，只更新测试目录或验收文档。

同一事实只选一个权威位置，其余文档链接过去，不复制。
