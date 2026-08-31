package cli

import "kc/knowledge"

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
	observation.putCount = puts
	observation.removeCount = removes
	observation.writerCountsSet = true
}
