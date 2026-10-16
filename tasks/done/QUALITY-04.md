# QUALITY-04 · 耦合 ratchet 升格与二期指标

- [x] 已落地（2026-09-24）：`scripts/coupling` 增加 `-strict` 硬门禁并接入 `make quality`（`scripts/check-quality.sh`）——卡传播成本超基线、Instability drift ≥ 0.05、基线外新单元（新单元须显式 `-write-baseline` 收紧并记本文件）；基线文件新增抽象度 A / 主序列 D / 公开面密度字段（传播成本与 Instability 口径不变，仍为 23 单元 / 38.74%），A/D/密度为正则文本代理（const 块成员不计、未走 go/types），只观测不拦，确认信号后再升格。失败路径经篡改基线验证（exit 1、报因正确）。方法与读法约定见 [`doc/engineering/quality.md`](../../doc/engineering/quality.md) §7。验收：go vet / gofmt 净；`make quality` 全绿且尾部输出 `coupling ratchet: held`（2026-09-24）。
