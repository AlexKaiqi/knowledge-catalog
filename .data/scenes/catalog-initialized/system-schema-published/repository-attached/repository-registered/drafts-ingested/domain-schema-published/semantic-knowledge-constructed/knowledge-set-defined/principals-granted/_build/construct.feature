# principals-granted：三种人已按人配权；声明式投影已追上 HEAD。HTTP 扮演身份是停在本状态上的探。

Feature: principals-granted

  Scenario: construct
    When I run `kc admin grant add --principal taihu:alice --action workspace.consume --catalog kr://scene/catalog --workspace scene-set`
    Then the output has:
      | principal | taihu:alice |
      | workspace | scene-set |
      | actions.0 | workspace.consume |
    When I run `kc admin grant add --principal taihu:alice --action knowledge.search --repo kr://scene/knowledge`
    Then the output has:
      | principal | taihu:alice |
      | actions.0 | knowledge.search |
    When I run `kc admin grant add --principal agent:copilot --action workspace.consume --catalog kr://scene/catalog --workspace scene-set`
    Then the output has:
      | principal | agent:copilot |
      | actions.0 | workspace.consume |
    When I run `kc admin grant add --principal agent:copilot --action knowledge.search --repo kr://scene/knowledge`
    Then the output has:
      | principal | agent:copilot |
      | actions.0 | knowledge.search |
    When I run `kc admin grant add --principal agent:copilot --action knowledge.read --repo kr://scene/knowledge`
    Then the output has:
      | principal | agent:copilot |
      | actions.0 | knowledge.read |
    When I run `kc admin grant list`
    Then the output includes:
      | rules[].principal | taihu:alice |
      | rules[].principal | agent:copilot |
      | rules[].actions.0 | knowledge.read |
    When I run `kc operations projection sync --repo kr://scene/knowledge`
    Then the output has:
      | repository  | kr://scene/knowledge |
      | basisCommit | nonempty |
      | objectCount | nonempty |
    When I run `kc operations projection describe --repo kr://scene/knowledge`
    Then the output has:
      | basisRepository | kr://scene/knowledge |
      | lagBehindHead   | false |
