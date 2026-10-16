# DEBT-WRITER-SOURCE-PATH-01 · Writer 把调用方本机目录写进对象来源

## 债

- 位置：`object/writer/preview.go` 的目录出版；`--dir` 原样进入 `provenance.sourceRefs` 与提交说明 `ingest <dir>`。
- 信号：2026-10-15 发布公开例子 Dataset 时，进程内测试读到的交付文件带有发布者机器上的绝对路径（`/Users/.../examples/updates/gmv-caliber-v2`）。
- 判定：真缺口，低危：不泄露凭据，但把调用方本机布局写进可公开读取的对象；同一份材料从不同目录发布，来源也不同。
- 处置：由设计裁定目录出版的来源应记录什么（调用方声明的来源标识，或不记录本机路径），再改 Writer 与回执；在此之前公开例子脚本以相对路径提交规避。
- 触发条件：下次触碰 Writer 目录出版或来源模型，有第二个公开 Dataset 发布方出现，或对外演示从本机目录发布对象前（演示会把演示者的本机路径写进可公开读取的来源）。

## 目标

目录出版写入的来源不含调用方本机路径，且对同一材料稳定。

## 依据

- [对象](../../doc/design/object.md)：来源回答“这份内容依据什么产生”，本机目录是调用方布局不是内容依据；来源信封由发布方声明。

## 验收

- 回归测试：从两个不同绝对目录提交同一材料，交付的来源一致且不含本机路径。
- `make check-docs`、`make quality`、`make test` 通过。

## 前置

无。

## 不做

不改变对象身份与内容摘要的计算。

## 交付（2026-10-16）

### 实现范围

- `object/writer/preview.go` 的 `Ingest` 不再伪造来源信封：无声明时 `ChangeSet.Provenance` 为 nil，提交说明从 `ingest <dir>` 改为稳定字面量 `ingest directory`。本机目录是调用方布局而非内容依据，依对象设计篇“来源回答这份内容依据什么产生”，来源由调用方经共享来源标志（`--origin-kind`、`--source-ref` 等）显式声明，`invocation` 层 `OriginFrom` 在 `IngestDesired` 中替换信封的原逻辑保留不变。
- 新增回归 `object/writer/ingest_source_path_test.go` 的 `TestIngestDoesNotRecordLocalDirectoryInProvenance`：同一材料从两个不同临时目录发布，两次均无来源信封、提交说明一致且不含路径分隔符。
- 同批改写 `object/writer/preview_test.go` 的 `TestT7Ingest` 旧断言（原断言“目录出版必带 OriginSource 信封”防的是来源丢失风险）：来源声明风险现由调用方标志路径覆盖——`cli/desired_replay_test.go` 既有回归证明显式 `--source-ref` 写入信封且不同来源产生摘要冲突被拒绝，无来源能力回退。

### 验证范围

- 先红后绿：旧实现上新回归以“SourceRefs 含 `/var/folders/.../kc-3884742594` 本机路径”失败，实现后通过。
- `go test ./object/writer/` 全绿；受影响 `go test ./invocation/ ./cli/`（desired/ingest/replay/writer 选择组）全绿；`make check-docs`、`make quality`（耦合基线持平）、`make check-validation`、`make test` 全部通过。
- 真实 lakeFS 的目录上传行为由 `WRITER-INITIAL-UPLOAD-01` 既有 16 项真实部署验收覆盖；本轮 message 字面量变化不触发传输语义差异，无新增 skip。
