package scenario

import (
	"kc/kernel"
	"kc/repository"
)

func schemaDoc(entity, aspect string, fields map[string]any) map[string]any {
	return map[string]any{
		"entity":  entity,
		"aspect":  aspect,
		"pattern": "record",
		"fields":  fields,
	}
}

func field(access ...string) map[string]any {
	vals := make([]any, len(access))
	for i, a := range access {
		vals[i] = a
	}
	return map[string]any{"access": vals}
}

func putSchema(objectID, entity, aspect string, fields map[string]any) repository.Operation {
	return repository.Operation{
		Op:      repository.OpPut,
		Address: kernel.Address{Kind: kernel.KindEntity, ObjectID: kernel.ObjectID(objectID)},
		Value:   schemaDoc(entity, aspect, fields),
	}
}

func putAspect(objectID kernel.ObjectID, aspect string, value any, schemaRef, pathHint string) repository.Operation {
	return repository.Operation{
		Op:        repository.OpPut,
		Address:   kernel.Address{Kind: kernel.KindAspect, ObjectID: objectID, AspectName: aspect},
		Value:     value,
		SchemaRef: schemaRef,
		PathHint:  pathHint,
	}
}

func putEntity(objectID kernel.ObjectID, value any, pathHint string) repository.Operation {
	return repository.Operation{
		Op:       repository.OpPut,
		Address:  kernel.Address{Kind: kernel.KindEntity, ObjectID: objectID},
		Value:    value,
		PathHint: pathHint,
	}
}

func sourceEnvelope(actor string) *kernel.ProvenanceEnvelope {
	return &kernel.ProvenanceEnvelope{
		OriginKind: kernel.OriginSource,
		ActorRef:   actor,
		SourceRefs: []string{"metastore"},
	}
}

func definitionEnvelope(actor string) *kernel.ProvenanceEnvelope {
	return &kernel.ProvenanceEnvelope{
		OriginKind: kernel.OriginDefinition,
		ActorRef:   actor,
	}
}

func observationEnvelope(actor string) *kernel.ProvenanceEnvelope {
	return &kernel.ProvenanceEnvelope{
		OriginKind: kernel.OriginObservation,
		ActorRef:   actor,
	}
}

func nestedString(v any, keys ...string) string {
	cur := v
	for _, key := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[key]
	}
	s, _ := cur.(string)
	return s
}

func metadataBoot() []repository.Operation {
	return []repository.Operation{
		putSchema(SchemaTableStruct, "Table", "structure", map[string]any{
			"db":   field("filter", "key"),
			"name": field("filter", "text"),
		}),
		putSchema(SchemaTableOwner, "Table", "ownership", map[string]any{
			"owner": field("filter"),
		}),
		putAspect(TableTrade, "structure", map[string]any{
			"db": "dw", "name": "dwd.trade_order",
		}, SchemaTableStruct, "tables/dwd.trade_order.structure.json"),
		putAspect(TableTrade, "ownership", map[string]any{
			"owner": "platform",
		}, SchemaTableOwner, "tables/dwd.trade_order.ownership.json"),
	}
}

func semanticsBoot() []repository.Operation {
	return []repository.Operation{
		putSchema(SchemaMetricDef, "Metric", "definition", map[string]any{
			"formula":     field("text"),
			"description": field("text", "summary"),
		}),
		putSchema(SchemaExampleBody, "Example", "body", map[string]any{
			"prompt": field("text"),
		}),
		putAspect(ExampleGMV, "body", map[string]any{
			"prompt": "退货是否算进 GMV？",
		}, SchemaExampleBody, "examples/gmv-refund.md"),
	}
}

func personalBoot() []repository.Operation {
	return []repository.Operation{
		putSchema(SchemaHabitNote, "Habit", "note", map[string]any{
			"when": field("filter"),
			"text": field("text"),
		}),
		putSchema(SchemaDistStats, "Dist", "stats", map[string]any{
			"topic": field("filter", "text"),
			"count": field("filter"),
		}),
		putAspect(HabitMorning, "note", map[string]any{
			"when": "morning", "text": "每天先看昨日异常单",
		}, SchemaHabitNote, ""),
		putAspect(DistErrors, "stats", map[string]any{
			"topic": "退款口径", "count": "12",
		}, SchemaDistStats, ""),
	}
}

func gmvClaim(formula string) []repository.Operation {
	return []repository.Operation{
		putAspect(MetricGMV, "definition", map[string]any{
			"formula":     formula,
			"description": "组织认领的交易额口径",
		}, SchemaMetricDef, "metrics/gmv.json"),
	}
}

func gmvDraft(formula string) []repository.Operation {
	return []repository.Operation{
		putAspect(MetricGMV, "definition", map[string]any{
			"formula":     formula,
			"description": "未发表的个人理解",
		}, "", "drafts/gmv.json"),
	}
}
