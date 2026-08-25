# local/

本机 Profile：⓪ `FileGitRepository` 提供 Snapshot/RawFileStore/Knowledge，③ SQLite 提供可重建的 text/filter/sort 投影。Catalog Registry 仍是独立的 ① git，位于 `layout.catalogs/<encoded-id>`。

SQLite adapter 同时实现 `Retriever` 与 `ProjectionMaintainer`：候选只含 identity、basis 与 evidence；公开结果必须回读同一 commit 的 Canonical。动态 state/stream 不在本包落盘，由 Aspect Binding 指向上层 Materialization runtime。
