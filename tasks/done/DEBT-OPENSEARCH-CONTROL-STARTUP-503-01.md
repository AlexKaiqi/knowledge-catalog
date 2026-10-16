# DEBT-OPENSEARCH-CONTROL-STARTUP-503-01 · 控制索引初次读取返回 503

## 债

- 位置：`declarativeindex/opensearch/internal/opensearchimpl/client.go` 的控制索引建立、`projection.go` 的控制文档读取，以及 Server 启动与显式 Dataset 准备的交界。
- 信号：冻结统一契约的 `TestConnectedPluginHandlerExplicitStartUsesOneFixedRuntime` 在 sync 时读取新控制文档返回 HTTP 503；后续同一测试连续 5 次和完整组件段通过。
- 判定：尚未定因的瞬时可用性问题，不能由事后通过倒推根因。
- 处置：并入正在修改同批检索准备与维护入口的 `INDEX-SEARCH-SPACE-01`，保留失败窗口采样及剩余验收。
- 触发条件：本任务统一门禁或相同控制索引 GET 503 再次出现。

## 目标

固定运行时激活后显式 Dataset 准备可靠；物理提供方短暂不可用有可归因证据，不以弱化生命周期断言隐藏。

## 依据

[发现](../../doc/design/discovery.md)、[测试与验证](../../doc/engineering/testing.md)。

## 验收

明确失效层后修复，原并发组件段和固定运行时激活的原断言通过；保留真实错误与物理节点诊断，不新增 skip 或无证据的业务重试。

## 前置

`INDEX-SEARCH-SPACE-01`。

## 不做

不清理他人资源，不降低测试并发，不放宽断言，不将一次隔离通过写成根因已解决。

## 关闭

未完成即归并：实现调查和剩余验收并入 `INDEX-SEARCH-SPACE-01`；本条不代表问题已修复。
