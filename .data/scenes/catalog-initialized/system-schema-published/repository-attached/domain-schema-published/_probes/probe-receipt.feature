# 在 domain-schema-published 上：按 command-id 查那次写的回执。不是知识 READ。

Feature: probe receipt

  Scenario: receipt of the schema commit
    When I run `kc writer receipt --command-id publish-domain-schema`
    Then the output has:
      | commandId | publish-domain-schema |
      | digest    | nonempty |
