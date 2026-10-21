---
entity: kc/guide/first-publication
aspect: guide
schema_ref: schema/system/usage-guide/v1
path_hint: guides/first-publication/guide.md
---
# 首次发布领域对象

先读已有交付，再整理自己的资料。`kc show` 中的 `qinghe-analysis`、`autumn-launch-kit`、`store-oncall` 分别展示销售分析、上新素材和值班交接；它们的对象文件与 Schema 可以通过公开文件入口读取。选择与任务接近的例子，用 `kc dataset show` 记录 revision，沿 `kc files list` 找到导读、对象及其 `schema_ref` 对应的 Schema。后续列目录与读取固定同一 revision，不从文件名猜对象身份。

以下复用 `autumn-launch-kit` 第 3 版中的商品 Schema 与一个商品说明，练习从公开文件发布到自己的仓。这里只引用已交付路径，不另造一套商品模型。先确认该版可读，准备自己的练习仓及发布、读回权；已有仓直接使用，尚无目标仓时再看 `kc help create`。下载目标必须不存在。

```sh
kc show
kc dataset show autumn-launch-kit --revision 3
kc files list --dataset autumn-launch-kit --revision 3 --path _schemas
kc files list --dataset autumn-launch-kit --revision 3 --path products/osmanthus-oolong
mkdir -p ./drafts/_schemas ./drafts/products/osmanthus-oolong
kc files read --dataset autumn-launch-kit --revision 3 --path _schemas/product.definition.aspect.yaml --out ./drafts/_schemas/product.definition.aspect.yaml
kc files read --dataset autumn-launch-kit --revision 3 --path products/osmanthus-oolong/definition.yaml --out ./drafts/products/osmanthus-oolong/definition.yaml
```

下载的是带文件头的完整对象文件。Schema 身份为 `schema/product.definition`；实例身份为 `product/qinghe.osmanthus-oolong`、方面为 `definition`。原样复用练习保留这些身份；整理自己的业务对象时改用自己的身份、真实正文与来源，沿用匹配的 Schema。不要把示例值当成自己的业务事实。只有现有类型不能表达资料时才设计新的 Schema，声明与实例可以同批提交。

目录文件的头部、正文和路径规则，以及一个完整的通用说明文件，见 `kc help writer commit`。只改一个完整值也可用 `kc writer put`，其 `--file` 只收纯 JSON 值，不收带头部的对象文件。`kc schema describe` 是理解结构的规范化展示，不能作为 Schema 原文提交；要取得原值，用 `kc read` 读取 Schema 对象并取响应的 `value`，或采用上面的 Dataset 原文件。

把下列目标仓、基点与回执版本占位符替换为自己的实际值；`kc writer head` 返回本次对照的 commit，提交时沿用该基点。

```sh
kc writer head --repo <your-repo>
kc diff --repo <your-repo> --base <base-commit> --dir ./drafts
kc writer commit --command-id first-publication-1 --repo <your-repo> --base <base-commit> --dir ./drafts
kc writer receipt --command-id first-publication-1
kc read 'urn:kc:object:schema/product.definition' --repo <your-repo> --commit <new-commit>
kc read 'urn:kc:object:product/qinghe.osmanthus-oolong#/definition' --repo <your-repo> --commit <new-commit>
```

保留本次 command-id；响应丢失先查原回执，不换身份重发。提交接受回执的 `result.newCommit` 是上述读回使用的 `<new-commit>`；查询回执时取 `receipt.result.newCommit`。原样复用练习与公开例子持有相同对象身份，读回显式限定自己的仓，不能用无范围读取来判断本次发布。

成功证据是同一接受版本中的 Schema 能解释实例，内容、身份和来源均可复核；`diff` 不替代 Writer 校验。
系统仓不能作为业务写目标。Meta Schema 与索引声明 Schema 的副本须保持与出版物一致；
其余类型遵守普通兼容规则，边界见 [Schema](../../concepts/schema/definition.md)。
需要搜索时另行发布索引声明；需要实时访问时发布取值描述与引用声明。对象发布、插件安装和投影就绪分别核对。
需要向同事交付时继续 [向消费者交付 Dataset](../share-a-dataset/guide.md)。

所需能力：[理解对象类型](../../capabilities/schemas/definition.md)、[发布与修订对象](../../capabilities/publish-object/definition.md)、[精确读取与复核](../../capabilities/read-object/definition.md)。
