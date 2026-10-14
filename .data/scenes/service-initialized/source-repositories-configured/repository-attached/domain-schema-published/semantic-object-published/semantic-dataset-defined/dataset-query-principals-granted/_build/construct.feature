# dataset-query-principals-granted：三种人已按人配权；声明式投影已追上 HEAD。HTTP 扮演身份是停在本状态上的探。

Feature: dataset-query-principals-granted

  Scenario: construct
    When I run `kc grant add --principal taihu:alice --action file.read --dataset scene-set`
    Then the output has:
      | principal | taihu:alice |
      | dataset | scene-set |
      | actions.0 | file.read |
    When I run `kc grant add --principal taihu:alice --action object.search --repo kr://scene/object`
    Then the output has:
      | principal | taihu:alice |
      | actions.0 | object.search |
    When I run `kc grant add --principal agent:copilot --action file.read --dataset scene-set`
    Then the output has:
      | principal | agent:copilot |
      | actions.0 | file.read |
    When I run `kc grant add --principal agent:copilot --action object.search --repo kr://scene/object`
    Then the output has:
      | principal | agent:copilot |
      | actions.0 | object.search |
    When I run `kc grant add --principal agent:copilot --action object.read --repo kr://scene/object`
    Then the output has:
      | principal | agent:copilot |
      | actions.0 | object.read |
    When I run `kc grant add --principal agent:copilot --action inventory.read`
    Then the output has:
      | principal | agent:copilot |
      | actions.0 | inventory.read |
    When I run `kc grant add --principal agent:copilot --action object.schema.read --repo kr://scene/object`
    Then the output has:
      | principal | agent:copilot |
      | actions.0 | object.schema.read |
    When I run `kc grant add --principal agent:copilot --action object.read --repo kr://scene/graph`
    Then the output has:
      | principal | agent:copilot |
      | actions.0 | object.read |
    When I run `kc grant add --principal agent:copilot --action object.search --repo kr://scene/graph`
    Then the output has:
      | principal | agent:copilot |
      | actions.0 | object.search |
    When I run `kc grant add --principal agent:copilot --action dataset.resolve --dataset scene-set`
    Then the output has:
      | principal | agent:copilot |
      | actions.0 | dataset.resolve |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | taihu:alice |
      | rules[].principal | agent:copilot |
      | rules[].actions.0 | object.read |
    When I run `kc operations projection sync --repo kr://scene/object`
    Then the output has:
      | repository  | kr://scene/object |
      | basisCommit | nonempty |
      | objectCount | nonempty |
    When I run `kc operations projection describe --repo kr://scene/object`
    Then the output has:
      | basisRepository | kr://scene/object |
      | lagBehindHead   | false |
    When I run `kc operations projection sync --repo kr://scene/graph`
    Then the output has:
      | repository  | kr://scene/graph |
      | basisCommit | nonempty |
      | objectCount | nonempty |
    When I run `kc operations projection describe --repo kr://scene/graph`
    Then the output has:
      | basisRepository | kr://scene/graph |
      | lagBehindHead   | false |
    When I run `kc search --repo kr://scene/graph --query defines`
    Then 1 hit rel/defines/gmv
    When I run `kc search --repo kr://scene/graph --query merchandise`
    Then 0 hits
