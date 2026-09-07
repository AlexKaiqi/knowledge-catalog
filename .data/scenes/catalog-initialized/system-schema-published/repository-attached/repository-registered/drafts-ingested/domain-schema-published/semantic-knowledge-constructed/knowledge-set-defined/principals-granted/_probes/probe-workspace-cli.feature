# 在 principals-granted 上：help consume 最短路径是发现 → pin --out → SEARCH/READ。不是 --repo 维护读。

Feature: probe workspace cli

  Scenario: consume discovers, pins, searches, then reads
    When I run `kc catalog list`
    Then the output includes:
      | catalogs[].id | kr://scene/catalog |
    When I run `kc catalog show`
    Then the output includes:
      | workspaces[].workspaceId | scene-set |
    When I run `kc knowledge schema list --repo kr://scene/knowledge`
    Then the output has:
      | repository | kr://scene/knowledge |
    When I run `kc knowledge search --as taihu:alice --workspace scene-set --query merchandise`
    Then 1 hit metric/gmv with body stripped
    When I run `kc workspace pin --workspace scene-set --out $home/scene-pin.json`
    Then the output has:
      | workspaceId | scene-set |
      | pinId       | nonempty |
      | out         | nonempty |
    When I run `kc knowledge search --as taihu:alice --workspace scene-set --pin $pinFile --query merchandise`
    Then 1 hit metric/gmv with body stripped
    When I run `kc knowledge read --as agent:copilot --workspace scene-set --pin $pinFile --object metric/gmv --aspect definition`
    Then the output includes:
      | [].knowledgeRef.object | metric/gmv |
      | [].value.name          | Gross merchandise value |
    When I run `kc knowledge resolve --as agent:copilot --workspace scene-set --pin $pinFile --object metric/gmv`
    Then the output includes:
      | [].status | RESOLVED |
