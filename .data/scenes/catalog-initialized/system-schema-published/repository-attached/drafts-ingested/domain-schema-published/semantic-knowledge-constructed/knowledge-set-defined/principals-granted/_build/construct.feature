# principals-granted：三种人已按人配权；投影已 sync；local HTTP 已起。

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
    Given local HTTP server
    When HTTP GET /identity/v1/whoami as taihu:alice
    Then whoami is taihu:alice
