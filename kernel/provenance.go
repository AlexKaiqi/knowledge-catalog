package kernel

type OriginKind string

const (
	OriginSource      OriginKind = "SOURCE"
	OriginObservation OriginKind = "OBSERVATION"
	OriginEvidence    OriginKind = "EVIDENCE"
	OriginAssertion   OriginKind = "ASSERTION"
	OriginDefinition  OriginKind = "DEFINITION"
	OriginDerivation  OriginKind = "DERIVATION"
)

type AlgorithmRef struct {
	DerivationSpecRef string `json:"derivationSpecRef,omitempty"`
	ModelRef          string `json:"modelRef,omitempty"`
	CodeHash          string `json:"codeHash,omitempty"`
}

func (a AlgorithmRef) Identified() bool {
	return a.DerivationSpecRef != "" || a.ModelRef != "" || a.CodeHash != ""
}

type ProvenanceEnvelope struct {
	OriginKind              OriginKind    `json:"originKind"`
	ActorRef                string        `json:"actorRef,omitempty"`
	ActivityRef             string        `json:"activityRef,omitempty"`
	SourceRefs              []string      `json:"sourceRefs,omitempty"`
	EvidenceRefs            []string      `json:"evidenceRefs,omitempty"`
	InputViewReadVersionRef string        `json:"inputViewReadVersionRef,omitempty"`
	Algorithm               *AlgorithmRef `json:"algorithm,omitempty"`
	ProducedAt              string        `json:"producedAt,omitempty"`
}

func ValidateProvenance(p *ProvenanceEnvelope) error {
	if p == nil || p.OriginKind != OriginDerivation {
		return nil
	}
	identified := p.Algorithm != nil && p.Algorithm.Identified()
	if p.InputViewReadVersionRef == "" || !identified {
		return Fail(ErrPreconditionFailed, "DERIVATION provenance requires a fixed input ViewReadVersion and algorithm identity")
	}
	return nil
}
