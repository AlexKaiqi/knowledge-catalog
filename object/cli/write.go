package cli

import (
	"context"
	"encoding/json"
	objectmodel "kc/object"
	"kc/object/client"
	"os"
	"strconv"
	"strings"
	"time"
)

// ReceiptWaitBudget preserves the CLI's established outcome-unknown policy.
func ReceiptWaitBudget() time.Duration {
	raw := strings.TrimSpace(os.Getenv("KC_COMMIT_RECEIPT_WAIT"))
	if raw == "" {
		return 10 * time.Minute
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if value, err := time.ParseDuration(raw); err == nil {
		return value
	}
	return 10 * time.Minute
}

func (e *executor) write(ctx context.Context, path string, flags map[string]any, options client.RequestOptions) (any, error) {
	var value any
	var err error
	if path == "writer put" {
		value, err = jsonValue(flags)
		if err != nil {
			return nil, err
		}
	}
	source, err := valueSource(flags)
	if err != nil {
		return nil, err
	}
	reference := flagString(flags, "reference")
	wait := e.context.ReceiptWait
	if wait == 0 {
		wait = ReceiptWaitBudget()
	}
	request := client.MaintenanceRequest{Reference: reference, Repository: flagString(flags, "repo"), CommandID: flagString(flags, "command-id"), Ref: flagString(flags, "ref"), Base: flagString(flags, "base"), Expected: flagString(flags, "expected"), Value: value, Kind: flagString(flags, "kind"), SchemaRef: flagString(flags, "schema-ref"), PathHint: flagString(flags, "path-hint"), IfAbsent: flagBool(flags, "if-absent"), IfDigest: flagString(flags, "if-digest"), Message: flagString(flags, "message"), Provenance: provenance(flags), ValueSource: source}
	return client.MaintainWithReceiptRecovery(ctx, e.client, path == "writer remove", request, options, wait)
}

func valueSource(flags map[string]any) (*objectmodel.ValueSource, error) {
	raw := flagString(flags, "value-source")
	if raw == "" {
		return nil, nil
	}
	var source objectmodel.ValueSource
	if err := json.Unmarshal([]byte(raw), &source); err != nil {
		return nil, usage("--value-source is not valid JSON")
	}
	if err := objectmodel.ValidateValueSource(&source); err != nil {
		return nil, err
	}
	return &source, nil
}

func provenance(flags map[string]any) *objectmodel.ProvenanceEnvelope {
	kind := flagString(flags, "origin-kind")
	sources := flagStrings(flags, "source-ref")
	evidence := flagStrings(flags, "evidence-ref")
	actor := flagString(flags, "actor-ref")
	activity := flagString(flags, "activity-ref")
	input := flagString(flags, "input-dataset-version")
	spec := flagString(flags, "algorithm-spec")
	model := flagString(flags, "algorithm-model")
	hash := flagString(flags, "algorithm-hash")
	produced := flagString(flags, "produced-at")
	if kind == "" && len(sources) == 0 && len(evidence) == 0 && actor == "" && activity == "" && input == "" && spec == "" && model == "" && hash == "" && produced == "" {
		return nil
	}
	origin := objectmodel.OriginKind(kind)
	if origin == "" {
		origin = objectmodel.OriginSource
	}
	envelope := &objectmodel.ProvenanceEnvelope{OriginKind: origin, ActorRef: actor, ActivityRef: activity, SourceRefs: sources, EvidenceRefs: evidence, InputWorkspaceVersionRef: input, ProducedAt: produced}
	if spec != "" || model != "" || hash != "" {
		envelope.Algorithm = &objectmodel.AlgorithmRef{DerivationSpecRef: spec, ModelRef: model, CodeHash: hash}
	}
	return envelope
}
