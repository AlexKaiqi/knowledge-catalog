# index-ready

走查检索前态：对 `table-meta`、`sales-semantic` 做 `operations projection sync`，再用 `kc search` 从索引命中表和指标。不是 READ，也不是 Bound State `kc access`。两仓表层对象打包在后继 `sales-dataset-defined`。

`python3 .data/scenes/goto.py index-ready`
