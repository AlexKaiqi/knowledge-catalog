# access-runtime-ready

走查前态：各仓 `writer commit --dir` 写入本节点清河茶铺材料。`table-meta` 是库/表/列、作业与血缘；`sales-semantic` 是语义模型、维度、度量、指标。`schema/table.stats` 的 Bound State origin 对所有 `table/shop.*` 实体生效，对着活 MySQL `COUNT(*)`。源目录变更由 Collector 对账后 Writer 更新 `table-meta`；Observer 对每张表的 `stats` 发 notice。只读 SQL 走 `resource/shop-sql`。索引访问在后继 `index-ready`。

`python3 .data/scenes/goto.py sales-dataset-defined`（或 `make deploy-local-goto NODE=sales-dataset-defined`）在已启动的走查环境上重放到 Dataset 叶。检索叶仍是 `index-ready`。
