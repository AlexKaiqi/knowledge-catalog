# 在 knowledge-set-defined 上：发现与 pin / check 是组合读，不解 object_id。

Feature: probe compose views

  Scenario: list check pin and access spec
    When I run `kc show`
    Then the output includes:
      | workspaces[].workspaceId | scene-set |
    When I run `kc workspace pin --workspace scene-set --out $home/scene-set.pin.json`
    Then the output has:
      | workspaceId | scene-set |
      | pinId       | nonempty |
      | out         | $home/scene-set.pin.json |
    When I run `kc workspace check --workspace scene-set`
    Then the output has:
      | workspaceId | scene-set |
      | outcome     | PASSED |
      | issues      | [] |
    When I run `kc operations access-spec describe --pin $home/scene-set.pin.json`
    Then the output has:
      | workspaceId | scene-set |
      | specs       | nonempty |
