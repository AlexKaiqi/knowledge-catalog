# QUESTION-DATASET-01 · 声明与交付是否真的一致

## 要回答的问题

`itemCount` 与实际可列出条目数是否吻合；`files read` 字节与 `files list` 声明大小是否一致；
`search` 命中的 `objectId` 能否在同一 revision 被 `read` 解析；来源 commit 是否与库存仓映射一致；
`dataset define` 的 target 是否允许 `../` 穿越；`dataset retire` 后三面行为是否一致。

## 已知证据

`qinghe-analysis` 实测 `itemCount: 23`、`items` 23 条，数目吻合。`qqbrowser` 的 metadata 无标题，
其余三个有中文标题，库存判断力不均。

任务书称 `dataset show __x__` 的错误码为 `USAGE_INVALID`，实测为 `DATASET_INVALID`，需重跑确认。

## 判定标准

- 四个 Dataset 的 `itemCount` 与可列条目数全部吻合，关闭为"一致"；有不吻合，关闭为"确认缺陷"并给出
  差异来源。
- 抽样（不少于 5 个文件、含跨 mount 路径）字节一致，关闭为"一致"。
- 同 revision 回读成立、跨 revision 明确报错，关闭为"合同成立"；静默降级，关闭为"确认缺陷"。
- `../` 若被接受且越出挂载根，关闭为"确认缺陷"；被拒绝或限定在根内，关闭为"安全"。
- `retire` 后三面能区分"已退役"与"无权限"，关闭为"合同成立"。

## 前提

无。

## 不涉及

不对既有 Dataset 执行 retire、不改已发布修订、不在业务仓做写操作。

## 结论

待填。
