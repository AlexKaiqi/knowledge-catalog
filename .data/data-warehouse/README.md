# MySQL + semantic knowledge acceptance

这里仅有一套数仓知识提供方验收：真实 MySQL 数据源经过 Collector 和
Connector FULL 对账进入物理知识仓，显式发布的语义知识引用这些物理对象，
消费者再通过固定 Workspace pin 读取和查询关系。Binding/QueryLog 不在本轮
夹具中，等待独立任务冻结合同后再增加。

```text
MySQL INFORMATION_SCHEMA
  -> connector/adapter.py
  -> connector/collector.py
  -> connector/connector.yaml + connector.Preview
  -> kc commit -> physical Repository
knowledge/**/*.aspect.yaml + knowledge/**/*.okf
  -> kc ingest preview -> kc commit -> semantic Repository
physical + semantic -> Workspace pin -> read/search/relations/provenance
```

## 用例规范

用例使用 Cucumber Gherkin，Behave 负责执行。一个 `Scenario` 是一个可以独立
运行的局部场景；每个 `When` 后必须立刻出现 `Then`。命令行直接写真实命令，
预期只观察退出码、stdout JSON、生成文件或最终 KC 状态。`check.sh` 会静态检查
这一约束。

这里没有 `action` DSL，也不把真实结果改写成 `catalogCount`、`REGISTERED` 等
测试专用 DTO。`$FIXTURE`、`$RUN`、`$KC_HOME` 只是路径坐标；例如知识目录仍由
`kc ingest --dir "$FIXTURE/knowledge/semantic"` 直接消费。

确定性用例有四个：

1. `DW-CLI-01`：物理 Schema、真实 Adapter、首次采集、Connector diff、提交与幂等；
2. `DW-CLI-02`：语义 Aspect Schema 与 OKF 直接发布；
3. `DW-CLI-03`：两个 Repository 组成 Workspace，消费表、作业、语义、关系与来源；
4. `DW-CLI-04`：真实 DDL、按 key 重新拉取、精确 Address diff 和旧新 pin 复现。

Agent companion `DW-AGENT-01` 附着于 `DW-CLI-01` 和 `DW-CLI-03`，不是规范用例
或 CLI 的替代执行器。它不在 prompt 中预写命令。第一次接入方只得到自然语言
目标和隔离后的 fixture 坐标，Agent 自行选择 Skill、`kc` 工具及源侧工具；第一次
消费方只提出业务问题。确定性结果由最终 KC 状态和回答断言负责；tool trace 记录
实际路径和可恢复试错，不用随机轨迹冒充 CLI 规范。

## 目录

```text
mysql/                     MySQL Compose、DDL 和最小数据
knowledge/physical/schemas/*.aspect.yaml  直接发布的物理 Aspect Schema
knowledge/semantic/schemas/*.aspect.yaml  直接发布的语义 Aspect Schema
knowledge/semantic/objects/*.okf          一文件一个 Address 的语义知识
connector/connector.yaml   唯一 Connector 的稳定 scope
connector/adapter.py       唯一 MySQL I/O 面：table/column/job metadata + query/execute
connector/collector.py     signal、FULL reconcile 与 checkpoint 编排
connector/mapping.py       MySQL 行到 provider domain snapshot 的翻译
connector/domain.py        source key、object_id 与物理 Address 单元构造
connector/preview/         调用仓库根 connector.Preview 的墙侧适配器
features/*.feature         唯一用例集：Gherkin Scenario
features/steps/            Behave 的命令、JSON oracle 与 Agent trace 实现
features/environment.py    二进制、独立 KC home、MySQL 和 kc serve 测试环境
run.sh                     确定性 CLI/Connector 验收
run-agent.sh               真实 DSH Agent 验收（单独运行）
runs/                      Behave JSON、JUnit、命令输出和 trace 等可重建证据（不提交）
```

## 运行

快速检查不启动 MySQL：

```bash
make test-data-warehouse-check
```

确定性验收要求 Docker。每个 Scenario 使用独立 KC home，并重建 MySQL Compose
项目 `kc-dw-acceptance`：

```bash
make test-data-warehouse
```

按 tag 单独执行一个独立用例：

```bash
.data/data-warehouse/run.sh DW-CLI-03
```

真实 Agent 验收需要已安装的 `dsh`。`run-agent.sh` 会先运行完整 `run.sh`；只有
全部 CLI 用例通过，才会加载凭证、构建 DSH profile 并启动模型，而且 Agent 阶段
复用刚通过验收的同一组二进制。入口会从用户 `.env` 加载构建插件需要的
`NPM_TOKEN`，并优先使用 DSH credentials store 中已经登记的模型 credential ref；
key 不会写入镜像、patch、Behave 报告或 trace。也可以用 `DSH_MODEL_PATCH` 显式选择
模型配置。入口随后复用 `dsh-plugin/scripts/agent-env.sh` 准备隔离 profile：

```bash
make test-data-warehouse-agent
```

标准结果在 `runs/current/behave.json`、`runs/current/junit/`；每条命令的原文、
stdout 和 stderr 位于对应 Scenario 目录。Agent 的回答与 trace 位于
`runs/current-agent/agent/`，它所依赖的前置 CLI 证据位于
`runs/current-agent/cli/`。`runs/` 是可重建证据，不是 Canonical；Schema 正式形态
仍由 Writer 写入知识 Repository。

## 知识文件不是私有 DSL

`*.aspect.yaml` 和 `*.okf` 都是可直接交给 `kc ingest --dir` 的知识单元：
frontmatter 声明 `object_id`、`aspect_name` / `member_key`、`kind` 和
`schema_ref`，分隔线后的 YAML 就是该 Address 的业务值。一个文件只对应
一个 Address。`ingest` 只做机械预览和 ChangeSet 编译，不执行数仓领域转换。

静态 Schema 与语义知识因此不再经过 `domain.py`。只有来自 live MySQL 的
物理观察需要 Connector 把 source key 映射为稳定 `object_id` 并生成 desired
knowledge；这个翻译属于接入方，而不是知识文件格式。

物理 Repository 保存的是带 SOURCE provenance 的可复现观察，不冒充 MySQL
权威。invalidation 只触发 Collector；Collector 必须经 Adapter 重新拉取当前态，
再对通知 key 的 Address scope 做 reconcile，并周期执行 FULL reconcile。Table、Column、DataJob 及其包含关系全部保存为
Snapshot 知识。当前 MVP 不采集 row count、profile、freshness、quality summary 或
usage summary；`query` / `execute` 只是 Adapter operation，不在这里包装成 Binding。

语义侧只保留 SemanticModel、Metric，以及 SemanticModel 下的 Dimension/Measure
member。只有可独立引用的 Metric 是 Entity；Dimension/Measure 不复制成独立对象。
