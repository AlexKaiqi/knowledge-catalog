# ARCH-HOME-REPOSITORIES-01 · 仓供给与连接管理从 Home 归位

## 目标

`home/repositories/` 统一拥有托管供给、自有连接、凭据轮换、所属私有账本及恢复流程。
根 Home 只连接该服务与来源登记、当前授权、Reader/Writer、插件和生命周期，
不再同时实现仓管理业务与整个资源图。

## 依据

- [服务](../../doc/design/service.md)、[权限](../../doc/design/permissions.md)、[Snapshot](../../doc/design/snapshot.md)。
- [架构](../../doc/architecture.md)、[架构不变量](../../doc/engineering/architecture-invariants.md)。
- 实施时核对 `home/README.md`、`snapshot/README.md` 与管理入口的公开代码和合同。

## 范围

归位 `home/managed*.go`、`home/connections*.go` 的管理、账本与恢复职责；
关联的初始授权应用、来源安装和私有凭据读取按其现有 owner 接口注入。
从 `invocation/session_helpers.go` 移出供给专用辅助，但基础授权规则仍由 `snapshot/access` 拥有。
仓管理子包不接收或构造整个 Home；具体来源绑定仍符合 provider 装配边界。
根 Home 保留插件装配、请求视图与统一启停，不把它们藏入仓管理服务。

## 验收

搬完后以下既有行为断言不变，且回归可证伪：

- 托管创建的首次、重试、同名冲突及中断恢复保留同一分配、初始版本和结果；
  重试与重启不重复创建，也不补回已撤销的初始权限。
- 自有连接仍只验证已有来源，不初始化或写入来源；连接准入、owner 管理和当前仓级授权不变。
  冷恢复不联系来源，凭据轮换只在同一绑定验证成功后替换，失败保留旧绑定和凭据。
- 耐久状态的缺失、损坏和既有历史格式处理沿原合同，不新建空账掩盖丢失；
  公开结果、日志、trace 与对象内容不含私有凭据。
- 构造失败关闭已取得资源，根 Close 释放所属资源一次；借用请求视图不能重装配或关闭根资源。
  停止维护、排空请求、释放查询资源的顺序保持，取消继续传到来源打开与实际操作。
- 架构守卫证明仓管理不反向依赖根 Home、CLI、HTTP 或 Invocation；
  具体 provider 的构造位置显式登记，不能通过新子包绕过既有守卫。
- 管理服务的单元测试随新 owner 同批迁移；真实 HTTP/CLI 管理和部署恢复集成保留在原入口。
  受影响并发/取消 race 回归通过，并按 `tasks/README.md` 的「目录重组的共同约定」取门禁范围。

## 前置

- `READ-BASIS-01` 的“固定读取依据交付”与 `INDEX-PHYSICAL-01` 的
  “索引接入与接口稳定交付”：所需来源绑定、发布准备、持久资源恢复及装配接口已有实现和回归。
  交付点不要求两项行为任务整体归档，但必须记录对应源码候选与证据。
- `ARCH-HOME-CONFIG-01` 与 `ARCH-CLIENT-SESSION-01` 已完成（均已归档）。
- 触及来源恢复、取消或生命周期时，复用 `DEBT-MAINTENANCE-CANCEL-01` 的现有交付；
  与在途维护/生命周期任务重叠的批次须先固定接口和回归候选，不等待不相关功能全部结束。

Object 和 Index 在现有装配边界完成上述交付，不反向等待本次仓管理拆分。

## 不做

不改仓管理协议、发权策略、状态格式或恢复决定，不重做 Snapshot provider，
不实现新的存储后端，不把仓管理变成独立进程，不承担 Dataset 自动跟随与回收。
