# Snapshot 与 Object 零知识试跑

本轮只验证试跑形式以及 Snapshot、Object 的自主理解和使用。索引、LiveAccess、MySQL 的旧规格与
`world.sh` 保留为后续材料，不在本轮运行。

## 准备

```bash
trial=.data/scenes/service-initialized/named-repositories-created/qinghe-object-published/_materials/zero-knowledge/trial
bash "$trial/build.sh"
bash "$trial/snapshot-world.sh" up
bash "$trial/snapshot-world.sh" setup
```

构建从当前源码生成 `kc` 和部署夹具工具，放入独立镜像。Agent 固定为 pi 1.2.0；只安装在镜像中。
世界是独立的真实 lakeFS、PostgreSQL、MinIO 和 KC，不发布宿主端口，不启动索引或外部取值服务。
初始化系统仓后只建两个空业务仓并授予接入方明确的仓维护、交付发布与窄范围发权权限。

这是无业务示例的最小世界，不代表包含公开示例的完整使用环境。验证“从公开例子学会发布”时，操作员
须先按 [已有示例世界](../../examples/README.md) 发布三个 Dataset 并采用其部署读取声明，Agent 仍只通过
KC 发现和读取。示例源仓保持私有。启动 setup 前用 `KC_ZK_TABLE_REPOSITORY` 与
`KC_ZK_SEMANTIC_REPOSITORY` 指定独立的空供给仓（例如 `supplier-table-meta`、`supplier-sales-semantic`），
避免与示例源仓重名；默认值仍为 `table-meta`、`sales-semantic`。公开示例可以教格式和用法，当前业务事实
仍以本次原始资料为准，不能用示例中的旧口径覆盖它。每次记录必须说明是否提供了公开示例。

## 运行

```bash
bash "$trial/run.sh" env
bash "$trial/run.sh" supplier
bash "$trial/run.sh" colleague
bash "$trial/run.sh" revision
bash "$trial/run.sh" history
```

每条默认启动新会话。接入方只收到现有的原始口径说明；修订任务收到 v2。消费者没有本地业务资料，
对象组织、文件路径和对象身份都由 Agent 从 KC 学习。此顺序是一次供给、消费与修订旅程，各次调用不复用
聊天历史；消费使用上一角色真实交付的后态。

接入方或修订任务可用 `bash "$trial/run.sh" supplier --materials /path/to/raw-materials`
改用一组原始资料。运行器完整复制该目录到本轮 `input/`，保留相对路径与空目录；Agent 仍只从
`/work/materials/` 读取。先单独准备只含预期业务资料的目录，不直接传用户目录或仓库。
不存在的目录、符号链接和特殊文件会被拒绝，其它角色不能使用此参数。默认仍使用上述单份口径说明。
`run.json` 只记录材料的相对文件路径与 SHA256，不写文件正文；实际输入保存在本轮 `input/`。

密钥依次从 `ZK_MODEL_API_KEY`、`VOLCENGINE_API_KEY`、`ARK_API_KEY` 或用户目录 `.env` 的
`VOLCENGINE_API_KEY` 读取，不执行 `.env` 中的代码。默认端点和模型沿用已确认的试跑配置，可用
`ZK_MODEL_BASE_URL`、`ZK_MODEL_ID` 显式覆盖。真实密钥由主机运行器和网关持有，不进入 Agent 容器；Agent 只有占位认证值。

Agent 容器没有源码、文档、答案、宿主用户目录或 Docker socket；工作区、用户配置每轮重新创建。
它只能连接隔离网关，网关固定请求身份并仅代理 KC 和指定模型端点。运行前实际尝试伪报 admin，
确认服务观察到的仍是本轮身份。pi 的扩展、MCP、上下文文件和技能发现均关闭。
试跑镜像中的 `kc` 对 Agent 仅可执行、不可读；预检同时验证这一点，避免把提取二进制内嵌材料当成公开入口。

如果 Agent 提问，操作员按[替身规则](../README.md#人的替身)回答；使用同一角色的
`--resume <上次结果目录> --message '<原话>'` 继续；它只恢复会话，不恢复上轮工作区，旧工作产物仍保留在
上次结果目录的 `work/`。原问题与回答都保留在相应记录里。需要的业务信息不能由操作员暗中代找或补写。

## 证据与判定

结果写入仓库外 `~/kc-zk-runs/`，可用 `KC_ZK_RUNS` 覆盖。每轮保留原话、镜像 ID、身份隔离检查、
进程状态、原始事件流 `transcript.jsonl`、会话、工作产物 `work/`、最终回答和评审记录。运行成功只代表进程结束，
`run.json` 的 `review` 保持 `unassessed`，由操作员检查实际交付、对象内容和版本依据后填写评审。

- 环境认识：概念关系、能力、前提与用法有公开帮助或系统仓依据。
- 发布与消费：对象符合自选 Schema，经 Writer 发布；交付范围明确；同事仅凭自己的权限读到正确口径。
- 修订与历史：原对象身份延续，当前交付采用新规则，旧交付仍能读回旧规则并核对固定提交。
- 权限与信息：不借用其它身份、不读答案，不把推断或失败冒充已确认的事实。

独立验收通过正式 operator CLI 回读 Dataset 修订、文件和对象，不能以 Agent 自称完成为准。
正确受阻、产品缺口、Agent 错误和环境错误分别记录；一次试跑不作稳定性结论。

## 检查与清理

```bash
python3 -m unittest discover -s "$trial" -p 'test_*.py'
bash "$trial/snapshot-world.sh" check
bash "$trial/snapshot-world.sh" down
```

`down` 只清理本轮独立项目。已有 local、e2e 与开发栈不属于它的资源。先保存状态核对和评审记录，
再清理本轮世界。
