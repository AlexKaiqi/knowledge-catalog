# 在 knowledge-set-defined 上：发现与 pin / check 是组合读，不解 object_id。

Feature: probe compose views

  Scenario: list check pin and access spec
    When I run `kc workspace list`
    Then the output includes:
      | workspaces[].workspaceId | scene-set |
    When I run `kc workspace pin --workspace scene-set`
    Then the output has:
      | workspaceId | scene-set |
      | pinId       | nonempty |
      | revision    | 1 |
    When I run `kc workspace check --workspace scene-set`
    Then the output has:
      | workspaceId | scene-set |
      | outcome     | PASSED |
      | issues      | [] |
    When I run `kc operations access-spec describe --workspace scene-set`
    Then the output has:
      | workspaceId | scene-set |
      | specs       | nonempty |
