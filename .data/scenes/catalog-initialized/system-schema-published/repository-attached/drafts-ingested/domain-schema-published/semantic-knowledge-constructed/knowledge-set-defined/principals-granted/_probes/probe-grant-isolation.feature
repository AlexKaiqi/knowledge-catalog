# 在 principals-granted 上：授权按人、他入不继承。过程中还会给 etl / alice 补权。

Feature: probe grant isolation

  @P-23 @KC-AGENT-01
  Scenario: grants do not inherit across principals
    """
    taihu:alice 开始只有 consume+search；agent:copilot 另有 read；service:etl 无授权。
    alice 能定位但看不到公式。给 etl 补 search 后仍无正文。
    给 alice 补 read 后 alice 可见全文，etl 不继承。
    """

    """
    Agent as taihu:alice (search-only)
    你是 taihu:alice。知识集 scene-set 已 pin 语义仓。
    你能 SEARCH，不能 READ。请按业务名称找 GMV。你应该能定位到 metric/gmv，
    但看不到公式。不要用 measureKey 去搜。精确读取应被拒绝。
    不要改身份，也不要替别人授权。
    """

    When HTTP POST /knowledge/v1/search as taihu:alice:
      | workspace | scene-set |
      | query     | merchandise     |
    Then 1 hit metric/gmv with body stripped

    When HTTP POST /knowledge/v1/search as taihu:alice:
      | workspace | scene-set |
      | query     | unique-measure-token-zz9 |
    Then 0 hits

    When HTTP POST /knowledge/v1/search as taihu:alice:
      | workspace | scene-set |
      | equal     | unit=CNY        |
    Then 1 hit metric/gmv with body stripped

    When HTTP POST /knowledge/v1/objects:read as taihu:alice:
      | workspace | scene-set |
      | object    | metric/gmv      |
    Then error FORBIDDEN

    When HTTP POST /knowledge/v1/search as agent:copilot:
      | workspace | scene-set |
      | query     | merchandise     |
    Then 1 hit metric/gmv with full canonical

    When HTTP POST /knowledge/v1/objects:read as agent:copilot:
      | workspace | scene-set |
      | object    | metric/gmv      |
    Then READ body is full canonical

    When HTTP POST /knowledge/v1/search as service:etl:
      | workspace | scene-set |
      | query     | merchandise     |
    Then error FORBIDDEN

    When HTTP POST /knowledge/v1/objects:read as service:etl:
      | workspace | scene-set |
      | object    | metric/gmv      |
    Then error FORBIDDEN

    When I run `kc admin grant add --principal service:etl --action workspace.consume --catalog kr://scene/catalog --workspace scene-set`
    Then the output has:
      | principal | service:etl |
      | actions.0 | workspace.consume |
    When I run `kc admin grant add --principal service:etl --action knowledge.search --repo kr://scene/knowledge`
    Then the output has:
      | principal | service:etl |
      | actions.0 | knowledge.search |

    When HTTP POST /knowledge/v1/search as service:etl:
      | workspace | scene-set |
      | query     | merchandise     |
    Then 1 hit metric/gmv with body stripped

    When HTTP POST /knowledge/v1/objects:read as service:etl:
      | workspace | scene-set |
      | object    | metric/gmv      |
    Then error FORBIDDEN

    When I run `kc admin grant add --principal taihu:alice --action knowledge.read --repo kr://scene/knowledge`
    Then the output has:
      | principal | taihu:alice |
      | actions.0 | knowledge.read |

    When HTTP POST /knowledge/v1/search as taihu:alice:
      | workspace | scene-set |
      | query     | merchandise     |
    Then 1 hit metric/gmv with full canonical

    When HTTP POST /knowledge/v1/objects:read as taihu:alice:
      | workspace | scene-set |
      | object    | metric/gmv      |
    Then READ body is full canonical

    When HTTP POST /knowledge/v1/search as service:etl:
      | workspace | scene-set |
      | query     | merchandise     |
    Then 1 hit metric/gmv with body stripped
