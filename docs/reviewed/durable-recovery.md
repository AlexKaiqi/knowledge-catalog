# Durable recovery

定位：整理稿。能力简介，正文后续梳理。

进程没了以后，Catalog / 授权 / 命令账 / outbox 从耐久权威恢复到同一逻辑结果。

**独立验收。** 重启后续用同一逻辑结果，不重复仓、不假失败。

**不依赖也能说清的失败。** 登记表在 `/tmp`；PENDING 永占 command id。
