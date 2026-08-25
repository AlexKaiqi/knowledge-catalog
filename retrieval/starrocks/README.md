# retrieval/starrocks/

StarRocks 的 ③ provider adapter。它只声明并实现自己可兑现的列式检索能力，不承担 Snapshot authority，也不让物理表模型进入 Knowledge Schema。

`starrocks.go` 当前集中放配置边界、provider identity 与 capability 实现；扩展到真实 Projection 维护后，再按 config/client/projection/search 变化轴拆分。
