# DSH Loom 数仓操作链路

这份手册用于人工或 UI Agent 验收 `dsh-loom`。每一步都区分：发给 Agent 的自然语言、后台观察，以及可判定的预期结果。

## 目标拓扑

```text
Catalog  kr://acme/catalog
├── kr://acme/public/metadata       MySQL Connector 发布物理元数据
├── kr://acme/org/semantics         组织发布指标和语义知识
└── kr://acme/personals/kai         个人知识与工作草稿

Workspace analyst-board = metadata + semantics
Workspace kai-desk      = personal
```

身份当前由 DSH 进程的 `KC_AS` 固定，不是登录认证。建议各角色使用独立端口，共用同一 `KC_SERVE`；不要在同一聊天中让模型声称切换身份。

| 角色 | `KC_AS` | 建议端口 | 主要任务 |
|---|---|---:|---|
| Owner | 空 | 61300 | 初始化、挂仓、发权、定义 Workspace |
| Collector | `collector` | 61301 | 发布源系统同步结果 |
| Steward | `steward` | 61302 | 提议组织语义知识 |
| Reviewer | `reviewer` | 61303 | 验证、审批、合并 |
| Analyst | `analyst-agent` | 61304 | 消费公司知识 |
| Kai | `kai` | 61305 | 编辑个人知识 |
| Auditor | `auditor` | 61306 | 查看审计和来源 |
| Unauthorized | `intruder` | 61307 | 权限反例 |

推荐共用环境：

```bash
export KC_HOME=/tmp/dsh-loom-dw/.kc-home
export KC_SERVE=http://127.0.0.1:7390
export KC_CATALOG=kr://acme/catalog
```

Agent 应自动使用 preset 内置的 Knowledge Catalog Skill。操作者只描述目标，不发送“加载 skill”。

## 1. Owner 建公司 Catalog 和三仓

发给 Agent：

> 为 Acme 建一个公司级知识 Catalog。挂载平台元数据仓、组织语义仓和 Kai 的个人仓；再创建 analyst-board（元数据+语义）和 kai-desk（仅个人）两个 Workspace。先给我计划，确认对象和边界后执行，最后汇报固定标识。

后台观察：`kc read --catalog kr://acme/catalog`，并检查三条 Repository ID 和两个 Workspace 配方。

预期：Catalog 只有组合信息；Workspace 引用仓的已发布 selector，不复制知识，也不包含 `object_id`。

## 2. Owner 发最小权限

发给 Agent：

> 按最小权限配置角色：collector 只能向 metadata 发布；steward 只能向 semantics 提议；reviewer 可 preview、记录验证并 merge；analyst-agent 只能读 analyst-board；kai 只能写 personal 并读 kai-desk；auditor 只能审计和读 provenance。不要发对象级或表级 ACL。

后台观察：检查 `$KC_HOME/.kc/allow.json`；确认规则按仓、Workspace 和 `kc` 动词约束。

预期：没有把 MySQL GRANT 写入 Catalog ACL；Owner 不带 `KC_AS`，其他角色由进程身份求值。

## 3. 启动真实 MySQL fixture

这是操作者动作，不发给 DSH Agent：

```bash
cd .scenes/data-warehouse
KC_DW_KEEP_MYSQL=1 ./validation/playbook.sh DW-01
```

后台观察：MySQL 8.4.8 容器可用，`tpch` 有 8 张表；fixture 输出通过。

预期：源库只是外部权威，不是 Catalog 的 SnapshotStore，也不成为 Workspace 成员。

## 4. 让编程 Agent 生成 Connector

这一步发给工作在 scene 实体文件系统中的编程 Agent，不发给 Loom 虚拟文件系统 Agent：

> 在 `.scenes/data-warehouse/validation/connectors/mysql-structure/` 实现一个外部 MySQL Structure Connector。读取连接配置，发现 schema/table/column，使用稳定 source key 与 object_id 映射，附 SOURCE provenance；先输出 Address 级 ChangeSet 预览，再通过 `kc commit --changeset` 或 `/v1/commit` 发布。支持 dry-run、reconcile、checkpoint-after-commit、同 command_id 幂等、NON_FAST_FORWARD/CAS 重试和单元测试。Schema 必须作为 `schema/*` 知识对象写入，不得生成受 git 跟踪的 schema 源文件。不得给 `kc` 增加 connector-run。

后台观察：代码只出现在 scene 的 `validation/`；依赖通用 `connector/` 对账 kit，但源客户端、source key 和 checkpoint 都留在 scene。

预期：编译和测试通过；同一源对象每次映射到相同 Address；无凭证进入 git。

## 5. Collector 先做 dry-run

发给 Agent：

> 连接本次 TPC-H MySQL，生成物理结构同步预览，但先不要提交。汇总会新增、更新、删除多少个 Address，并列出 schema、table、column 的代表样例和来源。

后台观察：保存预览 ChangeSet；metadata 的 `refs/heads/main` 不变化；connector checkpoint 不变化。

预期：预览可解释且没有写副作用。当前 fixture 的 DW-01 oracle 应包含 69 个唯一 Address。

## 6. Collector 发布并验证幂等

发给 Agent：

