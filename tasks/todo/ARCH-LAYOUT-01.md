# ARCH-LAYOUT-01 · 能力目录与宿主边界重组联合验收

## 目标

在同一个候选上证明三件事：会话归调用方、`home/` 根包只拥有组合与生命周期、
`invocation/` 只拥有统一执行与必要的跨模块协调。模块实现与测试归所属能力，根 CLI 保留真实入口、
宿主装配与跨模块集成。交付职责与依赖图、迁移映射和测试覆盖映射。

本任务只做联合验收，不自己实施迁移。实现分别由下表子任务交付；错误码审计已改为探索
`QUESTION-ERROR-CODES-01`，不在本任务范围。

迁移完成后测试归属的残留核对也在本任务内：测试位置反映其真实验证边界——模块算法与模块入口合同归所属包，
根 CLI 保留 argv、身份运输、部署宿主、跨模块旅程和实际 Server 集成；测试覆盖、验证入口登记与架构守卫
仍能准确识别同一行为。这项核对与前六项验收共用同一份候选和同一份报告，不另立一条任务。

## 依据

- [架构](../../doc/architecture.md)、[路线](../../doc/roadmap.md) 的目录重组表。
- [服务](../../doc/design/service.md)、[权限](../../doc/design/permissions.md)、[插件](../../doc/design/plugins.md)。
- [架构不变量](../../doc/engineering/architecture-invariants.md)、[工程质量闭环](../../doc/engineering/quality.md)、
  [测试与验证](../../doc/engineering/testing.md)。

## 目标布局

| 目录或职责 | 目标 | 实施任务 |
|---|---|---|
| `kernel/`、`snapshot/`、`object/`、`discovery/`、`declarativeindex/`、`liveaccess/`、`systemrepo/` | 保留能力主干及各自服务、SDK、CLI；迁移只归还所属职责 | 各迁移任务 |
| `client/session/` | 服务地址、登录、会话持久化、刷新与注销；CLI 与挂载客户端复用 | `ARCH-CLIENT-SESSION-01` |
| `home/config/` | 部署配置、解析与静态校验 | `ARCH-HOME-CONFIG-01` |
| `home/repositories/` | 托管供给、自有连接、凭据轮换、私有账本与恢复 | `ARCH-HOME-REPOSITORIES-01` |
| `home/` 根包 | 装配依赖、恢复资源、请求借用视图、统一启停 | 上述 Home 两项及 `ARCH-DATASET-PIN-01` |
| `internal/datasetbridge/`（拟） | 已解析 Dataset 到对象读取依据的唯一纯转换；不持有运行资源 | `ARCH-DATASET-PIN-01` |
| `invocation/` 与 `invoke/` 的现有职责 | 执行器与授权协调归 `invocation/`；调用参数、执行阶段、HTTP 编解码和遥测按消费者归还；是否保留窄词汇包由实际依赖决定 | `ARCH-INVOCATION-01` |
| `cli/` 及各能力测试 | 单模块断言随 owner；根保留真实 argv、身份上下文、宿主与跨模块集成；迁移后的残留归属与登记收尾在本任务内一次核对 | 各迁移任务同批完成，残留收尾归本任务 |

`identity/`、`delivery/`、`observability/`、`hook/`、`server/`、`datasetfs/`、`cmd/` 保持现有职责。
场景与材料仍归 `.data/scenes/`，压测仍归 `.data/scale/`；不为目录整齐改动其组织合同。
目录树不是分层依据：每个新增 Go 包都须显式归层、列明允许依赖，并保留正反架构守卫。

## 验收

- 七项目录子任务各自达到验收；交付实现与原位置的映射，旧 owner 不再持有重复实现。
- 登录与会话的依赖闭包不进入 `invocation`、`home`、`server` 或具体存储/索引实现；业务 CLI 仍经
  Server 与显式主体。部署与 serve 命令的合法装配依赖单独验证。
- Home 的配置和仓管理子包不反向依赖根 Home；请求视图不能启停或关闭根资源。
  `invocation` 不重新实现所属能力的规则，不打开部署或构造具体 provider。
- 三份 Dataset 读取依据转换归一，保留所有既有来源、范围与解释依赖；测试夹具使用同一转换但独立断言预期，
  不从被测实现生成 oracle。
