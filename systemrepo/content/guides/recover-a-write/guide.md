---
entity: kc/guide/recover-a-write
aspect: guide
schema_ref: schema/system/usage-guide/v1
path_hint: guides/recover-a-write/guide.md
---
# 恢复结果不明的发布

适用于提交或合并的响应丢失、超时，尚不能确定是否接受的情形。先保留原 Server、主体、目标仓与引用、command-id、基点和完整请求；目录发布还要保留当时的文件字节，不能拿后来改过的目录重试。

## 查询原命令

连接保存原命令账的 Server，以有该目标回执读取权的主体执行：

```sh
kc writer receipt --command-id <original-command-id>
```

这一步只查账，不触发发布。核对命令记录的目标与原请求一致，再按状态处理：

- APPLIED：保存回执中的 `receipt.result.newCommit`，进入读回，不再提交。
- PENDING：尚无最终结果，保留原请求交管理者按现行恢复流程对账；等候超时不能解释为失败。
- ABANDONED：维护流程已明确放弃，核对该结论和目标仓状态后，重新读取基点并形成新的发布。
- 未知命令或查询失败：先核对 Server、原身份与当前权限；账本保留窗口、临时不可用都可能导致查不到，不能据此换 id 重写。

## 固定接受版本读回

有成功回执后，用其中的新 commit 读取本次涉及的单元：

```sh
kc read '<object-urn[#/pointer]>' --repo <repository-id> --commit <new-commit>
```

核对身份、完整正文和来源，保留回执与读取依据。当前 HEAD 可能已前进，不能拿最新内容的不同来推断原提交没发生。读回因授权或来源故障失败时，修复读取条件，不重做已接受的写入。

若原请求明确因基点冲突被拒，重读当前内容、重新核对差异，使用新基点和新 command-id 发布。只有原请求原样重放才沿用旧 id；同一 id 换内容会冲突。原请求是否安全重放由现行恢复合同决定，查回执本身不承诺恢复缺失账本。

完成判据是取得唯一已接受回执并按其版本复核，或取得明确未接受的结论及新的处理依据。索引、审计或后置通知失败不会撤销已经接受的提交，不能成为重复发布的理由。

所需能力：[发布与修订对象](../../capabilities/publish-object/definition.md)、[精确读取与复核](../../capabilities/read-object/definition.md)。
