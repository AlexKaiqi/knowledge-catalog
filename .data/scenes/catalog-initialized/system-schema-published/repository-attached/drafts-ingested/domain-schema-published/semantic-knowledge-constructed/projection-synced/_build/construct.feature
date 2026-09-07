# projection-synced：运营投影已追上该仓 published HEAD。

Feature: projection-synced

  Scenario: construct
    When I run `kc operations projection sync --repo kr://scene/knowledge`
    Then the output has:
      | repository  | kr://scene/knowledge |
      | basisCommit | nonempty |
      | objectCount | nonempty |
    When I run `kc operations projection describe --repo kr://scene/knowledge`
    Then the output has:
      | basisRepository | kr://scene/knowledge |
      | basisCommit     | nonempty |
      | objectCount     | nonempty |
      | lagBehindHead   | false |
