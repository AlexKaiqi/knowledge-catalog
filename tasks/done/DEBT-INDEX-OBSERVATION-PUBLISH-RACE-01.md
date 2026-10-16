# DEBT-INDEX-OBSERVATION-PUBLISH-RACE-01 · 当前修订校验与观察接受缺少原子条件边界

## 债

- 位置：`declarativeindex/projection/dataset_state.go` 的 `requireCurrentDataset` 与 `session.Commit`；Dataset 发布在独立权威服务中接受新修订。
- 信号：刷新入口、拿构建锁后和物理接受前均复查当前修订，已覆盖观察期间发布新修订的反例；最后一次校验返回后、`session.Commit` 完成前仍存在独立 Dataset 发布推进修订的窗口。
- 判定：已关闭可重复的观察期间竞态，但没有证明“只刷新当前修订”的完整并发合同；不能把连续校验当成原子条件接受。
- 处置：并入 `INDEX-SEARCH-SPACE-01`，建立观察接受与 Dataset 当前修订权威之间的条件边界，保留原观察代直至接受成功。
- 触发条件：`INDEX-SEARCH-SPACE-01` 的当前修订观察空间验收继续收口。

## 目标

观察接受必须以同一 Dataset 修订仍为当前修订为条件；并发发布新修订后，保留修订的候选观察不得替换原观察代。

## 依据

- [发现](../../doc/design/discovery.md)：Dataset 修订检索空间与索引维护。
- [Dataset](../../doc/design/dataset.md)：发布修订与当前修订。
- [LiveAccess](../../doc/design/liveaccess.md)：固定声明依据与外部观察。

## 验收

- 在最后一次当前修订校验之后、物理接受之前推进 Dataset 修订，旧刷新被拒绝，原观察修订与可查询值保持不变。
- 成功刷新仍能发布同一当前 Dataset 修订的新观察代。
- 受影响合同及工程门禁通过。

## 前置

`INDEX-SEARCH-SPACE-01`。

## 不做

不增加跨仓对象写事务，不把外部观察写回冻结文件。

## 关闭

2026-10-16 登记时主任务已在进行，按债的归并规则立即并入 `INDEX-SEARCH-SPACE-01`。本条关闭表示归并，不表示并发合同已验收。
