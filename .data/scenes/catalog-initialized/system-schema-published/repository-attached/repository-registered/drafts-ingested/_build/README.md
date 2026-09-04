# drafts-ingested

`kc pack --dir $materials/drafts --out $home/schema.changeset.json` 把 Domain Schema 草稿预览成 ChangeSet。stdout 报告文件与诊断，不含 `changeSet`，不发表。下一节点 `writer commit` 才进权威。

这与墙外 Collector `Preview`（`changeset-previewed`）相邻：一条是人/Agent 目录草稿，一条是源系统对账。
