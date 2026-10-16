# DEBT-LIVE-RERANK-RETIRED-GRANT-01 · 付费 live rerank 夹具仍申请退休搜权

## 债

- 位置：`cli/rerank_http_test.go` 的 `TestLiveHTTPSearchRerankWithLuna`，当前第 335 行。
- 信号：测试已有 Dataset `read-workspace` 消费授权，随后仍申请 `object.search,object.rerank`；当前公开授权合同已退休 `object.search`，显式启用后会在调用真实排序模型前被拒绝。
- 判定：付费 live 测试授权夹具落后于 Dataset 检索迁移，不是需要恢复的公开动作；普通 HTTP rerank 同类夹具已迁移，不能据此宣称付费 live 验收通过。
- 处置：由在途 `INDEX-SEARCH-SPACE-01` 同批迁移，仅保留 Dataset 的 `file.read` 消费与独立 `object.rerank` 授权，保留原排序和证据断言；不为登记本债修改 live 源码或执行付费试跑。
- 触发条件：`INDEX-SEARCH-SPACE-01` 的 CLI 迁移及下一次已明确授权的 live rerank 验收；主任务已在 `doing/`，登记后立即并入。

## 目标

付费 HTTP rerank 旅程使用当前合法的 Dataset 消费授权，仍验证真实模型排序和检索证据。

## 依据

- [发现](../../doc/design/discovery.md)、[权限](../../doc/design/permissions.md)。
- [工程质量闭环](../../doc/engineering/quality.md)、[测试与验证](../../doc/engineering/testing.md)。

## 验收

- 授权夹具不再申请退休动作，不扩大来源仓读取范围；保留排序、候选数、提供方与模型等全部原断言。
- 下一次已授权的 live rerank 验收执行该真实旅程，不能将未执行或 skip 算通过。
- 受影响合同、`make check-docs`、`make quality` 通过。

## 前置

与在途 `INDEX-SEARCH-SPACE-01` 的 CLI 授权迁移同批处理。

## 不做

不恢复退休权限、不扩写 help、不新增陌生 Agent 试跑、不在本轮启动付费验证。

## 关闭

本债未实现、未验收；按同批文件归并规则立即并入 `INDEX-SEARCH-SPACE-01`，由该任务承接夹具迁移与剩余验收。本条关闭不表示付费 live 测试已通过。
