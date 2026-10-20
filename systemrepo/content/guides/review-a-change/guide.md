---
entity: kc/guide/review-a-change
aspect: guide
schema_ref: schema/system/usage-guide/v1
path_hint: guides/review-a-change/guide.md
---
# 评审并合入变更

适用于需要先检查候选内容，再发布到目标引用的变更。先确定目标仓和评审范围；主体须有新分支的 writer.branch、分支的 writer.commit，以及目标引用的 writer.merge。示例目标为默认 main，系统仓不接受此流程。

## 固定基点并提交候选

取得 main 的 commit，以该 commit 开工作分支，再将已准备的对象文件经 Writer 提交：

```sh
kc writer head --repo <repository-id>
kc writer branch --repo <repository-id> --name review/change-1 --commit <main-commit>
kc writer commit --repo <repository-id> --ref refs/heads/review/change-1 --base <main-commit> --command-id <draft-command-id> --dir ./drafts
```

保留创建返回的引用与提交回执。分支已存在时先核对其起点和内容，不直接沿用同名分支；起点不存在则核对 commit。提交校验失败时修正草稿，响应丢失先查原回执。此时 main 未推进，分支也不会自动跟随 main。

## 审阅固定 commit

从分支提交回执取得候选 commit，在该版本读取修改对象并比较原 main 内容；业务评审和 CI 的结论都关联这一候选 commit，而不是可移动的分支名。例如：

```sh
kc read '<object-urn>' --repo <repository-id> --commit <reviewed-commit>
```

产出是业务侧批准的候选 commit 与目标基点。KC 不保存审批结论；需要强制审批时，由管理者预先在 writer.merge 上配置前置 Hook。评论通过不会自动产生合并权，分支写入也不会发布 Dataset。

## 条件合并与复核

带着审阅过的两个 commit 合并：

```sh
kc writer merge --command-id <merge-command-id> --repo <repository-id> --from review/change-1 --commit <reviewed-commit> --expected <main-commit>
kc read '<object-urn>' --repo <repository-id> --commit <new-commit>
```

候选移动就审阅新 commit；目标推进、不能快进时从新 main 开分支，重新应用并评审变更，不自动变基或三方合并。Schema 或前置 Hook 拒绝时修正对应条件后重新评审；不能省略版本保护来绕过拒绝。合并响应丢失，查询原 command-id 的回执。

完成判据是合并回执固定了新发布 commit，且在该 commit 读回审阅内容。后置通知失败不回滚合并；源仓发布成功也不代表 Dataset 已采用新版，需要交付时另行发布 Dataset。

所需能力：[发布与修订对象](../../capabilities/publish-object/definition.md)、[精确读取与复核](../../capabilities/read-object/definition.md)。
