@cli @mysql
Feature: Collector 感知源变化后重新取当前值并保持旧 pin 可复现

  @DW-CLI-04
  Scenario: MySQL DDL 变化只改对应 Address，旧新 Workspace pin 各自稳定
    When I run `kc local init --home "$KC_HOME" --catalog kr://dw/catalog`
    Then stdout JSON satisfies:
      | path    | matcher | expected        |
      | catalog | equals  | kr://dw/catalog |

    When I run `kc local repository attach --home "$KC_HOME" --catalog kr://dw/catalog --repo kr://dw/physical`
    Then stdout JSON satisfies:
      | path         | matcher      | expected         |
      | repositoryId | equals       | kr://dw/physical |
      | head         | is non-empty |                  |

    When I run `kc local repository attach --home "$KC_HOME" --catalog kr://dw/catalog --repo kr://dw/semantic`
    Then stdout JSON satisfies:
      | path         | matcher      | expected         |
      | repositoryId | equals       | kr://dw/semantic |
      | head         | is non-empty |                  |

    When I run `kc local grant bootstrap --home "$KC_HOME" --principal service:e2e`
    Then the command succeeds

    When I run `kc writer ingest --repo kr://dw/physical --dir "$FIXTURE/knowledge/schemas/physical" --out "$RUN/physical-schema.changeset.json" --origin-kind DEFINITION --actor-ref data-warehouse-domain-model --source-ref knowledge://data-warehouse/physical-aspects/v1`
    Then stdout JSON satisfies:
      | path                      | matcher    | expected |
      | diagnostics.schemaObjects | equals     | 9        |
      | diagnostics.files         | equals     | 9        |
    And JSON file "$RUN/physical-schema.changeset.json" satisfies:
      | path       | matcher    | expected |
      | operations | has length | 9        |

    When I run `kc writer commit --command-id dw-cli-04-physical-schema --changeset "$RUN/physical-schema.changeset.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/physical |
      | result.commitId     | is non-empty |                  |

    When I run `kc writer ingest --repo kr://dw/physical --dir "$FIXTURE/knowledge/physical" --out "$RUN/physical-resource.changeset.json" --origin-kind DEFINITION --actor-ref data-warehouse-domain-model --source-ref knowledge://data-warehouse/physical-aspects/v1`
    Then stdout JSON satisfies:
      | path                      | matcher    | expected |
      | diagnostics.schemaObjects | equals     | 0        |
      | diagnostics.files         | equals     | 1        |
    And JSON file "$RUN/physical-resource.changeset.json" satisfies:
      | path       | matcher    | expected |
      | operations | has length | 1        |

    When I run `kc writer commit --command-id dw-cli-04-physical-resource --changeset "$RUN/physical-resource.changeset.json" | tee "$RUN/physical-schema.receipt.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/physical |
      | result.commitId     | is non-empty |                  |

    When I run `printf '%s\n' '{"checkpoint":{},"signal":{"kind":"bootstrap-full"}}' | "$PYTHON" "$FIXTURE/connector/collector.py" | tee "$RUN/mysql-v1.observation.json"`
    Then stdout JSON satisfies:
      | path                      | matcher    | expected |
      | observation.coverage.kind | equals     | FULL     |
      | desired                   | has length | 101      |

    When I run `"$CONNECTOR_PREVIEW" --manifest "$FIXTURE/connector/connector.yaml" --observation "$RUN/mysql-v1.observation.json" --base "$(jq -r '.result.commitId' "$RUN/physical-schema.receipt.json")" --out "$RUN/mysql-v1.preview.json"`
    Then the command succeeds
    And JSON file "$RUN/mysql-v1.preview.json" satisfies:
      | path          | matcher | expected |
      | empty         | equals  | false    |
      | summary.added | equals  | 101      |

    When I run `jq '.changeSet' "$RUN/mysql-v1.preview.json" > "$RUN/mysql-v1.changeset.json"`
    Then the command succeeds
    And JSON file "$RUN/mysql-v1.changeset.json" satisfies:
      | path       | matcher    | expected |
      | operations | has length | 101      |

    When I run `kc writer commit --command-id dw-cli-04-mysql-v1 --changeset "$RUN/mysql-v1.changeset.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/physical |
      | result.commitId     | is non-empty |                  |

    When I run `kc writer ingest --repo kr://dw/semantic --dir "$FIXTURE/knowledge/schemas/semantic" --out "$RUN/semantic-schema.changeset.json" --origin-kind DEFINITION --actor-ref semantic-sales --source-ref knowledge://finance/tpch-sales`
    Then stdout JSON satisfies:
      | path                       | matcher    | expected |
      | diagnostics.schemaObjects  | equals     | 7        |
      | diagnostics.knowledgeUnits | equals     | 0        |
      | diagnostics.files          | equals     | 7        |
    And JSON file "$RUN/semantic-schema.changeset.json" satisfies:
      | path       | matcher    | expected |
      | operations | has length | 7        |

    When I run `kc writer commit --command-id dw-cli-04-semantic-schema --changeset "$RUN/semantic-schema.changeset.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/semantic |
      | result.commitId     | is non-empty |                  |

    When I run `kc writer ingest --repo kr://dw/semantic --dir "$FIXTURE/knowledge/semantic" --out "$RUN/semantic.changeset.json" --origin-kind DEFINITION --actor-ref semantic-sales --source-ref knowledge://finance/tpch-sales`
    Then stdout JSON satisfies:
      | path                       | matcher    | expected |
      | diagnostics.schemaObjects  | equals     | 0        |
      | diagnostics.knowledgeUnits | equals     | 8        |
      | diagnostics.files          | equals     | 8        |
    And JSON file "$RUN/semantic.changeset.json" satisfies:
      | path       | matcher    | expected |
      | operations | has length | 8        |

    When I run `kc writer commit --command-id dw-cli-04-semantic --changeset "$RUN/semantic.changeset.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/semantic |
      | result.commitId     | is non-empty |                  |

    When I run `kc catalog workspace define --catalog kr://dw/catalog --workspace warehouse-agent --revision 1 --source kr://dw/physical=refs/heads/main --source kr://dw/semantic=refs/heads/main`
    Then stdout JSON satisfies:
      | path        | matcher    | expected        |
      | workspaceId | equals     | warehouse-agent |
      | revision    | equals     | 1               |
      | sources     | has length | 2               |

    When I run `kc catalog workspace resolve --catalog kr://dw/catalog --workspace warehouse-agent | tee "$RUN/v1.pin.json"`
    Then stdout JSON satisfies:
      | path                         | matcher      | expected        |
      | workspaceId                  | equals       | warehouse-agent |
      | pinId                        | is non-empty |                  |
      | repositories.kr://dw/physical | is non-empty |                |
      | repositories.kr://dw/semantic | is non-empty |                |

    When I run `docker exec --env MYSQL_PWD=dw-test-root "$KC_MYSQL_CONTAINER" mysql --user=root --database=tpch --execute 'ALTER TABLE orders DROP COLUMN o_comment, ADD COLUMN o_pipeline_note VARCHAR(64) NULL'`
    Then the command succeeds

    When I run `jq '{checkpoint:.nextCheckpoint,signal:{kind:"invalidation",keys:["mysql:fixture:table:tpch.orders"]}}' "$RUN/mysql-v1.observation.json" | "$PYTHON" "$FIXTURE/connector/collector.py" | tee "$RUN/mysql-v2.observation.json"`
    Then stdout JSON satisfies:
      | path                      | matcher    | expected                                |
      | observation.coverage.kind | equals     | KEYS                                    |
      | observation.coverage.keys | has length | 1                                       |
      | observation.coverage.keys[0] | equals  | mysql:fixture:table:tpch.orders         |
      | desired                   | has length | 12                                      |
      | observed                  | has length | 12                                      |
      | nextCheckpoint.observed   | has length | 101                                     |

    When I run `"$CONNECTOR_PREVIEW" --manifest "$FIXTURE/connector/connector.yaml" --observation "$RUN/mysql-v2.observation.json" --base "$(kc writer head --repo kr://dw/physical | jq -r '.commit')" --out "$RUN/mysql-v2.preview.json"`
    Then the command succeeds
    And JSON file "$RUN/mysql-v2.preview.json" satisfies:
      | path              | matcher    | expected |
      | empty             | equals     | false    |
      | summary.added     | equals     | 1        |
      | summary.updated   | equals     | 1        |
      | summary.removed   | equals     | 1        |
      | summary.unchanged | equals     | 10       |
      | changeSet.operations | has length | 3     |

    When I run `jq '.changeSet' "$RUN/mysql-v2.preview.json" > "$RUN/mysql-v2.changeset.json"`
    Then the command succeeds
    And JSON file "$RUN/mysql-v2.changeset.json" satisfies:
      | path       | matcher    | expected |
      | operations | has length | 3        |

    When I run `kc writer commit --command-id dw-cli-04-mysql-v2 --changeset "$RUN/mysql-v2.changeset.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/physical |
      | result.commitId     | is non-empty |                  |

    When I run `kc catalog workspace resolve --catalog kr://dw/catalog --workspace warehouse-agent | tee "$RUN/v2.pin.json"`
    Then stdout JSON satisfies:
      | path                         | matcher      | expected        |
      | workspaceId                  | equals       | warehouse-agent |
      | pinId                        | is non-empty |                  |
      | repositories.kr://dw/physical | is non-empty |                |

    When I run `kc knowledge read --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-column-1e32257e9f6b3a08d89fb42b`
    Then stdout JSON satisfies:
      | path                      | matcher    | expected  |
      | $                         | has length | 1         |
      | [0].value.properties.name | equals     | o_comment |

    When I run `kc knowledge read --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-column-ec6633d61d0dc89bd96b91b7`
    Then stdout JSON satisfies:
      | path | matcher    | expected |
      | $    | has length | 0        |

    When I run `kc knowledge read --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v2.pin.json" --object dw-mysql-tpch-column-1e32257e9f6b3a08d89fb42b`
    Then stdout JSON satisfies:
      | path | matcher    | expected |
      | $    | has length | 0        |

    When I run `kc knowledge read --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v2.pin.json" --object dw-mysql-tpch-column-ec6633d61d0dc89bd96b91b7`
    Then stdout JSON satisfies:
      | path                      | matcher    | expected        |
      | $                         | has length | 1               |
      | [0].value.properties.name | equals     | o_pipeline_note |

    When I run `kc knowledge relations --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object kc://dw/physical/dw-mysql-tpch-column-1e32257e9f6b3a08d89fb42b --relation-type contains --role member`
    Then the command fails with stdout error code "CAPABILITY_UNSATISFIED"

    When I run `kc knowledge relations --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v2.pin.json" --object kc://dw/physical/dw-mysql-tpch-column-1e32257e9f6b3a08d89fb42b --relation-type contains --role member`
    Then the command fails with stdout error code "CAPABILITY_UNSATISFIED"
