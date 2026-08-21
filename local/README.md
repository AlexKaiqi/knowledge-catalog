# local/

本机方案集：**⓪ FileGit（Snapshot = git）** + **独立 JSONLStream（Stream，不是 git）**，**③ SQLite**（FTS + filter 列）。不要 Redis。

对外：`repository.SnapshotStore` / `Knowledge`（FileGit）与 `repository.Stream`（`JSONLStream`）以及 `index.Engine`。JSONL 放在 `.git` 旁是落盘方便，不是「APPEND 是 git 的能力」。登记表 git 是 ①，在 `layout.catalogs/<encoded-id>`。
