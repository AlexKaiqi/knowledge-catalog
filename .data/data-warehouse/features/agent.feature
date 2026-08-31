@agent @mysql
Feature: 数仓 CLI 规范用例的 DSH Agent 附加验收
  这不是数仓接入规范的执行器。run-agent.sh 只会在全部 DW-CLI 用例通过后运行它，
  并复用刚刚通过 CLI 验收的 kc 和 connector-preview 二进制。
  提供方从 Connector 操作说明发现同步步骤；消费方从自然语言业务名称开始，经
  SEARCH 发现 CandidateRef、Canonical READ 回读，并按任务明确要求调用公开
  ResourceDescriptor 操作。验收检查最终
  状态、回答和真实 tool trace。可恢复的参数试错作为质量证据记录，不替代确定性
  的最终状态断言，也不把随机 Agent 轨迹冒充 CLI 规范。

  @DW-AGENT-01 @companion-DW-CLI-01 @companion-DW-CLI-03
  Scenario: Agent 验证提供方同步计划并回答固定版本消费问题
    When a provider asks the DSH Agent to preview synchronization:
      """
      我接手了一个已通过确定性发布验收的 MySQL 数仓知识提供方。接入材料位于
      $FIXTURE，其中 mysql 是可用的数据源 fixture，knowledge 是待发布的
      schema 与语义知识，connector 是现成的 Adapter、Collector、manifest 和
      connector-preview；运行时二进制位置由环境变量 KC_BIN、CONNECTOR_PREVIEW、
      PYTHON、KC_MYSQL_CONTAINER 和 KC_MYSQL_AUTH 提供。

      只加载 Knowledge Catalog Skill，并只通过 bash：读取准确路径
      `$FIXTURE/connector/connector.yaml` 和 `$FIXTURE/connector/README.md`，然后原样执行 README
      的 “No-op synchronization preview” 段，不要猜文件名、KC 命令或 Collector 参数。
      宿主已提供 `$KC_DW_CHECKPOINT`，结果必须写到 README 指定的
      `$RUN/agent-provider.observation.json` 与 `$RUN/agent-provider.preview.json`。
      不得调用 writer、不得修改 KC/fixture、
      不得调用 search/relations、不得用文件模型工具或 todo。确认 observation 有 101 个 desired，
      且因为当前源与已发布状态一致，preview 必须是 empty=true、added/updated/removed 都为 0。
      最后用中文说明无需发布，并指出消费入口是 warehouse-agent，成员为 kr://dw/physical 与
      kr://dw/semantic。不要执行其他验证命令。
      """
    Then the Agent succeeds
    And the Agent answer contains:
      | one of                    |
      | warehouse-agent          |
      | kr://dw/physical         |
      | kr://dw/semantic         |
      | 无需发布;无需更新;empty  |
    And the Agent trace includes:
      | kind  | name              |
      | skill | knowledge-catalog |
      | tool  | bash              |
    And the Agent trace excludes retired KC model tools
    And the Agent trace quality is recorded
    And the Agent trace stays within the "provider" quality budget
    And JSON file "$RUN/agent-provider.preview.json" satisfies:
      | path              | matcher | expected |
      | empty             | equals  | true     |
      | summary.added     | equals  | 0        |
      | summary.updated   | equals  | 0        |
      | summary.removed   | equals  | 0        |

    When I run `kc catalog workspace resolve --catalog kr://dw/catalog --workspace warehouse-agent | tee "$RUN/agent-provider.pin.json"`
    Then stdout JSON satisfies:
      | path                           | matcher      | expected        |
      | workspaceId                    | equals       | warehouse-agent |
      | pinId                          | is non-empty |                  |
      | repositories.kr://dw/physical  | is non-empty |                  |
      | repositories.kr://dw/semantic  | is non-empty |                  |

    When I run `kc knowledge read --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/agent-provider.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068`
    Then stdout JSON satisfies:
      | path                          | matcher    | expected |
      | $                             | has length | 1        |
      | [0].value.properties.name     | equals     | lineitem |
      | [0].value.schema.columnCount  | equals     | 16       |

    When I run `kc knowledge read --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/agent-provider.pin.json" --object dw-semantic-sales-metric-7630439d2660b81de165d124`
    Then stdout JSON satisfies:
      | path                      | matcher    | expected                |
      | $                         | has length | 1                       |
      | [0].value.properties.name | equals     | Gross merchandise value |

    When I run `kc operations projection sync --repo kr://dw/physical --ref refs/heads/main`
    Then the command succeeds

    When I run `kc operations projection sync --repo kr://dw/semantic --ref refs/heads/main`
    Then the command succeeds

    When a first-time consumer asks the DSH Agent:
      """
      我第一次使用这个数仓知识工作区，只知道业务名称，不知道任何 object ID。不要扫描或读取
      fixture、测试、源码、二进制、帮助文本或文件 mount。请先通过当前自动绑定的固定 Workspace
      调 `kc knowledge search --query ...`，分别发现 lineitem、inspect_urgent_orders 和
      Gross merchandise value。SEARCH 命中只是 CandidateRef；必须从命中的
      `knowledge.knowledgeRef.object` 取得 ID，再用 `kc knowledge read --object ...` 回读正式内容，
      不得从 SEARCH 摘要直接作答。随后按需要调用 `kc knowledge provenance`。请查清楚：
      lineitem 表有多少列；inspect_urgent_orders 是什么作业、是否启用；Gross merchandise
      value 指标基于哪个语义模型和物理表；该物理表各 Aspect 声明引用了哪些 Aspect
      Schema（schema_ref），以及语义模型到物理表的关系和两边的来源是什么。不要写入任何内容。
      SQL ResourceDescriptor 的稳定 ID 是 `resource/mysql-tpch-sql`；先回读其声明，然后必须准确运行
      `kc resource access --object resource/mysql-tpch-sql --operation query --input
      '{"sql":"SELECT COUNT(*) FROM tpch.customer"}'`；不要添加其他 flag，也不要直接调用 runtime。
      该命令必须实际执行
      `SELECT COUNT(*) FROM tpch.customer`，告诉我实时查询得到的客户数；不要从 fixture 文件
      或表元数据推断。用中文给出结论，并说明知识结论固定在哪个 Workspace pin 上、SQL
      结果使用了哪个 runtime generation。
      """
    Then the Agent succeeds
    And the Agent answer contains:
      | one of                                      |
      | lineitem                                    |
      | 16;十六                                     |
      | inspect_urgent_orders                       |
      | 未启用;禁用;disabled;enabled: false;是否启用：**否** |
      | Gross merchandise value;GMV                |
      | schema/table.properties                     |
      | schema/table.schema                         |
      | pin;不可变 repository commit;固定版本          |
      | resource/mysql-tpch-sql;MySQL TPC-H read-only SQL |
      | mysql-tpch-fixture-v1                       |
      | 客户;customer                               |
    And the Agent trace includes:
      | kind  | name              |
      | skill | knowledge-catalog |
      | tool  | bash              |
    And the Agent shell trace contains:
      | text                         |
      | kc knowledge search          |
      | kc knowledge read            |
      | kc knowledge provenance      |
      | kc resource access           |
    And the Agent trace excludes retired KC model tools
    And the Agent trace quality is recorded
    And the Agent trace stays within the "consumer" quality budget
