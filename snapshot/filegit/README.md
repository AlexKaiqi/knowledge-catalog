# snapshot/filegit/

本机权威 adapter：`FileGitRepository` 实现⓪ `snapshot.Store` / `TreeStore`，并提供可选② `knowledge.Repository` capability。Catalog Registry 仍是独立的① Git，位于 `layout.catalogs/<encoded-id>`。

SQLite 已独立到 `retrieval/sqlite/`，本包不依赖 Reader、Index 或任何物理检索 provider。动态 state/stream 不在本包落盘，由 Aspect Binding 指向上层 Materialization runtime。