> 发布刚才确认的结构 ChangeSet 到公共 metadata 仓。提交成功后再用同一 command_id 和同一内容重放一次，报告 commit、checkpoint 和幂等结果。

后台观察：第一次推动 metadata main；checkpoint 只在 commit 成功后推进；第二次重放不产生新 commit。

预期：知识可通过 `read`/`list`/`provenance` 回读；SOURCE envelope 指向 MySQL source key；重放返回同一结果。

## 7. Analyst 做发布前基线消费

发给 Agent：

> 在 analyst-board 中列出 TPC-H 表，读取 orders 表的结构和 provenance，搜索 revenue/GMV 相关知识，并告诉我当前是否已有正式 GMV 指标。

后台观察：一次命令开始时 `resolve --workspace analyst-board` 固定 metadata 与 semantics 的 commit。

预期：物理表可见；尚未发布的 GMV 指标不可见；Agent 不直接指定 Repository 或追随中途变化的 HEAD。

## 8. Steward 提议 GMV

发给 Agent：

> 基于 orders 和 lineitem，起草组织级 GMV 指标：说明业务定义、公式、粒度、时间口径、依赖表和 owner。以 Proposal `PR-gmv-v1` 提交到语义仓，不要直接发布到 main。

后台观察：candidate ref 出现；semantics main 不变化；Proposal 记录 base commit。

预期：`analyst-board` 普通读仍看不到 GMV；提案可以 Preview。

## 9. Reviewer 验证并合并

发给 Agent：

> 审查 `PR-gmv-v1`。先在 analyst-board 上 Preview，检查公式引用和 schema_ref；运行结构校验，并记录外部 `metrics-contract` 套件 PASSED。所有 gate 满足后合并；逐项汇报 previewId、validation report 和最终 commit。

后台观察：Preview 钉住 Workspace 和 proposal base；gate 只接受绑定同一 previewId 的证据；merge 前后对比 semantics main。

预期：证据不足时不能合并；合并成功只推动 semantics main，不改 Catalog 配方。

## 10. Analyst 新命令消费已发布知识

发给 Agent：

> 重新打开 analyst-board，搜索并读取正式 GMV 指标。给出它的定义、依赖、owner、来源，以及本次 Workspace 的固定仓 commit。

后台观察：新命令重新 ResolveWorkspace；回读 Canonical，而不是把索引摘要当权威。

预期：GMV 现在可见；metadata 和 semantics 各有固定 pin；旧会话中的命令内快照不被篡改。

## 11. Kai 编辑个人知识

发给 Agent：

> 在 kai-desk 记录我对 GMV 的个人分析习惯和待验证问题，随后修改其中一条。请展示修改前后 diff。不要把个人草稿发布到组织语义仓，也不要复制公共知识。

后台观察：只推动 personal main；可用 `log`/`diff` 查看对象变化；analyst-board 不出现个人对象。

预期：个人知识独立维护；Workspace 不做 public/group/personal 覆盖。

## 12. MySQL CDC 增量更新

发给 Collector Agent：

> 处理下一条 MySQL binlog 事件，只更新受影响的观测知识，并将原始事件 APPEND 到流。知识 commit 和 stream append 都成功后再推进 connector checkpoint；然后重放同一事件并报告结果。

后台观察：知识 Snapshot、Stream cursor、connector checkpoint 按顺序推进；同一事件重放不重复生效。

预期：旧 position 被拒绝；重复事件不产生额外知识 commit 或流记录。可用 `./validation/playbook.sh DW-03` 作 oracle。

## 13. Connector 全量 reconcile

发给 Collector Agent：

> 对 MySQL 当前态做一次全量 reconcile。先只给预览，明确新增、更新、删除；确认后发布。任何不属于本 connector source boundary 的对象都不得删除。

后台观察：REMOVE 只针对 connector 曾管理且源端已消失的 Address；过期 target commit 触发 `NON_FAST_FORWARD` 后重新拉当前态、重做 diff，并使用新 command_id。

预期：连续两次无源变化 reconcile 的第二次是空 ChangeSet。

## 14. Auditor 查证据链

发给 Agent：

> 审计 GMV 从提议、预览、验证到合并的过程，并追溯 orders 结构和最新观测的 provenance。区分 Catalog 审计、知识仓 git 历史和来源信封，不要把三者混为一谈。

后台观察：`kc audit` 查看 Catalog 登记变更；`kc log --workspace --object` 查看对象知识历史；`kc provenance` 查看单元来源。

预期：三类证据能分别回答“组合何时变”“知识何时发布”“知识来自哪里”。

## 15. Unauthorized 反例

发给 Agent：

> 尝试把一个伪造 GMV 定义直接写入组织语义仓，并报告系统拒绝结果。不要绕过 Writer，也不要修改权限。

后台观察：操作前后 semantics main 相同；错误信封是明确的 `FORBIDDEN`。

预期：Agent 不自称 Owner、不切换身份、不改 `allow.json`，失败没有副作用。

## 验收收口

```bash
cd .scenes/data-warehouse
./validation/playbook.sh WORKBENCH
./validation/playbook.sh DW-03
```

UI 验收还应保存每个角色会话的提示词、Agent 回复和关键 `kc` receipt。自动化 oracle 判协议状态，UI 会话判 Skill 自动加载、身份边界、解释质量和错误恢复。
