Feature: Dataset 重新发布后搜索与回读使用服务版本

  Scenario: Dataset 重新发布后搜索与回读使用服务版本
    When I run `kc search --dataset scene-set --query merchandise`
    Then 1 hit metric/gmv
    When I run `kc read --repo kr://scene/knowledge --commit $last.hits.0.commit --object metric/gmv --aspect definition`
    Then the output has:
      | objectId | metric/gmv |
      | repository | kr://scene/knowledge |
      | value.name          | Gross merchandise value |
    When I run `kc writer put --command-id after-search --repo kr://scene/knowledge --object note/after-search --value '{"text":"after-search"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/knowledge |
      | result.newCommit    | nonempty |
    When I run `kc operations projection sync --repo kr://scene/knowledge`
    Then the output has:
      | repository  | kr://scene/knowledge |
      | basisCommit | nonempty |
    When I run `kc read --dataset scene-set --object metric/gmv --aspect definition`
    Then the output includes:
      | [].objectId | metric/gmv |
      | [].value.name          | Gross merchandise value |
    When I run `kc relations --dataset scene-set --object kc://scene/knowledge/metric/gmv`
    Then the output includes:
      | hits[].objectId | rel/defines/gmv |
      | hits[].repository | kr://scene/graph |
    When I run `kc dataset define --dataset scene-set --revision 2 --source kr://scene/knowledge=refs/heads/main@metrics/metric/gmv@metrics/metric/gmv --source kr://scene/graph=refs/heads/main@relations/rel@relations/rel --source kr://scene/knowledge=refs/heads/main@schemas/metric@_schemas --source kr://scene/graph=refs/heads/main@schemas/relations@_schemas`
    Then the output has:
      | setId | scene-set |
      | revision    | 2 |
      | sources.0.commit | nonempty |
    When I run `kc operations projection sync --repo kr://scene/knowledge`
    Then the output has:
      | repository  | kr://scene/knowledge |
      | basisCommit | nonempty |
    When I run `kc search --dataset scene-set --query merchandise`
    Then 1 hit metric/gmv
    When I run `kc read --dataset scene-set --object metric/gmv --aspect definition`
    Then the output includes:
      | [].objectId | metric/gmv |
      | [].value.name          | Gross merchandise value |
