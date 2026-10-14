// Package evidence retains the dynamic results actually adopted by a read.
// Access process records refer to these results without copying their payloads.
package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"kc/kernel"
	objectmodel "kc/object"
)

type Outcome string

const (
	ValueOutcome   Outcome = "value"
	FailureOutcome Outcome = "failure"
)

type Failure struct {
	Code    kernel.ErrorCode `json:"code"`
	Message string           `json:"message"`
}

// Sample is one adopted dynamic unit. ValueOutcome includes an explicit JSON
// null; FailureOutcome has a reason and no value or successful observation.
type Sample struct {
	Repository  kernel.RepositoryID          `json:"repository"`
	Commit      kernel.CommitID              `json:"commit"`
	Address     objectmodel.Address          `json:"address"`
	Declaration objectmodel.UnitDeclaration  `json:"declaration"`
	Outcome     Outcome                      `json:"outcome"`
	Value       any                          `json:"-"`
	Failure     *Failure                     `json:"failure,omitempty"`
	Observation *objectmodel.UnitObservation `json:"observation,omitempty"`
}

type Record struct {
	Identity  kernel.IdentityContext `json:"identity"`
	Trace     kernel.TraceContext    `json:"trace,omitempty"`
	RequestID string                 `json:"requestId"`
	Units     []Sample               `json:"units"`
}

// Reference is the payload-free pointer retained by the access process ledger.
type Reference struct {
	ID          string        `json:"id"`
	Digest      kernel.Digest `json:"digest"`
	OccurredAt  string        `json:"occurredAt"`
	RetainUntil string        `json:"retainUntil"`
}

// Store acknowledges a Record only after it is durable. Read returns the same
// adopted results and never asks a source for replacement values.
type Store interface {
	Record(ctx context.Context, record Record) (Reference, error)
	Read(ctx context.Context, reference Reference) (Record, error)
}

func (r Record) Validate() error {
	if err := r.Identity.Validate(); err != nil {
		return err
	}
	if err := r.Trace.Validate(); err != nil {
		return err
	}
	if r.RequestID == "" || strings.TrimSpace(r.RequestID) != r.RequestID || len(r.Units) == 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "retained results require a requestId and adopted units")
	}
	seen := map[sampleTarget]bool{}
	for _, sample := range r.Units {
		if err := sample.Validate(); err != nil {
			return err
		}
		target := sampleTarget{sample.Repository, sample.Commit, sample.Address}
		if seen[target] {
			return kernel.Fail(kernel.ErrUsageInvalid, "retained results repeat an adopted unit")
		}
		seen[target] = true
	}
	return nil
}

type sampleTarget struct {
	repository kernel.RepositoryID
	commit     kernel.CommitID
	address    objectmodel.Address
}

func (s Sample) Validate() error {
	if strings.TrimSpace(string(s.Repository)) == "" || strings.TrimSpace(string(s.Commit)) == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "retained unit requires repository and fixed commit")
	}
	if err := objectmodel.AssertWritable(s.Address); err != nil {
		return err
	}
	if s.Declaration.Address != s.Address || strings.TrimSpace(string(s.Declaration.DeclarationDigest)) == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "retained unit requires its fixed declaration")
	}
	switch s.Outcome {
	case ValueOutcome:
		return s.validateObservation()
	case FailureOutcome:
		if s.Value != nil || s.Observation != nil || s.Failure == nil || s.Failure.Code == "" || strings.TrimSpace(s.Failure.Message) == "" {
			return kernel.Fail(kernel.ErrUsageInvalid, "retained failure requires a reason without value or successful observation")
		}
		return nil
	default:
		return kernel.Fail(kernel.ErrUsageInvalid, "retained unit requires value or failure outcome")
	}
}

func (s Sample) validateObservation() error {
	if s.Failure != nil || s.Observation == nil {
		return kernel.Fail(kernel.ErrUsageInvalid, "retained value requires a successful observation without failure")
	}
	o := s.Observation
	if o.Address != s.Address || o.DeclarationCommit != s.Commit || o.DeclarationDigest != s.Declaration.DeclarationDigest {
		return kernel.Fail(kernel.ErrUsageInvalid, "retained observation does not match its fixed declaration")
	}
	return objectmodel.ValidateObservationBasis(o.Basis)
}

type sampleJSON struct {
	Repository  kernel.RepositoryID          `json:"repository"`
	Commit      kernel.CommitID              `json:"commit"`
	Address     objectmodel.Address          `json:"address"`
	Declaration objectmodel.UnitDeclaration  `json:"declaration"`
	Outcome     Outcome                      `json:"outcome"`
	Value       json.RawMessage              `json:"value,omitempty"`
	Failure     *Failure                     `json:"failure,omitempty"`
	Observation *objectmodel.UnitObservation `json:"observation,omitempty"`
}

func (s Sample) MarshalJSON() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	wire := sampleJSON{Repository: s.Repository, Commit: s.Commit, Address: s.Address,
		Declaration: s.Declaration, Outcome: s.Outcome, Failure: s.Failure, Observation: s.Observation}
	if s.Outcome == ValueOutcome {
		value, err := json.Marshal(s.Value)
		if err != nil {
			return nil, err
		}
		wire.Value = value
	}
	return json.Marshal(wire)
}

func (s *Sample) UnmarshalJSON(raw []byte) error {
	var wire sampleJSON
	if err := kernel.UnmarshalJSON(raw, &wire); err != nil {
		return err
	}
	decoded := Sample{Repository: wire.Repository, Commit: wire.Commit, Address: wire.Address,
		Declaration: wire.Declaration, Outcome: wire.Outcome, Failure: wire.Failure, Observation: wire.Observation}
	if wire.Outcome == ValueOutcome {
		if len(wire.Value) == 0 {
			return fmt.Errorf("retained value is missing (use null for an observed null)")
		}
		if err := kernel.UnmarshalJSON(wire.Value, &decoded.Value); err != nil {
			return err
		}
	} else if len(wire.Value) != 0 {
		return fmt.Errorf("retained failure cannot contain a value")
	}
	if err := decoded.Validate(); err != nil {
		return err
	}
	*s = decoded
	return nil
}
