# MySQL + semantic knowledge acceptance

这里仅有一套数仓知识提供方验收：真实 MySQL 数据源经过 Collector 和
Connector FULL 对账进入物理知识仓，显式发布的语义知识引用这些物理对象，
消费者再通过固定 Workspace pin 读取和查询关系。数据库级只读 SQL 通过
ResourceDescriptor 声明并由独立 `resource-access/v1` runtime 执行；QueryLog 和
表级 State Binding 不在本轮夹具中。

```text
MySQL INFORMATION_SCHEMA
  -> connector/adapter.py
  -> connector/collector.py
  -> connector/connector.yaml + connector.Preview
  -> kc writer commit -> physical Repository
knowledge/**/*.aspect.yaml + knowledge/**/*.yaml
  -> kc writer ingest preview -> kc writer commit -> semantic Repository
physical + semantic -> Workspace pin -> read/search/relations/provenance
resource/mysql-tpch-sql -> resource-access/v1 -> connector/access.py -> adapter.query
```

## 用例规范

用例使用 Cucumber Gherkin，Behave 负责执行。一个 `Scenario` 是一个可以独立
运行的局部场景；每个 `When` 后必须立刻出现 `Then`。命令行直接写真实命令，
预期只观察退出码、stdout JSON、生成文件或最终 KC 状态。`check.sh` 会静态检查
这一约束。

这里没有 `action` DSL，也不把真实结果改写成 `catalogCount`、`REGISTERED` 等
测试专用 DTO。`$FIXTURE`、`$RUN`、`$KC_HOME` 只是路径坐标；例如知识目录仍由
`kc writer ingest --dir "$FIXTURE/knowledge/schemas/semantic"` 发布 Schema，
`kc writer ingest --dir "$FIXTURE/knowledge/semantic"` 发布实例。

确定性用例有五个：

1. `DW-CLI-01`：物理 Schema、真实 Adapter、首次采集、并发推进后的重算、提交幂等与冲突；
2. `DW-CLI-02`：语义 Aspect Schema 与实例 YAML 直接发布，以及无法解析 Schema 时拒绝写入；
3. `DW-CLI-03`：两个 Repository 组成 Workspace；消费方先发现 Catalog/Schema、再 resolve/check pin，读对象、来源与对象历史，经 `kc resource access` 执行只读 SQL，并验证缺失对象、权限和检索能力边界；
4. `DW-CLI-04`：真实 DDL、按 key 重新拉取、精确 Address diff 和旧新 pin 复现；
5. `DW-CLI-05`：数据源不可用、非法或过期 checkpoint 定向信号失败，修正后恢复采集。

全流程覆盖按责任边界组织，不在每个 Scenario 中重复所有初始化步骤：

| 阶段 | 正常路径 | 失败、恢复与不变量 |
| --- | --- | --- |
| 提供方定义 | 物理/语义 Schema 与实例 YAML 直接 ingest | 未解析 `schema_ref` 拒写，随后合法发布仍可成功 |
| 源系统接入 | Adapter 读取真实 MySQL，Collector FULL 观察 | 数据源不可用不输出伪观察；修正连接后可恢复 |
| 增量维护 | checkpoint + invalidation 只重拉目标表 | 非法 key、旧 checkpoint 拒绝；合法信号恢复；FULL 周期对账保持兜底 |
| 预览与发布 | Connector 生成 Address diff，Writer commit | target 被并发推进时 `NON_FAST_FORWARD`；基于新 head 重算后成功 |
| 命令重试 | 相同 `command_id` 与相同内容返回 `REPLAYED` | 相同 `command_id` 携带不同内容返回 `IDEMPOTENCY_CONFLICT` |
| 组合消费 | Catalog 发现、schema browse、named/临时 resolve、check、access describe；固定 pin 上 read/log/provenance | 不存在对象返回空结果；旧 pin 在新发布后仍可复现 |
| 授权 | Workspace 权限与每个成员仓读取权限同时满足后放行 | 未授权、仅有 Workspace 权限都 fail closed |
| 检索 | 查询使用同一 Workspace pin | 未配置 Retrieval provider 时明确返回 `CAPABILITY_UNSATISFIED`，不伪装成无结果 |
| 实时 SQL | `kc resource access` 按 pin 上的 ResourceDescriptor 调用独立 runtime | Descriptor 固定声明版本；endpoint、凭证和实时结果不写入知识仓 |