- 测试迁移有前后覆盖对应，原断言、CLI/HTTP 共用执行器、固定版本、当前授权与失败语义仍可证伪；
  不以挪动测试、改名或降低门槛制造通过。
- 测试归属收尾：测试改名、拆分或合并后原授权、版本、失败及恢复断言完整保留，不以测试数量减少作为收益；
  模块内部算法不靠虚假的 CLI/HTTP 调用伪装成集成测试，真实 argv、身份注入、宿主装配与跨模块旅程仍在
  真实边界执行；根 CLI 生产/测试边界、公开入口保护与测试 owner 守卫保留，守卫路径随真实 owner 更新并用
  正反例证明不会因迁移漏扫；测试辅助复用单一实现但不复用被测逻辑生成预期，不为迁出测试增加生产后门或
  公开工具函数；测试目录、架构不变量、场景关联、race 清单和验证清单中的受影响定位均可解析到实际入口，
  `make check-validation` 无遗漏或重复认领。
- 在最终同一源码候选上执行 `make check-docs`、`make quality`、`make test`、`make test-contracts`，
  保存范围、源码指纹、失败/跳过计数及证据路径。子任务的历史绿不能代替此联合验收。
  真实部署验收由 `FINAL-ACCEPTANCE-01` 及相应行为任务独立完成，不把夹具结果报告为部署通过。
- 交付顶层与具名子包目录、实际依赖图、每个目录一句职责、必要公开兼容桥及耦合变化，
  与 `doc/architecture.md`、`doc/design/service.md` 和 `doc/roadmap.md` 逐条核对。这份报告进入
  `FINAL-ACCEPTANCE-01`。

## 前置

五项子任务全部达到各自验收：`ARCH-CLIENT-SESSION-01`、`ARCH-HOME-CONFIG-01`、
`ARCH-HOME-REPOSITORIES-01`、`ARCH-DATASET-PIN-01`、`ARCH-INVOCATION-01`。

`ARCH-INVOCATION-01` 含原 `ARCH-INVOKE-01` 的调用词汇归位，其完成即调用链边界稳定。
测试归属的残留核对不作独立前置：它与本任务的其他验收在同一候选上一起做，触及
`ACCEPT-HARNESS-01` 仍在修改的 probe 或登记文件时，先固定其该批交付再更新定位，不改规格原预期。

子任务开工按各自的 `## 前置`，不要求本任务先行。

## 不做

不重建全仓分层目录，不改公开行为，不重做路由清单，不改 Dataset 版本与生命周期、索引物理布局或对象版本语义。
不按包体积机械拆包，不为压低测试比例迁走真实入口测试，不预定 `invoke/` 必须删除或只改名。
不把无生产者错误码清理捆入目录迁移。不机械迁走所有 `cli/*_test.go`，不追求测试/生产行数比例，
不删除、跳过或弱化断言，不把未完成的实现验收记成测试整理完成。

## 沿革

`ROUTE-MANIFEST-01`、`LIVEACCESS-READ-01` 已归档交付，本轮复用。
2026-10-16 原并入的路由清单与 UI 跳转状态码债拆为 `ROUTE-MANIFEST-01`。
2026-10-20 按当前目录重划范围并补入 Object/Index 交付点前置，只更新规划，未开始代码迁移。
`object/reader` 下沉候选经层表核对不适用：它属于 object 层，不能依赖 dataset 层，
转换改用窄的上层私有桥接。这些变更的实现与验收分别记在各自任务，本文件不再展开。

2026-10-23 两项并入：

- `ARCH-INVOKE-01` 已并入 `ARCH-INVOCATION-01`（理由见该文件“沿革”），本表两行合并为一行。
- `ARCH-TEST-OWNERSHIP-01` 并入本任务。理由：本任务自己的验收里就有“交付测试覆盖映射”，而该任务的
  全部内容是“迁移后的残留归属与登记收尾”——本任务要的证据正是它的产出。它自己的“前置”又写着要等
  各项迁移与对应测试完成，也就是本任务的前置已经覆盖。三项分开时，同一份职责与覆盖报告要产三遍。
  并入后测试归属的验收条款保留在本任务，不再单独归档。

本任务的报告进入 `FINAL-ACCEPTANCE-01`；`FINAL-ACCEPTANCE-01` 的目录与错误码核对一节引用本任务产出，
两者不再各自要求“在同一候选上证明一批事”。
