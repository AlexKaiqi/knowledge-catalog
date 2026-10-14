---
entity: kc/guide/review-a-change
aspect: guide
schema_ref: schema/system/usage-guide/v1
path_hint: guides/review-a-change/guide.md
---
# 评审并合入变更

1. 目标仓、变更内容和评审范围已确定，主体有新分支的 writer.branch、分支的 writer.commit 与目标引用的 writer.merge。
2. 从当前发布版本开工作分支，把变更写到该分支；main 不动。
3. 业务系统按分支 commit 完成评审；需要在 KC 里强制审批时配置前置 writer.merge Hook。
4. 按审阅的分支 commit 与目标 commit 请求合并；分支或目标移动则在新基点上重新评审。
5. 成功证据：合并回执固定发布版本，可据此读回目标内容。

所需能力：[发布与修订对象](../../capabilities/publish-object/definition.md)、[精确读取与复核](../../capabilities/read-object/definition.md)。