仓库根的 Go 测试负责协议代数、文件格式、导入分层和单组件边界；这里的五个
Scenario 负责从公开命令和真实数据源观察跨组件行为。两者共同覆盖，但不把内部
实现断言复制到黑盒用例中。

Agent companion `DW-AGENT-01` 附着于 `DW-CLI-01` 和 `DW-CLI-03`，不是规范用例
或 CLI 的替代执行器。第一次接入方从隔离 fixture 的 Connector 操作说明发现同步
步骤；第一次消费方只接收业务名称，先用 SEARCH 发现 CandidateRef，再回读 Canonical
内容并按任务要求调用公开 ResourceDescriptor 操作，不能读取 fixture 推断实时结果或绕过 `kc` 直连 runtime。
确定性结果由最终 KC 状态、外部 preview、回答和真实 tool trace 共同断言；随机
Agent 轨迹不冒充 CLI 规范。

## 目录

```text
mysql/                     MySQL Compose、DDL 和最小数据
knowledge/schemas/physical/               直接发布的物理 Aspect Schema
knowledge/schemas/semantic/               直接发布的语义 Aspect Schema
knowledge/physical/resources/             物理仓实例（按类型）
knowledge/semantic/metrics|semantic-models|relations/  语义仓实例（按类型）
connector/connector.yaml   唯一 Connector 的稳定 scope
connector/adapter.py       唯一 MySQL I/O 面：table/column/job metadata + query/execute
connector/collector.py     signal、FULL reconcile 与 checkpoint 编排
connector/access.py        `resource-access/v1` SQL runtime provider；只开放只读 query
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

### 一条命令启动完整环境

macOS 上的完整开发拓扑统一由 Compose 管理。默认包含 MySQL、Gitea、
OpenSearch、resource-access runtime、KC Server，以及一个 Docker 构建的
HTTP bash Client（ttyd + `kc`）。带 Linux `/dev/fuse` 的 DSH Client 是可选
`dsh` profile。首次启动会幂等创建一个混合 Workspace：物理仓使用 Dolt，语义仓
使用 Gitea；后续启动复用同一组 Compose volumes 并重新执行 Client smoke。

```bash
make dw-env-up       # 构建、启动、bootstrap，并验证 HTTP CLI
make dw-env-status
make dw-env-down     # 停服务，保留数据
make dw-env-reset    # 删除 kc-dw-e2e 的容器和 volumes，重新从空环境开始
.data/data-warehouse/dev.sh dsh-up   # 可选：启动 DSH Web（需要 OPENAI_*）
```

需要验证可观测链路时使用可选的 `observability` profile。它不只启动界面，
还会向真实 KC Server 发出完整 SEARCH，并断言 Prometheus target、原始
Histogram、recording rules、OTLP Collector→Jaeger trace、Collector→Loki log 和 Grafana dashboard 全部可用：

```bash
make dw-obs-up       # 启动并执行可观测 smoke
make dw-obs-smoke    # 对已启动环境再验证一次
make dw-obs-down     # 停止服务；Prometheus volume 保留，trace/log 为本地易失数据
```

通过后可直接打开：

- Grafana 系统总览：`http://127.0.0.1:7300/d/kc-overview/knowledge-catalog-system-overview`
- Grafana SEARCH 分析：`http://127.0.0.1:7300/d/kc-search-analysis/knowledge-catalog-search-analysis`
- Grafana 运行时健康：`http://127.0.0.1:7300/d/kc-runtime-health/knowledge-catalog-runtime-health`
- Grafana 诊断日志：`http://127.0.0.1:7300/d/kc-logs/knowledge-catalog-diagnostic-logs`
- Prometheus：`http://127.0.0.1:9090`
- Jaeger：`http://127.0.0.1:16686`
- Loki API：`http://127.0.0.1:3100`
- KC 原始指标：`http://127.0.0.1:7380/metrics`

