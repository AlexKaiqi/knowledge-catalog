# ARCH-INVOKE-01 · 共用调用词汇按消费者归位

## 目标

`invoke/` 中的调用参数、执行阶段、HTTP 编解码和观测职责分别有明确 owner。
只有仍被多个入口需要且不引入上层依赖的纯词汇可以保留为窄公共包；
包是否保留由实际调用决定，不以消除相似名称作为完成标准。

## 依据

- [服务](../../doc/design/service.md)、[Affordance](../../doc/design/affordance.md)、[架构](../../doc/architecture.md)。
- [架构不变量](../../doc/engineering/architecture-invariants.md)、[工程质量闭环](../../doc/engineering/quality.md)。
- 实施时核对 `invoke/README.md`、`invocation/README.md`、`internal/httpjson/README.md` 及公开输出合同。

## 范围

以实际生产消费者梳理 `invoke` 的七类现有文件及导出符号。
执行阶段归执行器；通用 HTTP 编解码与现有 `internal/httpjson` 统一；
调用观测按应用适配和底层遥测分开；参数读取与规范输出按共享需求确定归属。
HTTP 基础设施不能为接收现有结果类型而反向依赖 Invocation，底层遥测也不取得授权或证据决策权。
既有公开 Go 入口按总任务兼容约定处理，不保留私有壳或另造宽泛工具包。

## 验收

- 每项原职责有唯一去向和生产消费者，剩余公共包的允许依赖可明确列出；
  无仅为改名新增的平行实现或全量原样转发。
- HTTP 解码、输入大小与压缩限制、无损 JSON 数字、错误信封和状态映射保留原合同；
  CLI 输出与服务端编码的已有边界回归通过。
- 执行阶段和 Hook 作用范围不因位置变化改变；日志、trace 与证据观察仍关联同一请求，
  不重复计数，不将凭据或敏感正文写入遥测。
- 通用 HTTP/JSON 基础设施、参数词汇和底层遥测各自依赖方向受守卫保护；
  CLI 与 Server 仍使用同一应用执行器，而不是相互调用运输适配。
- 对删除、保留或必要公开兼容桥逐项说明；模块 SDK 与 CLI 不因共享辅助导入 Home 或执行器实现。
- 执行 `ARCH-LAYOUT-01` 的共同实施与验收约定；同步包说明、依赖表和相关测试登记。

## 前置

本项后置于 `ARCH-INVOCATION-01`，承接其 Object/Index 交付点与全部目录前置；
先稳定实际执行职责和消费者，避免对尚待归位的调用链整体改名。
不以共用词汇拆分阻塞 Object 或 Index 的实现，也不提前为它们预造第二套调用类型。
路由清单继续复用 `ROUTE-MANIFEST-01` 的现有交付。

## 不做

不强制移除 `invoke`，不重写命令框架或路由清单，不改输出、状态码或错误码集合，
不把 KC 会话、授权或领域 DTO 塞入 HTTP 基础设施。
