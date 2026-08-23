# scale/

规模化方案集。Snapshot / Stream / ③ 引擎分开。CLI `profile: scale`。

| 层 | 实现 | 状态 |
|---|---|---|
| ⓪ Snapshot | Dolt | `DoltRepository` 用原生 Dolt `kc_files` 版本表实现 `SnapshotStore` / `RawFileStore` / `Knowledge`；commit、branch、AS OF 都由 Dolt 提供，不创建 `.git` |

运行时优先使用 `KC_DOLT_BIN`，其次使用 `PATH` 中的 `dolt`。未安装本机二进制时可用
Docker fallback；`KC_DOLT_DOCKER_IMAGE` 固定镜像，`KC_DOLT_FORCE_DOCKER=1` 可强制走该路径。
生产服务建议使用本机 Dolt 二进制或专门部署的 adapter，Docker fallback 主要用于可复现验收。
| ⓪ Append | 有序段 | `OpenStream` 返回 `repository.Stream`（stub Append）；**不是**仓 |
| ③ 全文 | Elasticsearch MATCH | `OpenElasticsearch` 已有 |
| ③ 列索引 | StarRocks | `OpenStarRocks` stub |
| ③ 热尾 | Redis | 只做 cache，不是仓 |

不要 `repo-add --driver stream`。不要 `--driver mysql`。协议根不长 Hippo SR connector。不要把 Redis 当仓或比较引擎。
