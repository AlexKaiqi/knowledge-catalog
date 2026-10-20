---
entity: kc/guide/revise-an-aspect
aspect: guide
schema_ref: schema/system/usage-guide/v1
path_hint: guides/revise-an-aspect/guide.md
---
# 修订对象的一个切面

适用于已存在对象的一个完整方面需要修改，其它方面应保留的情形。先确定目标仓、对象 URN、方面名，以及该仓的读取、预览和发布权限。组合读取中的方面可能来自不同仓，写目标须取自该方面的来源依据。

## 读取并准备完整值

先记录目标仓的基点，再在这个 commit 读取目标方面和完整对象，保留修改前的值及来源：

```sh
kc writer head --repo <repository-id>
kc read '<object-urn>#/<aspect>' --repo <repository-id> --commit <base-commit>
kc read '<object-urn>' --repo <repository-id> --commit <base-commit>
```

核对该方面采用的 Schema，按约束准备完整替换值，保存为纯 JSON 文件。只取响应的 `value` 并编辑它，不把响应包络、来源依据或文件头当成值。PUT 是完整替换：遗漏字段会被移除；只想改一个深字段，也要先取得其所属完整维护单元。成员集合应按成员维护，不用完整方面覆盖它。

## 条件发布并读回

替换占位符后，明确沿用刚才读取的基点：

```sh
kc writer put '<object-urn>#/<aspect>' --repo <repository-id> --base <base-commit> --command-id <new-command-id> --file ./aspect-value.json
kc read '<object-urn>#/<aspect>' --repo <repository-id> --commit <new-commit>
kc read '<object-urn>' --repo <repository-id> --commit <new-commit>
```

新 commit 取自接受回执的 `result.newCommit`。Schema 校验失败时修正完整值再发布；基点冲突时重读新的方面和其它方面，核对他人的修改，再用新基点及新命令身份发布。响应丢失时先走[恢复结果不明的发布](../recover-a-write/guide.md)，不能换 id 重发。

完成判据是同一对象身份下，目标方面等于预期完整值，其余方面与修改前逐项一致。读回要限定目标仓和接受 commit；无范围的组合结果可能包含别仓方面，最新 HEAD 也可能已有别人的后续修订。需要修改其它仓时另行提交，不把多次单仓操作当作原子事务。

所需能力：[精确读取与复核](../../capabilities/read-object/definition.md)、[理解对象类型](../../capabilities/schemas/definition.md)、[发布与修订对象](../../capabilities/publish-object/definition.md)。
