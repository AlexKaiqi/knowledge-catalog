# internal/gitdir/

不带领域语义的 Git 目录 plumbing，当前供 ① Catalog Registry 的登记表历史与 CAS 使用。`tree` 读 blob/tree，`worktree` 管 checkout 与 porcelain，`commit` 管 CAS 提交，`refs` 管 ref 形状，`log` 管历史，`signature` 管 kc trailers。

`BuildFlatCommit` 只写不可变 Git objects，不碰 ref、index 或工作区；`CompareAndSwap` 使用 `update-ref` 的 expected-old，`PushCAS` 使用远端 `--force-with-lease`。拒绝提交时 ref 不动。`OpenCache` 不创建 root commit；`FetchRef` 只获取 authority objects，`RemoteRef` 区分不存在与传输失败。`Materialize` 只更新可丢工作区和 index，不移动 ref。远端发布失败但 authority 已确认 candidate 时按已接受处理，避免丢失确认造成重复发布。
