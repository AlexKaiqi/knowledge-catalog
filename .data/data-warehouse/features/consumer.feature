@cli @mysql
Feature: 第一次接触的数据消费方通过 Workspace 使用数仓知识

  @DW-CLI-03 @resource
  Scenario: 消费方组合两个 Repository 并读取表、作业、语义和来源
    When I run `kc local init --home "$KC_HOME" --catalog kr://dw/catalog`
    Then stdout JSON satisfies:
      | path    | matcher | expected        |
      | catalog | equals  | kr://dw/catalog |

    When I run `kc local repository attach --home "$KC_HOME" --catalog kr://dw/catalog --repo kr://dw/physical`
    Then stdout JSON satisfies:
      | path         | matcher      | expected         |
      | repositoryId | equals       | kr://dw/physical |
      | head         | is non-empty |                  |

    When I run `kc catalog repo register --catalog kr://dw/catalog --repo kr://dw/physical`
    Then the command succeeds

    When I run `kc local repository attach --home "$KC_HOME" --catalog kr://dw/catalog --repo kr://dw/semantic`
    Then stdout JSON satisfies:
      | path         | matcher      | expected         |
      | repositoryId | equals       | kr://dw/semantic |
      | head         | is non-empty |                  |

    When I run `kc catalog repo register --catalog kr://dw/catalog --repo kr://dw/semantic`
    Then the command succeeds

    When I run `kc local grant bootstrap --home "$KC_HOME" --principal service:e2e`
    Then the command succeeds

    When I run `kc whoami`
    Then stdout JSON satisfies:
      | path      | matcher | expected    |
      | principal | equals  | service:e2e |

    When I run `kc pack --repo kr://dw/physical --dir "$FIXTURE/knowledge/schemas/physical" --out "$RUN/physical-schema.changeset.json" --origin-kind DEFINITION --actor-ref data-warehouse-domain-model --source-ref knowledge://data-warehouse/physical-aspects/v1`
    Then stdout JSON satisfies:
      | path                       | matcher    | expected |
      | diagnostics.schemaObjects  | equals     | 9        |
      | diagnostics.knowledgeUnits | equals     | 0        |
      | diagnostics.files          | equals     | 9        |
    And JSON file "$RUN/physical-schema.changeset.json" satisfies:
      | path       | matcher    | expected |
      | operations | has length | 9        |

    When I run `kc writer commit --command-id dw-cli-03-physical-schema --changeset "$RUN/physical-schema.changeset.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/physical |
      | result.commitId     | is non-empty |                  |

    When I run `kc pack --repo kr://dw/physical --dir "$FIXTURE/knowledge/physical" --out "$RUN/physical-resource.changeset.json" --origin-kind DEFINITION --actor-ref data-warehouse-domain-model --source-ref knowledge://data-warehouse/physical-aspects/v1`
    Then stdout JSON satisfies:
      | path                       | matcher    | expected |
      | diagnostics.schemaObjects  | equals     | 0        |
      | diagnostics.knowledgeUnits | equals     | 1        |
      | diagnostics.files          | equals     | 1        |
    And JSON file "$RUN/physical-resource.changeset.json" satisfies:
      | path       | matcher    | expected |
      | operations | has length | 1        |

    When I run `kc writer commit --command-id dw-cli-03-physical-resource --changeset "$RUN/physical-resource.changeset.json" | tee "$RUN/physical-schema.receipt.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/physical |
      | result.commitId     | is non-empty |                  |

    When I run `printf '%s\n' '{"checkpoint":{},"signal":{"kind":"bootstrap-full"}}' | "$PYTHON" "$FIXTURE/connector/collector.py" | tee "$RUN/mysql.observation.json"`
    Then stdout JSON satisfies:
      | path                      | matcher    | expected |
      | observation.coverage.kind | equals     | FULL     |
      | desired                   | has length | 101      |

    When I run `"$CONNECTOR_PREVIEW" --manifest "$FIXTURE/connector/connector.yaml" --observation "$RUN/mysql.observation.json" --base "$(jq -r '.result.commitId' "$RUN/physical-schema.receipt.json")" --out "$RUN/mysql.preview.json"`
    Then the command succeeds
    And JSON file "$RUN/mysql.preview.json" satisfies:
      | path          | matcher | expected |
      | empty         | equals  | false    |
      | summary.added | equals  | 101      |

    When I run `jq '.changeSet' "$RUN/mysql.preview.json" > "$RUN/mysql.changeset.json"`
    Then the command succeeds
    And JSON file "$RUN/mysql.changeset.json" satisfies:
      | path             | matcher    | expected         |
      | targetRepository | equals     | kr://dw/physical |
      | operations       | has length | 101              |

    When I run `kc writer commit --command-id dw-cli-03-mysql --changeset "$RUN/mysql.changeset.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/physical |
      | result.commitId     | is non-empty |                  |

    When I run `kc pack --repo kr://dw/semantic --dir "$FIXTURE/knowledge/schemas/semantic" --out "$RUN/semantic-schema.changeset.json" --origin-kind DEFINITION --actor-ref semantic-sales --source-ref knowledge://finance/tpch-sales`
    Then stdout JSON satisfies:
      | path                       | matcher    | expected |
      | diagnostics.schemaObjects  | equals     | 7        |
      | diagnostics.knowledgeUnits | equals     | 0        |
      | diagnostics.files          | equals     | 7        |
    And JSON file "$RUN/semantic-schema.changeset.json" satisfies:
      | path       | matcher    | expected |
      | operations | has length | 7        |

    When I run `kc writer commit --command-id dw-cli-03-semantic-schema --changeset "$RUN/semantic-schema.changeset.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/semantic |
      | result.commitId     | is non-empty |                  |

    When I run `kc pack --repo kr://dw/semantic --dir "$FIXTURE/knowledge/semantic" --out "$RUN/semantic.changeset.json" --origin-kind DEFINITION --actor-ref semantic-sales --source-ref knowledge://finance/tpch-sales`
    Then stdout JSON satisfies:
      | path                       | matcher    | expected |
      | diagnostics.schemaObjects  | equals     | 0        |
      | diagnostics.knowledgeUnits | equals     | 8        |
      | diagnostics.files          | equals     | 8        |
    And JSON file "$RUN/semantic.changeset.json" satisfies:
      | path       | matcher    | expected |
      | operations | has length | 8        |

    When I run `kc writer commit --command-id dw-cli-03-semantic --changeset "$RUN/semantic.changeset.json"`
    Then stdout JSON satisfies:
      | path                | matcher      | expected         |
      | disposition         | equals       | APPLIED          |
      | result.repositoryId | equals       | kr://dw/semantic |
      | result.commitId     | is non-empty |                  |

    When I run `kc workspace define --catalog kr://dw/catalog --workspace warehouse-agent --revision 1 --source kr://dw/physical=refs/heads/main --source kr://dw/semantic=refs/heads/main`
    Then stdout JSON satisfies:
      | path        | matcher    | expected        |
      | workspaceId | equals     | warehouse-agent |
      | revision    | equals     | 1               |
      | sources     | has length | 2               |

    When I run `kc catalog list`
    Then stdout JSON satisfies:
      | path           | matcher    | expected        |
      | catalogs       | has length | 1               |
      | catalogs[0].id | equals     | kr://dw/catalog |

    When I run `kc catalog show --catalog kr://dw/catalog`
    Then stdout JSON satisfies:
      | path                        | matcher    | expected          |
      | catalogId                   | equals     | kr://dw/catalog   |
      | workspaces                  | has length | 1                 |
      | workspaces[0].workspaceId   | equals     | warehouse-agent   |
      | repositories                | contains   | kr://dw/physical  |
      | repositories                | contains   | kr://dw/semantic  |

    When I run `kc catalog repo list --catalog kr://dw/catalog`
    Then stdout JSON satisfies:
      | path         | matcher  | expected         |
      | catalogId    | equals   | kr://dw/catalog  |
      | repositories | contains | kr://dw/physical |
      | repositories | contains | kr://dw/semantic |

    When I run `kc workspace show --catalog kr://dw/catalog --workspace warehouse-agent`
    Then stdout JSON satisfies:
      | path         | matcher    | expected          |
      | workspaceId  | equals     | warehouse-agent   |
      | revision     | equals     | 1                 |
      | repositories | has length | 2                 |
      | repositories | contains   | kr://dw/physical  |
      | repositories | contains   | kr://dw/semantic  |

    When I run `kc catalog audit --catalog kr://dw/catalog`
    Then stdout JSON satisfies:
      | path     | matcher      | expected |
      | source   | equals       | catalog  |
      | entries  | is non-empty |          |

    When I run `kc knowledge schema list --repo kr://dw/physical`
    Then stdout JSON satisfies:
      | path       | matcher      | expected         |
      | repository | equals       | kr://dw/physical |
      | commit     | is non-empty |                  |
      | schemas    | has length   | 9                |
      | exhausted  | equals       | true             |

    When I run `env -u KC_WORKSPACE kc workspace pin --catalog kr://dw/catalog --source kr://dw/physical`
    Then stdout JSON satisfies:
      | path                          | matcher      | expected |
      | pinId                         | is non-empty |          |
      | repositories                  | has length   | 1        |
      | repositories.kr://dw/physical | is non-empty |          |

    When I run `kc workspace list --catalog kr://dw/catalog`
    Then stdout JSON satisfies:
      | path                      | matcher    | expected        |
      | workspaces                | has length | 1               |
      | workspaces[0].workspaceId | equals     | warehouse-agent |

    When I run `kc workspace pin --catalog kr://dw/catalog --workspace warehouse-agent | tee "$RUN/v1.pin.json"`
    Then stdout JSON satisfies:
      | path                              | matcher      | expected        |
      | workspaceId                       | equals       | warehouse-agent |
      | revision                          | equals       | 1               |
      | pinId                             | is non-empty |                  |
      | repositories.kr://dw/physical     | is non-empty |                  |
      | repositories.kr://dw/semantic     | is non-empty |                  |

    When I run `kc workspace pin --catalog kr://dw/catalog --workspace warehouse-agent --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068`
    Then the command fails with stdout error code "USAGE_INVALID"

    When I run `kc workspace pin --catalog kr://dw/catalog --workspace warehouse-agent --aspect properties`
    Then the command fails with stdout error code "USAGE_INVALID"

    When I run `kc workspace check --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json"`
    Then stdout JSON satisfies:
      | path        | matcher    | expected        |
      | workspaceId | equals     | warehouse-agent |
      | outcome     | equals     | PASSED          |
      | issues      | has length | 0               |

    When I run `kc operations access-spec describe --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json"`
    Then stdout JSON satisfies:
      | path        | matcher    | expected        |
      | workspaceId | equals     | warehouse-agent |
      | specs       | has length | 2               |

    When I run `kc knowledge read --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068`
    Then stdout JSON satisfies:
      | path                              | matcher    | expected |
      | $                                 | has length | 1        |
      | [0].value.properties.name         | equals     | lineitem |
      | [0].value.schema.columnCount      | equals     | 16       |

    When I run `kc knowledge read --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object resource/mysql-tpch-sql`
    Then stdout JSON satisfies:
      | path                        | matcher | expected           |
      | [0].value.kind              | equals  | ResourceDescriptor |
      | [0].value.runtime           | equals  | mysql-tpch         |
      | [0].value.protocol          | equals  | resource-access/v1 |
      | [0].value.access.query.call | equals  | mysql.query         |

    When I run `kc knowledge access --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object resource/mysql-tpch-sql --operation query --input '{"sql":"SELECT COUNT(*) FROM tpch.customer"}'`
    Then stdout JSON satisfies:
      | path                              | matcher      | expected                   |
      | operation                         | equals       | query                      |
      | result.rows                       | has length   | 1                          |
      | result.rows[0]                    | equals       | "1"                        |
      | result.rowCount                   | equals       | 1                          |
      | basis.runtimeGeneration           | equals       | mysql-tpch-fixture-v1      |
      | basis.descriptor.objectId         | equals       | resource/mysql-tpch-sql    |
      | basis.descriptor.commit           | is non-empty |                            |

    When I run `kc knowledge read --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-data-job-2da1aa95c4226ac7a681db63`
    Then stdout JSON satisfies:
      | path                           | matcher | expected              |
      | [0].value.properties.name      | equals  | inspect_urgent_orders |
      | [0].value.definition.language  | equals  | SQL                   |
      | [0].value.definition.enabled   | equals  | false                 |

    When I run `kc knowledge read --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-semantic-sales-semantic-model-d40acd4665b1011643d74d5a`
    Then stdout JSON satisfies:
      | path                                     | matcher    | expected                                         |
      | [0].value.definition.baseTableRef        | equals     | dw-mysql-tpch-table-c02fedc564bba85c8d5d1068    |
      | [0].value.dimensions                     | is non-empty |                                                  |
      | [0].value.measures                       | is non-empty |                                                  |

    When I run `kc knowledge read --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-semantic-sales-metric-7630439d2660b81de165d124`
    Then stdout JSON satisfies:
      | path                         | matcher | expected                |
      | [0].value.properties.name    | equals  | Gross merchandise value |

    When I run `kc knowledge schema describe --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068`
    Then stdout JSON satisfies:
      | path | matcher    | expected |
      | $    | has length | 1        |
      | [0].schemas | has length | 2  |

    When I run `kc knowledge relations --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object kc://dw/physical/dw-mysql-tpch-table-c02fedc564bba85c8d5d1068 --relation-type contains --role member`
    Then the command fails with stdout error code "CAPABILITY_UNSATISFIED"

    When I run `kc knowledge provenance --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068`
    Then stdout JSON satisfies:
      | path                    | matcher | expected |
      | [0].chain[0].originKind | equals  | SOURCE   |

    When I run `kc knowledge resolve --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068`
    Then stdout JSON satisfies:
      | path       | matcher    | expected |
      | $          | has length | 1        |
      | [0].status | equals     | RESOLVED |

    When I run `kc knowledge resolve --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068 --aspect properties`
    Then stdout JSON satisfies:
      | path                    | matcher    | expected   |
      | $                       | has length | 1          |
      | [0].status              | equals     | RESOLVED   |
      | [0].address.aspectName  | equals     | properties |

    When I run `kc knowledge resolve --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object object/does-not-exist`
    Then stdout JSON satisfies:
      | path | matcher    | expected |
      | $    | has length | 0        |

    When I run `kc knowledge log --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068 --limit 20`
    Then stdout JSON satisfies:
      | path              | matcher      | expected                                       |
      | logs              | has length   | 1                                              |
      | logs[0].objectId  | equals       | dw-mysql-tpch-table-c02fedc564bba85c8d5d1068   |
      | logs[0].revisions | is non-empty |                                                |
      | exhausted         | equals       | true                                           |

    When I run `kc knowledge log --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068 --limit 0`
    Then stdout JSON satisfies:
      | path              | matcher      | expected                                       |
      | logs              | has length   | 1                                              |
      | logs[0].objectId  | equals       | dw-mysql-tpch-table-c02fedc564bba85c8d5d1068   |
      | logs[0].revisions | is non-empty |                                                |
      | exhausted         | equals       | true                                           |

    When I run `kc knowledge log --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068 --limit 201`
    Then the command fails with stdout error code "USAGE_INVALID"

    When I run `kc knowledge log --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068 --aspect properties`
    Then the command fails with stdout error code "USAGE_INVALID"

    When I run `kc knowledge provenance --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-semantic-sales-metric-7630439d2660b81de165d124`
    Then stdout JSON satisfies:
      | path                    | matcher | expected   |
      | [0].chain[0].originKind | equals  | DEFINITION |

    When I run `kc knowledge read --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object object/does-not-exist`
    Then stdout JSON satisfies:
      | path | matcher    | expected |
      | $    | has length | 0        |

    When I run `kc knowledge search --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --query lineitem`
    Then the command fails with stdout error code "CAPABILITY_UNSATISFIED"

    When I run `kc knowledge read --as analyst --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068`
    Then the command fails with stdout error code "FORBIDDEN"

    When I run `kc admin grant add --principal analyst --action workspace.consume --catalog kr://dw/catalog --workspace warehouse-agent`
    Then the command succeeds

    When I run `kc knowledge read --as analyst --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068`
    Then the command fails with stdout error code "FORBIDDEN"

    When I run `kc knowledge resolve --as analyst --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068`
    Then the command fails with stdout error code "FORBIDDEN"

    When I run `kc knowledge log --as analyst --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068`
    Then the command fails with stdout error code "FORBIDDEN"

    When I run `kc admin grant add --principal analyst --action knowledge.read --repo kr://dw/physical`
    Then the command succeeds

    When I run `kc admin grant add --principal analyst --action knowledge.read --repo kr://dw/semantic`
    Then the command succeeds

    When I run `kc knowledge read --as analyst --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068`
    Then stdout JSON satisfies:
      | path                          | matcher    | expected |
      | $                             | has length | 1        |
      | [0].value.properties.name     | equals     | lineitem |
      | [0].value.schema.columnCount  | equals     | 16       |

    When I run `kc knowledge resolve --as analyst --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --object dw-mysql-tpch-table-c02fedc564bba85c8d5d1068`
    Then stdout JSON satisfies:
      | path       | matcher    | expected |
      | $          | has length | 1        |
      | [0].status | equals     | RESOLVED |

    When I run `kc knowledge search --as analyst --catalog kr://dw/catalog --workspace warehouse-agent --pin "$RUN/v1.pin.json" --query lineitem`
    Then the command fails with stdout error code "CAPABILITY_UNSATISFIED"

    When I run `kc admin grant list`
    Then stdout JSON satisfies:
      | path  | matcher      | expected |
      | rules | is non-empty |          |
