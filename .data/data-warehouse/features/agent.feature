@agent @mysql
Feature: 数仓 CLI 规范用例的 DSH Agent 附加验收
  这不是数仓接入规范的执行器。run-agent.sh 只会在全部 DW-CLI 用例通过后运行它，
  并复用刚刚通过 CLI 验收的 kc 和 connector-preview 二进制。
  这里不把 kc 命令写进 prompt。Agent 必须从自然语言目标、插件 Skill 和隔离后的
  fixture 材料中自行选择工具；验收检查最终状态、回答和真实 tool trace。可恢复的
  参数试错作为质量证据记录，不替代确定性的最终状态断言，也不把随机 Agent 轨迹
  冒充 CLI 规范。

  @DW-AGENT-01 @companion-DW-CLI-01 @companion-DW-CLI-03
  Scenario: Agent 自主完成首次数据接入并回答首次消费问题
    When a first-time provider asks the DSH Agent:
      """
      我第一次给 Knowledge Catalog 接入一个 MySQL 数仓。接入材料位于
      $FIXTURE，其中 mysql 是可用的数据源 fixture，knowledge 是待发布的
      schema 与语义知识，connector 是现成的 Adapter、Collector、manifest 和
      connector-preview；运行时二进制位置由环境变量 KC_BIN、CONNECTOR_PREVIEW、
      PYTHON、KC_MYSQL_CONTAINER 和 KC_MYSQL_AUTH 提供。

      请通过 DSH 的 Knowledge Catalog 插件完成接入：Catalog 使用 kr://dw/catalog，
      物理和语义 Repository 分别使用 kr://dw/physical、kr://dw/semantic，最终给消费方
      建立 warehouse-agent Workspace。物理知识必须来自对当前 MySQL 的实际采集与
      Connector diff，语义知识直接使用 fixture 中可入库的文件。KC 内的动作使用插件
      工具；源侧 Adapter、Collector 和 preview 可以使用宿主工具。不要修改 fixture，
      不要伪造采集结果。完成后用中文简要说明发布了什么以及消费入口。
      """
    Then the Agent succeeds
    And the Agent answer contains:
      | one of                    |
      | warehouse-agent          |
      | kr://dw/physical         |
      | kr://dw/semantic         |
    And the Agent trace includes:
      | kind  | name              |
      | skill | knowledge-catalog |
      | tool  | kc                |
      | tool  | bash              |
    And the Agent trace quality is recorded
    And the Agent trace stays within the "provider" quality budget

    When I run `kc resolve --home "$KC_HOME" --catalog kr://dw/catalog --workspace warehouse-agent | tee "$RUN/agent-provider.pin.json"`
    Then stdout JSON satisfies:
      | path                           | matcher      | expected        |
      | workspaceId                    | equals       | warehouse-agent |
      | pinId                          | is non-empty |                  |
      | repositories.kr://dw/physical  | is non-empty |                  |
      | repositories.kr://dw/semantic  | is non-empty |                  |

    When I run `kc read --home "$KC_HOME" --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/agent-provider.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068`
    Then stdout JSON satisfies:
      | path                          | matcher    | expected |
      | $                             | has length | 1        |
      | [0].value.properties.name     | equals     | lineitem |
      | [0].value.schema.columnCount  | equals     | 16       |

    When I run `kc read --home "$KC_HOME" --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/agent-provider.pin.json" --object dw-semantic-sales-metric-7630439d2660b81de165d124`
    Then stdout JSON satisfies:
      | path                      | matcher    | expected                |
      | $                         | has length | 1                       |
      | [0].value.properties.name | equals     | Gross merchandise value |

    When a first-time consumer asks the DSH Agent:
      """
      我第一次使用这个数仓知识工作区。请帮我从当前自动绑定的 Workspace 中查清楚：
      lineitem 表有多少列；inspect_urgent_orders 是什么作业、是否启用；Gross merchandise
      value 指标基于哪个语义模型和物理表；该物理表各 Aspect 声明引用了哪些 Aspect
      Schema（schema_ref），以及语义模型到物理表的关系和两边的来源是什么。请自行发现
      对象，不要让我提供 object_id，不要写入任何内容。用中文给出结论，并说明这些结论
      固定在哪个 Workspace pin 上。
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
      | warehouse-agent                             |
      | pin                                         |
    And the Agent trace includes:
      | kind  | name                 |
      | skill | knowledge-catalog    |
      | tool  | knowledge_list       |
      | tool  | knowledge_read       |
      | tool  | knowledge_relations  |
      | tool  | knowledge_provenance |
    And the Agent trace quality is recorded
    And the Agent trace stays within the "consumer" quality budget
