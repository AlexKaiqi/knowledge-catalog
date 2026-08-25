# scale/

规模化 Profile 把 Snapshot authority 与检索派生分开：

| 层 | 实现 | 状态 |
|---|---|---|
| ⓪ Snapshot | Dolt | `DoltRepository` 用原生 Dolt `kc_files` 版本表实现 SnapshotStore / RawFileStore / Knowledge；commit、branch、AS OF 由 Dolt 提供 |
| ③ 全文 Retriever/Maintainer | Elasticsearch | 高频子集：MATCH 三种 mode、EQ/IN/EXISTS/MISSING/PREFIX；其它算子逐 clause 明确 Unsupported |
| ③ 列投影 | StarRocks | `OpenStarRocks` stub，缺能力显式失败 |

Dolt 优先使用 `KC_DOLT_BIN`，其次是 PATH 中的 `dolt`，最后可用 Docker fallback；`KC_DOLT_DOCKER_IMAGE` 固定镜像，`KC_DOLT_FORCE_DOCKER=1` 强制 Docker。密码只走相应环境变量，不写 stores.yaml。

动态 state/stream 属于 Aspect Binding 指向的上层运行时，不是 scale Repository 或 cache。
