package cli

import (
	"encoding/json"

	"kc/knowledge"
)

func setTelemetryChangeCounts(observation *operationTelemetry, operations []knowledge.Operation) {
	observation = noOperationTelemetry(observation)
	puts, removes := 0, 0
	for _, operation := range operations {
		switch operation.Op {
		case knowledge.OpPut:
			puts++
		case knowledge.OpRemove:
			removes++
		}
	}
	observation.PutCount = puts
	observation.RemoveCount = removes
	if payload, err := json.Marshal(operations); err == nil {
		observation.WriterPayloadBytes = len(payload)
	} else {
		observation.WriterPayloadBytes = -1
	}
	observation.WriterCountsSet = true
}
