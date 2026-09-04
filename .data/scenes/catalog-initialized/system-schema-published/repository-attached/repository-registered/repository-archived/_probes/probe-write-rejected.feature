# 在 repository-archived 上：不能再 COMMIT。

Feature: probe write rejected

  Scenario: archived repository rejects put
    When I run `kc writer put --command-id after-archive --repo kr://scene/knowledge --object note/after --value '{"text":"no"}'`
    Then error REPOSITORY_ARCHIVED