数据源、四个 dashboard JSON、PromQL/LogQL 与验收逻辑均维护在
[`observability/`](observability/)；不会依赖运行后在 Grafana UI 中手工保存。

这里区分定义与数据：Git 只维护配置、查询、面板和 smoke 定义；Prometheus 样本、
Jaeger spans、Loki entries 都是可丢弃的运行数据。Jaeger 的 System Architecture 是从
已采集 span 推导的服务依赖图，不是静态项目架构图。完整边界和不足清单见
[`observability/README.md`](observability/README.md)。

这是本地/验收拓扑，Grafana 匿名只读、Jaeger/Loki 易失存储和本机端口都不是生产
部署模板。Prometheus 3 scrape 显式使用 legacy/下划线命名，避免 protobuf
协商后保留 OTel 点号 instrument 名而与已发布 recording rules 不一致。

启动完成后打开 HTTP CLI `http://127.0.0.1:7681`。容器里已安装 `kc` Client，
默认连 `http://kc-server:7380`。不要预置 `KC_AS`，也不要在这里跑 `kc local` /
`kc serve`。这个 Compose Server 是 `--auth local` 测试捷径，不是 Taihu 产品配对。用
`kc login --mode local --as <principal>` 把主体写进客户端凭证库；后续请求只发
`X-Kc-As`。

消费方先发现再读取：

```bash
kc login --mode local --as agent:dsh
kc catalog list
kc catalog show
kc knowledge schema browse --repo kr://dw/physical
kc catalog workspace resolve > pin.json
kc knowledge search --query lineitem
kc knowledge relations --object kc://dw/physical/dw-mysql-tpch-table-c02fedc564bba85c8d5d1068
kcfs plan --server "$KC_SERVER_URL" --as agent:dsh --workspace warehouse-agent \
  --view semantic --root /workspace
```

接入与治理共用 `service:bootstrap`：`kc login --mode local --as service:bootstrap`
后可用 `writer ingest/head`、`catalog show`、`catalog audit`、`admin grant list`、
`operations projection describe`、`operations access describe`。`kc help consumer|provider|governor` 是角色最短闭环。
KC Server 位于 `http://127.0.0.1:7380`，Gitea 位于 `http://127.0.0.1:3000`。端口可分别通过
`KC_DW_CLI_PORT`、`KC_DW_SERVER_PORT`、`KC_DW_GITEA_PORT` 等环境变量覆盖。
可选 Basic Auth 用 `KC_DW_CLI_CREDENTIAL=user:pass`。生成的 bootstrap 和 Client
工作目录位于 `runs/compose/`；它们不是手工启动输入，也不是 Canonical。

DSH 仍是可选 Web Agent 入口：`./.data/data-warehouse/dev.sh dsh-up` 后访问
`http://127.0.0.1:7400`。GPT 路由只在 DSH 进程启动时从 `${HOME}/.env` 读取
`OPENAI_BASE_URL` 与 `OPENAI_API_KEY`。

DSH Client 容器每次启动都在容器内的 `/run/dsh-home` 从空目录创建 profile；不复用
上一次容器的 profile、pnpm store、Session 或 settings。DSH、pnpm 和
`dsh-multi-model-provider` 版本固定，profile 的完整依赖锁摘要、bundle 顺序、当前源码
构建出的 `dsh-loom`、Knowledge Catalog Skill、最终 Luna/Sol 路由、默认模型以及
secret-free 配置都会在 Web Server 启动前校验。当前固定的 multi-model rc.19 浏览器
入口会被确定性适配到 DSH rc.2 的 `connection.api` settings wire；适配目标或依赖图
发生漂移时容器失败关闭，不能带着旧 profile 继续运行。
registry 解析只发生在镜像构建阶段；镜像先生成并校验只读 seed lock/store，容器启动
再从它执行 `pnpm --offline --frozen-lockfile`。因此启动不访问 registry，也不会从宿主机
或前一次容器借用 package cache。

