# Host file projection

定位：整理稿。能力简介，正文后续梳理。外围能力，不进核心五层。

把已冻结的文件树投影成宿主 mount。只读；pin 不随 HEAD 移动。

**独立验收。** FUSE / kcfs：只读、pin 不移动。

**不依赖也能说清的失败。** mount 自动 COMMIT；root-mount 冒充 Store。

**明确不是。** Snapshot、Writer。
