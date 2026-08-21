# scale/

规模化方案集。Snapshot / Stream / ③ 引擎分开。CLI `profile: scale`。

| 层 | 实现 | 状态 |
|---|---|---|
| ⓪ Snapshot | Dolt | `DoltRepository` 实现 `SnapshotStore`；当前是 git 形知识文件（与 FileGit 同口）。原生 Dolt SQL 未装配 |
| ⓪ Append | 有序段 | `OpenStream` 返回 `repository.Stream`（stub Append）；**不是**仓 |
| ③ 全文 | Elasticsearch MATCH | `OpenElasticsearch` 已有 |
| ③ 列索引 | StarRocks | `OpenStarRocks` stub |
| ③ 热尾 | Redis | 只做 cache，不是仓 |

不要 `repo-add --driver stream`。不要 `--driver mysql`。协议根不长 Hippo SR connector。不要把 Redis 当仓或比较引擎。
