---
entity: kc/guide/first-read
aspect: guide
schema_ref: schema/system/usage-guide/v1
path_hint: guides/first-read/guide.md
---
# 第一次读取对象

1. 客户端已经配置目标 Server，调用者持有有效凭证。
2. 通过身份能力核对主体，用 `kc show` 按名称和用途选择可消费的 Dataset，再用 `kc dataset show <id>` 核对发布修订。
3. 在固定修订中浏览 Dataset 交付的 Schema 和已选入索引声明；已知对象身份时直接读取，未知时用 `kc search --dataset <id> --revision <N> --query <文本>` 搜索。
4. 用 `kc read <对象URN> --dataset <id> --revision <N>` 按同一修订回读命中，保留来源仓、对象与版本坐标。
5. 成功证据：已得到请求范围内的正文及依据。发现来源而无正文、无投影和无匹配分别解释。

正文包含需要取得的文件时，按对象说明中的资源地址继续使用[文件读取能力](../../capabilities/datasets/definition.md)；先读 `kc help files read`，用资源自己的仓、固定版本和路径下载。读取对象本身不会自动取文件或发放文件读取权。

所需能力：[身份与准入](../../capabilities/identity/definition.md)、[进入和查看 登记空间](../../capabilities/inventory/definition.md)、[理解对象类型](../../capabilities/schemas/definition.md)、[搜索对象](../../capabilities/search-object/definition.md)、[精确读取与复核](../../capabilities/read-object/definition.md)。
