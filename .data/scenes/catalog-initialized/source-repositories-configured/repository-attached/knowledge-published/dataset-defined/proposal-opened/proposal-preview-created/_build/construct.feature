# Preview consumes the Dataset already published by its ancestor.
Feature: proposal-preview-created

  Scenario: construct
    When I run `kc governance preview create --proposal PR-scene --dataset scene-notes`
    Then the output has:
      | previewId           | nonempty |
      | candidate.commitId  | nonempty |
      | candidate.repositoryId | kr://scene/knowledge |
    When I run `kc read --repo kr://scene/knowledge --object note/hello`
    Then the output has:
      | value.text | hi |
