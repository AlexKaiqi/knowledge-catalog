---
entity: kc/guide/first-read
aspect: guide
schema_ref: schema/system/usage-guide/v1
path_hint: guides/first-read/guide.md
---
# 第一次读取对象

适用于已有 KC Server 和有效身份，希望从共享内容里找到并读取一个对象的读者。先确认连接的是预期服务；没有登录或权限时，先完成身份与授权，不把空库存当作内容不存在。

## 选 Dataset 并固定版本

```sh
kc show
kc dataset show <dataset-id>
```

从库存按名称和用途选出可消费的 Dataset，记录 show 返回的具体 revision。后面的查找、翻页与读取都使用这个号；不要在每一步重新取 latest。库存可见只说明有发现入口，不证明正文读取或搜索已经就绪。没有合适的 Dataset 时，向发布者申请交付，不能为了搜索而扩大到所有源仓。

以下命令以 revision 3 演示；执行时将 `3` 换成刚才确认的实际版本号。

## 找到对象并读回

已知对象 URN 时直接读取；未知时，先浏览同版文件中的 Schema 和索引声明，确认搜索适用，再查询：

```sh
kc search --dataset <dataset-id> --revision 3 --query '<task-text>'
kc read '<object-urn>' --dataset <dataset-id> --revision 3
```

从命中取对象坐标，不把标题或交付路径当作身份。读回后保存正文、各方面的来源仓与 commit、采用的 Schema 及错误信息。合法的 null 与未取得内容要按返回依据区分。

查询未就绪时等待或联系维护者处理声明与投影，不能现场改索引，也不能当作零命中。查询成功且无匹配时，再核对任务词与交付范围；命中但读取无权、缺失或方面冲突时，按对应错误处理，不能偷偷切换到源仓或最新版本来凑结果。Schema 未选入导致解释受限时向发布者反馈，不擅自扩大范围。

## 取得正文引用的文件

若对象说明引用素材或文档，先核对资源自己的仓、固定 commit 和路径，再用[文件读取能力](../../capabilities/datasets/definition.md)取得字节；已在 Dataset 中的文件也沿用本次 revision：

```sh
kc files read --dataset <dataset-id> --revision 3 --path <selected-path>
```

对象读权不会自动产生其它资源的文件读权，读取对象也不会自动下载文件。文件不存在或无权时保留失败与坐标，不改读另一个版本。

完成判据是所需正文及来源依据已取得，范围内未取得的部分有明确原因。搜索命中、库存可见或说明里有路径都不能单独证明内容已交付。

所需能力：[身份与准入](../../capabilities/identity/definition.md)、[进入和查看 登记空间](../../capabilities/inventory/definition.md)、[理解对象类型](../../capabilities/schemas/definition.md)、[搜索对象](../../capabilities/search-object/definition.md)、[精确读取与复核](../../capabilities/read-object/definition.md)。
