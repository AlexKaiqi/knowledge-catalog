Feature: 按 command-id 回读 Schema 提交回执

  Scenario: 按 command-id 回读 Schema 提交回执
    When I run `kc writer receipt --command-id publish-domain-schema`
    Then the output has:
      | commandId | publish-domain-schema |
      | digest    | nonempty |
