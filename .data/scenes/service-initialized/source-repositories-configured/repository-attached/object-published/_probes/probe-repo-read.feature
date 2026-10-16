# 在 object-published 上：维护口 HEAD 已前进，--repo 精确读回正文。

Feature: probe repo read

  Scenario: read published object
    When I run `kc writer head --repo kr://scene/object`
    Then the output has:
      | repository | kr://scene/object |
      | commit     | nonempty |
    When I run `kc read urn:kc:object:note/hello`
    Then the output has:
      | objectId | note/hello |
      | repository          | kr://scene/object |
      | value.text          | hi |
    When I run `kc resolve --repo kr://scene/object --object note/hello`
    Then the output has:
      | status     | RESOLVED |
      | objectId   | note/hello |
      | repository | kr://scene/object |
      | address    | absent |
    When I run `kc log --repo kr://scene/object --object note/hello`
    Then the output has:
      | continuation | absent |
      | logs         | nonempty |
    When I run `kc provenance --repo kr://scene/object --object note/hello`
    Then the output has:
      | objectId   | note/hello |
      | repository | kr://scene/object |
    When I run `kc dataset define --dataset plain-notes --revision 1 --source kr://scene/object=refs/heads/main`
    Then the output has:
      | setId | plain-notes |
      | revision | 1 |
      | sources.0.repository | kr://scene/object |
      | sources.0.commit | nonempty |
    When I run `kc read urn:kc:object:note/hello --dataset plain-notes --revision 1`
    Then the output has:
      | objectId | note/hello |
      | value.text | hi |
      | commit | nonempty |
    When I run `kc relations --dataset plain-notes --revision 1 --object note/hello`
    Then the output has:
      | hits | [] |
      | exhausted | true |
      | generation | nonempty |
      | continuation | absent |
    When I run `kc operations projection sync --dataset plain-notes --revision 1`
    Then the output has:
      | dataset | plain-notes |
      | revision | 1 |
      | snapshot.mode | ready |
    When I run `kc relations --dataset plain-notes --revision 1 --object note/hello`
    Then the output has:
      | exhausted | true |
      | hits | [] |
      | generation | nonempty |
