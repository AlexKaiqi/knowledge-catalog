Feature: Dataset 重新发布后搜索与回读使用服务版本

  Scenario: Dataset 重新发布后搜索与回读使用服务版本
    When I run `kc search --dataset scene-set --query merchandise`
    Then 1 hit metric/gmv
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fobject:metric%2Fgmv#/definition --commit $last.hits.0.commit`
    Then the output has:
      | objectId | metric/gmv |
      | repository | kr://scene/object |
      | value.name          | Gross merchandise value |
    When I run `kc writer put urn:kc:object:kr%3A%2F%2Fscene%2Fobject:note%2Fafter-search --command-id after-search --value '{"text":"after-search"}'`
    Then the output has:
      | disposition         | APPLIED |
      | result.repositoryId | kr://scene/object |
      | result.newCommit    | nonempty |
    When I run `kc operations projection sync --repo kr://scene/object`
    Then the output has:
      | repository  | kr://scene/object |
      | basisCommit | nonempty |
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fobject:metric%2Fgmv#/definition --dataset scene-set`
    Then the output has:
      | objectId | metric/gmv |
      | value.name          | Gross merchandise value |
    When I run `kc relations --dataset scene-set --object kc://scene/object/metric/gmv`
    Then the output includes:
      | hits[].objectId | rel/defines/gmv |
      | hits[].repository | kr://scene/graph |
    When I run `kc dataset define --dataset scene-set --revision 2 --source kr://scene/object=refs/heads/main@metrics/metric/gmv@metrics/metric/gmv --source kr://scene/graph=refs/heads/main@relations/rel@relations/rel --source kr://scene/object=refs/heads/main@schemas/metric@_schemas --source kr://scene/graph=refs/heads/main@schemas/relations@_schemas`
    Then the output has:
      | setId | scene-set |
      | revision    | 2 |
      | sources.0.commit | nonempty |
    When I run `kc operations projection sync --repo kr://scene/object`
    Then the output has:
      | repository  | kr://scene/object |
      | basisCommit | nonempty |
    When I run `kc search --dataset scene-set --query merchandise`
    Then 1 hit metric/gmv
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fobject:metric%2Fgmv#/definition --dataset scene-set`
    Then the output has:
      | objectId | metric/gmv |
      | value.name          | Gross merchandise value |
