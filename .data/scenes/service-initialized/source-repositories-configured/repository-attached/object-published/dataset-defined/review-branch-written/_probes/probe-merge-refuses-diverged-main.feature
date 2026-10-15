# 审阅后 main 被另一写入推进；合并只快进，因此拒绝且不改写已发布值。

Feature: 主分支在审阅后被推进时合并以 NON_FAST_FORWARD 拒绝且主分支保持推进后的值

  Scenario: 主分支在审阅后被推进时合并以 NON_FAST_FORWARD 拒绝且主分支保持推进后的值
    When I run `kc writer put urn:kc:object:note/hello --repo kr://scene/object --command-id scene-drift --value '{"text":"drift"}'`
    Then the output has:
      | disposition      | APPLIED |
      | result.targetRef | refs/heads/main |
    When I run `kc writer merge --command-id scene-merge-diverged --repo kr://scene/object --from review/scene`
    Then error NON_FAST_FORWARD
    When I run `kc read urn:kc:object:note/hello`
    Then the output has:
      | value.text | drift |
