# retrieval/

③ 的物理 provider adapters。逻辑查询与公开结果契约在 `reader/`，候选/Projection 端口和执行编排在 `index/`；本目录只把这些端口翻译到具体介质。

| 目录 | 定位 |
|---|---|
| `sqlite/` | reference profile；本机可重建 Projection，覆盖常见 typed 查询 |
| `elasticsearch/` | 规模化文本/过滤子集；未实现算子必须明确 Unsupported |
| `starrocks/` | StarRocks provider 能力边界；不冒充 Snapshot authority |

Provider 只返回带 basis 的 `CandidateRef`，不得把 `_source`、stored field 或物理 score payload 当 Canonical 返回。
