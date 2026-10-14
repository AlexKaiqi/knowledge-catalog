# Preview consumes the Dataset already published by its ancestor.
Feature: proposal-preview-created

  Scenario: construct
    When I run `kc governance preview create --proposal PR-scene --dataset scene-notes`
    Then the output has:
      | previewId           | nonempty |
      | candidate.commitId  | nonempty |
      | candidate.repositoryId | kr://scene/object |
    When I run `kc read urn:kc:object:kr%3A%2F%2Fscene%2Fobject:note%2Fhello`
    Then the output has:
      | value.text | hi |
