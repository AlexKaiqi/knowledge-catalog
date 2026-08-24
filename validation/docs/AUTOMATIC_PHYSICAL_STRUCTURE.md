# 物理表结构自动更新验收

日期：2026-08-25

这条验收回答一个具体问题：MySQL 物理表发生 DDL 后，墙外 Connector 是否能
在无人手工执行写入的情况下，把物理结构知识更新到 Repository，同时保持已
开始任务的 Workspace pin 可复现。

## 边界

`kc` 不发现、部署或定时运行源系统 Connector。运行责任属于数仓场景侧的
Integration Host；它通过公开 HTTP facade 调 Writer，不进入协议根，也不直写
Git。通用 `connector.Preview` 只负责 Address Scope 对账。

```text
MySQL information_schema
  → scheduled Collector（FULL STATE）
  → Address reconcile
  → POST /v1/commit（connector/mysql-structure-auto）
  → Snapshot commit
  → fresh Workspace resolve
```

## 固定初态

- 源：digest-pinned MySQL 8.4.8，TPC-H SF0.01；
- 结构：8 张表、61 列；
- 目标：`kr://tpch/validation/auto-physical`；
- Workspace：`tpch-auto-physical`；
- Connector generation 必须先验证并显式激活；
- 触发器：`schedule every: 1s`；
- Connector principal 只能在目标 Repository 的 main 上执行 `commit`。

手工调用只执行 preview，断言 69 个新增 Address。随后启动 Host，第一次实际
写入必须来自 scheduler。首次成功后保存 Workspace pin P0。

## 真实 DDL

```sql
ALTER TABLE orders
  ADD COLUMN o_pipeline_note VARCHAR(64) NULL
  COMMENT 'scheduled metadata probe';

ALTER TABLE customer
  MODIFY COLUMN c_phone VARCHAR(32) NOT NULL;

ALTER TABLE part
  DROP COLUMN p_comment;
```

对应 Address 差异为：

- `added=1`：新增列；
- `updated=3`：orders 的 `columnCount`、customer.c_phone、part 的
  `columnCount`；
- `removed=1`：part.p_comment；
- `unchanged=65`。

## 不变量

1. 两次实际写入的 run trigger 都是 `schedule`；
2. FULL observation 才允许 reconcile 产生 REMOVE；
3. Writer 成功后才推进 Host checkpoint；
4. 新增列保留 `originKind=SOURCE`、MySQL source ref 和固定 Connector actor；
5. fresh pin P1 看到 orders 10 列、part 8 列和 `varchar(32)` phone；
6. old pin P0 仍看到 orders 9 列、part 9 列、`char(15)` phone 和
   part.p_comment；
7. P0 不包含新增列，P1 不包含删除列。

## 边界审计

2026-08-25 对自动更新链重新从失败不变量审计，并补充以下真实运行断言：

- **generation 漂移**：激活后向 integration repo 推送新 bundle，Host 自动同步
  后必须拒绝 schedule；target head 与 checkpoint 都不变。只有重新 validate +
  activate 后才恢复执行；
- **源暂时不可用**：暂停真实 MySQL，scheduled run 必须 `FAILED`，不得推进
  checkpoint 或 target；恢复 MySQL 后由下一次 schedule 自动重试；
- **稳定源 no-op**：重新激活后、源恢复后、DDL 提交后的稳定观察均为 `EMPTY`，
  只允许记录观察 checkpoint，不允许创建新的知识 commit；
- **自动性证据**：最终 run history 动态计算 `manual + SUCCEEDED` 数量，必须为
  0，不能用报告里的常量冒充自动执行；
- **不完整快照**：Collector 拒绝重复 table 行，以及“可见 table 但没有任何
  可见 column”的 observation，避免把明显不完整的读取声明成 FULL。

通用测试已覆盖而未在 DW-04 重复注入的边界包括：过期 base 的
`NON_FAST_FORWARD`、Writer 失败 checkpoint 不前进、Scope 外 Address 拒绝、
KEYED observation 禁止 reconcile REMOVE、同一 command digest 幂等重放。

仍需明确的产品边界：MySQL 名称是当前 source key 的组成部分，因此表/列 rename
表现为旧身份 REMOVE + 新身份 PUT，而不是自动保持对象身份；若上游能提供稳定
内部 ID，应由场景 Connector 改用该 ID。另一个无法由通用 Host 单独判断的风险
是“权限导致整个表对 Collector 不可见但查询仍成功”，生产 integration 必须用
声明的 source boundary、独立权限探针或 inventory oracle 证明 FULL coverage。

## 执行与证据

```bash
./validation/playbook.sh DW-04
```

固定 oracle：

```text
validation/fixtures/tpch-sf001/expected/dw04.json
```

ignored 机器证据：

```text
.data/datawarehouse/actual/dw04.json
.data/datawarehouse/dw04/evidence/
```
