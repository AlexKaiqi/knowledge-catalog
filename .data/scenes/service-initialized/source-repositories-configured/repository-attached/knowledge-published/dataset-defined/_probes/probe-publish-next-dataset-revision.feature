Feature: publish another Dataset revision

  Scenario: next revision pins the current repository commit
    When I run `kc dataset define --dataset scene-notes --revision 2 --source kr://scene/knowledge=refs/heads/main`
    Then the output has:
      | setId | scene-notes |
      | revision    | 2 |
      | sources.0.repository | kr://scene/knowledge |
      | sources.0.commit | nonempty |
