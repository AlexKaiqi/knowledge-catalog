# DEBT-LIVE-MAINTENANCE-DIGEST-01 · 外部取值声明的维护摘要阻止旧条件覆盖

## 债

- 位置：`object/units.go` 的 PUT/REMOVE 条件比较、`object/reader/` 的维护 Resolve 摘要。
- 信号：真实部署以旧维护摘要把成员由描述 A 改为 B 后，再用同一旧摘要更新仍成功；两次声明正文都为 null，当前条件只比较内容摘要。
- 判定：已发布的取值来源变更没有进入维护依据，不能阻止覆盖刚刚更新过的声明；与 accept-02 的并发维护验收不符。
- 处置：保持 Unit、内容 READ、引用与索引内容摘要的原语义；由相同维护依据覆盖外部声明本身，并共同用于 Resolve 与条件 PUT/REMOVE，含复合对象删除。不纳入外部实时值、观察时间或远端最新描述。
- 触发条件：`ACCEPT-HARNESS-01` 的 `accept-live-maintenance-read-declaration` 实际部署红。

## 目标

外部声明变更后旧维护摘要拒绝继续覆盖，静态内容与观察摘要保持既有合同。

## 依据

- [对象读取与维护](../../doc/design/object.md#读取)。
- [LiveAccess 声明](../../doc/design/liveaccess.md#声明)。

## 验收

- 本单元声明 A→B 后旧条件 PUT/REMOVE 均拒绝，HEAD 与正文不变；复合对象条件删除同样不能漏过子声明变化。
- 静态维护摘要与内容摘要兼容，排序稳定，维护过程不触外部调用，外部值与观察变化不改变维护依据。
- 受影响合同、冻结文档/质量门禁和原实际部署 probe 通过。

## 关闭

2026-10-16 登记后按 `tasks/README.md` 的环境归并规则立即并入正在进行的 `ACCEPT-HARNESS-01`。本条关闭表示归并，不表示验收通过；专项红绿及原部署红保留在 `.validation/runs/`，最终验收由主任务记录。
