# sales-dataset-defined

走查叶：`kc dataset define qinghe-sales` 从 `table-meta` 只挂 `tables/`，从 `sales-semantic` 只挂 `semantic-models/` 与 `metrics/`。冻结后 `read --dataset` 能读订单表、销售模型和 GMV，读不到作业。不是整仓别名。

`python3 .data/scenes/goto.py sales-dataset-defined`（或 `make deploy-local-goto NODE=sales-dataset-defined`）
