# internal/datasetbridge/

Dataset 解析结果到对象读取依据的**唯一字段映射**。输入是 `snapshot/dataset.ResolvedDataset`，
输出是 `object/reader.DatasetPin`；生产 Home、共享夹具（`internal/testkit`）与插件测试调用
同一个 `Pin`，不再各自抄一份字段。

转换是纯函数：只读一个已解析值、写一个 pin，不取得服务句柄、不访问来源、不解析 HEAD。
`Dependencies` 按解释依赖原样冻结到 pin 上，但不进入 `Repositories` 或 `Items`——依赖是解释所用
的版本，不是选入成员，扩大成员范围会让被挡住的仓变成可读。`Items` 保持恰好是已发布的文件范围，
空选入范围不会被扩大成整仓。来源与依赖的版本 map 会被复制，转换之后来源推进不改写已冻结坐标。

本包不拥有发布准备、冲突检查或依赖回调；那些仍留在根 Home。它也不作为 `object/reader` 的一部分：
`object` 层不得依赖 `dataset` 层，桥接显式归上层并由 `internal/arch` 的禁止清单限制为只依赖
两个公开形状。