健康检查也不再只接受首页 HTTP 200：容器内 Chromium 必须实际完成全部 Browser
插件激活并渲染会话 shell，multi-model catalog 必须同时观察到 live 的
`lore-openai/gpt-5.6-luna` 与 `lore-openai/gpt-5.6-sol`。随后才运行
SEARCH、Canonical READ、Resource Access 和 Linux FUSE smoke。

这里的边界是明确的：`kc` Client/SDK 和 DSH 本身并非只支持 Linux；只有执行
真实只读挂载的 `kcfs mount` 依赖 Linux `/dev/fuse`，因此 macOS 上把 DSH Client
连同 `kcfs` 放进 Linux 容器。一个 Catalog Workspace 可以组合不同 Repository
authority，本环境正是 Dolt + Gitea；每个 Repository 仍只选择一个 Snapshot Store，
这不是把同一个 Repository 同时镜像到多个 Store。Dolt 的知识 Canonical 使用原生
`kc_units` 行，DSH 通过 KC API 读取；`kcfs` 的字面文件投影由 Gitea-backed
Repository 验证。OpenSearch 只是可重建检索投影，不是第三个 authority。

快速检查不启动 MySQL：

```bash
make test-data-warehouse-check
```

确定性验收要求 Docker。每个 Scenario 使用独立 KC home，并重建 MySQL Compose
项目 `kc-dw-acceptance`。入口先执行静态 surface/spec 检查，再为没有本机 Dolt 的环境
启动一个 run-scoped 常驻 Dolt 容器；后续 CLI 通过 `docker exec` 复用它，不会为每个
读写命令重复冷启动容器。终端会打印当前 Scenario、公开 `When` 操作和耗时：

```bash
make test-data-warehouse
```

按 tag 单独执行一个独立用例：

```bash
.data/data-warehouse/run.sh DW-CLI-03
```

真实 Agent 验收需要已安装的 `dsh`。入口会在确定性门禁前校验 DSH、npm 和仓库声明的
Node 24；若机器已安装匹配 `.node-version` 的 Homebrew/NVM runtime，会只为本次进程
选择它，不修改全局 Node。`run-agent.sh` 随后运行完整 `run.sh`；只有
全部 CLI 用例通过，才会加载凭证、在本次证据目录构建临时 DSH home/profile 并启动模型，而且 Agent 阶段
复用刚通过验收的同一组二进制。入口会从用户 `.env` 加载构建插件需要的
`NPM_TOKEN`，并优先使用 DSH credentials store 中已经登记的模型 credential ref；
key 不会复制进证据目录、镜像、Behave 报告或 trace。临时 profile 不安装到
`~/.dsh/profiles`；它和 session 都属于本次 run。也可以用 `DSH_MODEL_PATCH` 显式选择
模型配置。入口随后启动临时 OpenSearch、构建固定 commit 投影，并复用
`dsh-plugin/scripts/agent-env.sh` 准备临时 profile。消费 Agent 从自然语言名称开始执行
SEARCH→CandidateRef→Canonical READ，不再由宿主预塞 object ID：

```bash
make test-data-warehouse-agent
```

标准结果在 `runs/current/behave.json`、`runs/current/junit/`；每条命令的原文、
stdout 和 stderr 位于对应 Scenario 目录。Agent 的回答与 trace 位于
`runs/current-agent/agent/`，它所依赖的前置 CLI 证据位于
`runs/current-agent/cli/`。`runs/` 是可重建证据，不是 Canonical；Schema 正式形态
仍由 Writer 写入知识 Repository。

## 知识文件不是私有 DSL

`*.aspect.yaml` 和实例 `*.yaml` 都是可直接交给 `kc writer ingest --dir` 的知识单元：
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
usage summary。`resource/mysql-tpch-sql` 把只读 `query(sql)` 声明成消费能力，实际
调用由墙外 runtime 完成；无约束的 `execute` 仍只是提供方 Adapter operation，不向
消费 Agent 暴露，也不包装成 Binding。

语义侧只保留 SemanticModel、Metric，以及 SemanticModel 下的 Dimension/Measure
member。只有可独立引用的 Metric 是 Entity；Dimension/Measure 不复制成独立对象。
