# dataset-query-principals-granted：三种人已按人配权；声明的索引已按接受的修订准备。HTTP 扮演身份是停在本状态上的探。

Feature: dataset-query-principals-granted

  Scenario: construct
    When I run `kc grant add --principal taihu:alice --action file.read --dataset scene-set`
    Then the output has:
      | principal | taihu:alice |
      | dataset | scene-set |
      | actions.0 | file.read |
    When I run `kc grant add --principal agent:copilot --action file.read --dataset scene-set`
    Then the output has:
      | principal | agent:copilot |
      | actions.0 | file.read |
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
    When I run `kc grant add --principal agent:copilot --action dataset.resolve --dataset scene-set`
    Then the output has:
      | principal | agent:copilot |
      | actions.0 | dataset.resolve |
    When I run `kc grant list`
    Then the output includes:
      | rules[].principal | taihu:alice |
      | rules[].principal | agent:copilot |
      | rules[].actions.0 | object.read |
    When I run `kc index prepare --dataset scene-set --revision 1`
    Then the output has:
      | dataset | scene-set |
      | revision | 1 |
      | ready | true |
      | members.0.repository | kr://scene/graph |
      | members.1.repository | kr://scene/object |
      | members.0.commit | nonempty |
      | members.1.commit | nonempty |
    When I run `kc index describe --dataset scene-set --revision 1`
    Then the output has:
      | dataset | scene-set |
      | revision | 1 |
      | members.0.repository | kr://scene/graph |
      | members.0.stage | ready |
      | basis.mode | rebuild |
      | members.1.repository | kr://scene/object |
    When I run `kc index --dataset scene-set --revision 1`
    Then the output has:
      | dataset | scene-set |
      | revision | 1 |
      | ready | true |
      | objectCount | nonempty |
    When I run `kc search --dataset scene-set --revision 1 --query defines`
    Then 1 hit rel/defines/gmv
    When I run `kc search --dataset scene-set --revision 1 --query merchandise`
    Then 1 hit metric/gmv
