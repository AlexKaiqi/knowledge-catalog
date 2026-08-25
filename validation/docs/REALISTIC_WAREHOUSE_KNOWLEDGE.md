# 真实数仓知识图谱验收

这条验收补足“协议操作能通过，但知识内容彼此孤立”的缺口。目标不是模拟一套
完整调度器或查询引擎，而是在一个固定 Workspace pin 上构造足够真实、自洽、
可追溯的数仓知识。

MySQL source ref 固定为 DW-01/DW-02 使用的
`mysql://127.0.0.1:13306/tpch`；源 Table/Column 使用相同的 source key 和
SHA-256 截断映射，因此专项血缘引用的是实际采集用例中的同一批稳定身份。

## 用户问题

验收必须让分析师和审计者回答：

1. GMV 的定义、粒度、时间口径、owner 和认证状态是什么？
2. GMV 依赖哪个 Measure，最终读取哪张 DWS 表的哪一列？
3. 哪些 ETLTask 生成 DWS/DWD 表，关键输出列由哪些源列计算？
4. 源表可关联的证据是什么，它为什么不等于生产血缘？
5. Ranger 中谁有数据 SELECT 权限，它为什么不能放行 Knowledge Catalog？
6. 当前任务固定了哪些 Snapshot commit 和 ETL run cursor？后续失败运行会不会
   改写已经开始的分析？

## 两仓与一条流

```text
Workspace finance-analyst-board
├── kr://acme/validation/warehouse-metadata
│   ├── DataSource / ResourceDescriptor / Database / Schema
│   ├── source + ODS + DWD + DWS Table/Column
│   ├── ETLJob / ETLTask / QualityRule
│   ├── structure / profile / freshness / joinEvidence
│   ├── inputs / outputs / columnMappings
│   ├── permissions / classification / ownership
│   └── Stream etl-runs（Catalog 只冻结 AppendCut）
└── kr://acme/validation/warehouse-semantics
    ├── MetricView / Dimension / Measure / Metric
    └── definition / dependencies / ownership / certification
```

Schema 不作为项目源码保存。测试先通过 Writer 把 `schema/*` 对象写入对应知识
Repository，再让所有 Aspect 用 `schema_ref` 解析这些对象。

## 固定业务链

```text
mysql.tpch.orders ─────→ ETLTask:sync-orders ────→ ods.orders ───┐
mysql.tpch.lineitem ───→ ETLTask:sync-lineitem ─→ ods.lineitem ─┤
mysql.tpch.customer ──────────────────────────────────────────────┤
mysql.tpch.nation ────────────────────────────────────────────────┘
                                  ↓
                     ETLTask:build-trade-order
                                  ↓
                        dwd.trade_order
                                  ↓
                   ETLTask:aggregate-sales-daily
                                  ↓
                         dws.sales_daily
                                  ↓
             MetricView:sales / Measure:gross-sales
                                  ↓
                            Metric:gmv
```

关键列级映射：

```text
ods.lineitem.extended_price = mysql.tpch.lineitem.l_extendedprice
ods.lineitem.discount_rate  = mysql.tpch.lineitem.l_discount

dwd.trade_order.discount_amount
  = ods.lineitem.extended_price * ods.lineitem.discount_rate

dwd.trade_order.net_amount
  = ods.lineitem.extended_price * (1 - ods.lineitem.discount_rate)

dws.sales_daily.gmv_amount
  = sum(dwd.trade_order.net_amount)
```

源表 `lineitem → orders` 另有 `joinEvidence`：关联列、孤儿数和 SQL digest。
它只证明数据可关联，不声明哪个任务产生了哪个输出。

## 权限边界

fixture 中的 `permissions` 是 Ranger 数据平面快照：

- `group:finance-analysts` 对 `Table:dws.sales_daily` 有 `SELECT`；
- `group:data-engineering` 对 `Table:dwd.trade_order` 有维护权限；
- customer name/phone 列带 PII 分类和 masking policy 引用。

测试同时确认一个只有 Ranger grant、没有 `kc allow` 的主体仍不能获得
Knowledge Catalog 的 `read-workspace` 权限。权限 Aspect 可以被
`AspectSelector` 裁掉，但 Canonical 内容仍保留。

## 运行历史

ETLJob/ETLTask 定义、schedule 和最新 `runtimeSummary` 是 Snapshot 知识；每次
运行是 `etl-runs` Stream 中的不可变事件。Workspace 首次解析时固定 cursor=2，
随后追加第三条 FAILED 运行：

- 使用旧 AppendCut 仍读到两条成功运行；
- live Stream 读到三条记录；
- 已开始的分析不会因为新失败事件漂移。

## 执行

```bash
./validation/playbook.sh REALISTIC-KNOWLEDGE
```

机器报告写入 ignored 文件：

```text
.data/datawarehouse/scenarios/realistic-warehouse-knowledge.json
```

报告记录两仓 commit、Workspace run cut、实体类型、Aspect、从 GMV 到源列的
完整路径，以及解析成功的关系目标数。

## 当前结果

2026-08-24 执行结果：**PASS**。

- Workspace 在两个固定 commit 上列出 88 个知识对象（包含 `schema/*`）；
- 13 类领域实体、15 种 Aspect；
- 自动发现并验证 45 个不同的对象引用，全部在同一 Workspace pin 上唯一解析；
- GMV 路径逐跳经过 MetricView、Measure、DWS 列、聚合任务、DWD 列、构建任务、
  ODS 列、同步任务，最终到达两个 MySQL 源列；
- 固定 run cursor 为 2，追加一条失败运行后 live cursor 为 3，旧 pin 仍只读两条。
