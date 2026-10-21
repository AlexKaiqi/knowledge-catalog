---
entity: kc/guide/share-a-dataset
aspect: guide
schema_ref: schema/system/usage-guide/v1
path_hint: guides/share-a-dataset/guide.md
---
# 向消费者交付 Dataset

适用于把已发布的来源文件交给其他主体消费。发布者须有目标 Dataset 的管理权，以及每个来源的内容发布许可；来源读取权不能替代发布许可。先写下消费者的任务、要交付的文件和不应交付的相邻文件。

## 选择范围并发布

核对来源已登记、分支可解析，确定交付路径。对象消费要选入任务所需方面及解释材料；搜索还须选入适用索引声明。文件交付本身不要求索引，也不承诺自动补齐对象。

以下把一个来源的 `handbook` 子目录交付为 `reference`。替换仓身份和 Dataset 名；先用 `kc dataset show <dataset-id>` 核对是否已有发布，已有时选择大于已接受版本的新号。当前版本 CLI 要求显式 revision，未来服务端分配的目标尚未落地。

```sh
kc dataset define <dataset-id> --revision <next-revision> --source '<repository-id>=refs/heads/main@reference@handbook'
kc dataset show <dataset-id> --revision <accepted-revision>
```

保留接受结果的 revision、冻结来源 commit 和交付路径，而不是把期望发布的版本当作成功。路径冲突、来源不可用、缺发布许可或索引准备失败时，修正候选后重试；原发布版继续服务。响应丢失先查询同版记录，核对完整定义；不能覆盖一个已接受版本，也不能盲目加号重复发布。需要多来源或单文件映射时，沿同一流程准备配方，精确语法见 `kc help dataset define`。

## 授权并由消费者复核

确定消费者主体后，另行授予 Dataset 文件消费权：

```sh
kc grant add --dataset <dataset-id> --principal <consumer-principal> --action file.read
```

授权由有管理权的主体执行；发布成功不隐含授权。消费者用自己的已认证身份核对固定发布版，再列出和读取交付文件：

```sh
kc dataset show <dataset-id> --revision <accepted-revision>
kc files list --dataset <dataset-id> --revision <accepted-revision> --path reference
kc files read --dataset <dataset-id> --revision <accepted-revision> --path reference/<selected-file>
```

产物是消费者实际取得的文件及原仓、commit、路径依据。无权时核对该消费者的 Dataset 消费授权；路径缺失时核对发布清单，不能直接补读源仓。需要完整对象时，在同一 revision 下用 `kc read` 复核；需要搜索时另外确认查询能力就绪，未就绪不能算零命中。

完成判据是消费者可读取选中内容、不能越过交付清单，来源坐标与接受记录一致。消费者无需取得源仓读权。来源后来更新不会改变旧版；重新发布并确认新版接受后，消费者明确切换到新版继续工作。下载到客户端的字节不因撤权自动收回。

所需能力：[组合与采用 Dataset](../../capabilities/datasets/definition.md)、[查询与管理授权](../../capabilities/permissions/definition.md)、[精确读取与复核](../../capabilities/read-object/definition.md)。
