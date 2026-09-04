# 在 principals-granted 上：consume 最短路径是 --workspace，可选冻结 --pin。不是 --repo 维护读。

Feature: probe workspace cli

  Scenario: workspace search then replay pin
    When I run `kc knowledge search --as taihu:alice --workspace scene-set --query merchandise`
    Then 1 hit metric/gmv with body stripped
    When I run `kc workspace pin --workspace scene-set`
    Then the output has:
      | workspaceId | scene-set |
      | pinId       | nonempty |
    When I run `kc knowledge search --as taihu:alice --workspace scene-set --pin $pinFile --query merchandise`
    Then 1 hit metric/gmv with body stripped
    When I run `kc knowledge read --as agent:copilot --workspace scene-set --pin $pinFile --object metric/gmv --aspect definition`
    Then the output includes:
      | [].knowledgeRef.object | metric/gmv |
      | [].value.name          | Gross merchandise value |
    When I run `kc knowledge resolve --as agent:copilot --workspace scene-set --pin $pinFile --object metric/gmv`
    Then the output includes:
      | [].status | RESOLVED |
