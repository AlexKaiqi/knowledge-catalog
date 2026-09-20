Feature: Dataset 授权按人隔离且不扩成仓级读权

  @P-23 @KC-AGENT-01
  Scenario: Dataset 授权按人隔离且不扩成仓级读权
    """
    taihu:alice 持有 Dataset file.read；agent:copilot 另有仓 knowledge.read；service:etl 无授权。
    alice 能读清单内文件，不能 --repo 读。etl 无授权则失败。
    给 etl 补 Dataset file.read 后 etl 可见清单内正文，alice 的仓 knowledge.read 不传给 etl。
    """

    Given local HTTP server
    When HTTP POST /knowledge/v1/search as taihu:alice:
      | dataset | scene-set |
      | query     | merchandise     |
    Then 1 hit metric/gmv with full canonical

    When HTTP POST /knowledge/v1/objects:read as taihu:alice:
      | dataset | scene-set |
      | object    | metric/gmv      |
    Then READ body is full canonical

    When I run `kc read --as taihu:alice --repo kr://scene/knowledge --object metric/gmv`
    Then error FORBIDDEN

    When HTTP POST /knowledge/v1/search as agent:copilot:
      | dataset | scene-set |
      | query     | merchandise     |
    Then 1 hit metric/gmv with full canonical

    When HTTP POST /knowledge/v1/objects:read as agent:copilot:
      | dataset | scene-set |
      | object    | metric/gmv      |
    Then READ body is full canonical

    When HTTP POST /knowledge/v1/search as service:etl:
      | dataset | scene-set |
      | query     | merchandise     |
    Then error FORBIDDEN

    When HTTP POST /knowledge/v1/objects:read as service:etl:
      | dataset | scene-set |
      | object    | metric/gmv      |
    Then error FORBIDDEN

    When I run `kc grant add --principal service:etl --action file.read --catalog kr://scene/catalog --dataset scene-set`
    Then the output has:
      | principal | service:etl |
      | actions.0 | file.read |

    When HTTP POST /knowledge/v1/search as service:etl:
      | dataset | scene-set |
      | query     | merchandise     |
    Then 1 hit metric/gmv with full canonical

    When HTTP POST /knowledge/v1/objects:read as service:etl:
      | dataset | scene-set |
      | object    | metric/gmv      |
    Then READ body is full canonical

    When I run `kc read --as service:etl --repo kr://scene/knowledge --object metric/gmv`
    Then error FORBIDDEN
