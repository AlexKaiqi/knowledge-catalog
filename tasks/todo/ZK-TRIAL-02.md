# ZK-TRIAL-02 · 发现与消费零知识试跑

## 目标

在真实部署上取得陌生 Agent 认识环境、理解业务和解决问题的真实记录。Agent 只拿到 `kc`、服务地址、只有
一份 Dataset 消费权的身份、MySQL 连接串与人的原话，要自己从库存找到 Dataset、在 Dataset 内搜索、按同一
修订回读对象，必要时读取外部值。结论区分试跑基础设施、任务完成与产品缺口。

## 依据

- [Affordance](../../doc/design/affordance.md)、[发现](../../doc/design/discovery.md)、[Dataset](../../doc/design/dataset.md)。
- [LiveAccess](../../doc/design/liveaccess.md)、[权限](../../doc/design/permissions.md)、[测试与验证](../../doc/engineering/testing.md)。

## 验收

- 规格
  [zk-01-environment](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/zk-01-environment.yaml)、
  [zk-02-business](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/zk-02-business.yaml)、
  [zk-03-tasks](../../.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_specs/zk-03-tasks.yaml)
  每条都有真实运行记录，按 `_materials/zero-knowledge/README.md` 的判定与封存答案逐项判定。
- 隔离与 `ZK-TRIAL-01` 相同：Agent 看不到源码、封存答案、其他身份凭证或模型密钥；KC 网关固定本次主体；
  每条任务用例开跑前按 README 重置 MySQL 世界。
- 保存环境、候选源码或镜像、完整工具记录、最终回答与独立状态核对；不把模型结束或命令退出零当作通过。
- 归因沿用 `ZK-TRIAL-01` 确立的信息充分性原则：只为信息本身不足修产品；资料足够时 Agent 的漏读、漏做或
  推断失误不算产品缺口，不为此追加提示或模型成功率优化。确认的产品缺口按三出口登记。
- `make check-docs` 通过。

## 前置

- `INDEX-SEARCH-SPACE-01`：搜索只按 Dataset 范围。
- `DEBT-INVENTORY-CONSUMPTION-01`：库存只列出可消费的 Dataset。
- `PUBLICATION-SYNC-01`：系统出版物与 `kc help` 说明的是新行为，否则试跑测的是过时说明。
- 真实测试部署健康，含 README 所列走查 compose（`resource-access` 与 MySQL）；环境恢复与共享搜索容量见
  `ACCEPT-HARNESS-01` 并入的环境债。复用 `ZK-TRIAL-01` 的试跑材料及已批准模型端点。

## 不做

不重做接入方发布旅程（`ZK-TRIAL-01` 已覆盖）；不调整在途实现，不提交代码，不重置已有部署和权威数据；
单一模型的若干次旅程不作为任意 Agent 成功率的结论。
