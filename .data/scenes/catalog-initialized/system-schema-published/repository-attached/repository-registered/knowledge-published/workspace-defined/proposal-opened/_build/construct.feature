# proposal-opened：candidate 存在；main 不动；read --repo 仍旧值。

Feature: proposal-opened

  Scenario: construct
    When I run `kc governance proposal create --proposal-id PR-scene --repo kr://scene/knowledge --target refs/heads/main --candidate refs/heads/candidates/PR-scene --object note/hello --value '{"text":"proposed"}'`
    Then the output has:
      | proposalId      | PR-scene |
      | candidateCommit | nonempty |
    When I run `kc knowledge read --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | value.text | hi |
    When I run `kc knowledge read --repo kr://scene/knowledge --object note/hello --ref refs/heads/candidates/PR-scene`
    Then the output has:
      | value.text | proposed |
