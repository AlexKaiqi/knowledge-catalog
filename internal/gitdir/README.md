# internal/gitdir/

不带领域语义的 Git 目录 plumbing，当前供 ① Catalog Registry 的登记表历史与 CAS 使用。`tree` 读 blob/tree，`worktree` 管 checkout 与 porcelain，`commit` 管 CAS 提交，`refs` 管 ref 形状，`log` 管历史，`signature` 管 kc trailers。
