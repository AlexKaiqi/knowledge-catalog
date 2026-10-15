# 合并本节点冻结的工作分支并从 main 读回；不依赖另一个 probe。

Feature: 合并审阅过的分支后从主分支读回新值

  Scenario: 合并审阅过的分支后从主分支读回新值
    When I run `kc writer merge --command-id scene-merge --repo kr://scene/object --from review/scene`
    Then the output has:
      | disposition         | APPLIED |
      | surface             | MERGE |
      | result.repositoryId | kr://scene/object |
      | result.targetRef    | refs/heads/main |
      | result.newCommit    | nonempty |
    When I run `kc read urn:kc:object:note/hello`
    Then the output has:
      | value.text | proposed |
