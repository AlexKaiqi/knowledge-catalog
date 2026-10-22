package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"kc/kernel"
	objectmodel "kc/object"
)

// MemberComputations reuses immutable construction values. The input identity
// includes construction rules and effective fields/values, never caller rights,
// a Dataset revision or source attribution. Implementations own durable leases,
// integrity, capacity and reclamation; a miss must execute compute unchanged.
type MemberComputations interface {
	ComputeMember(context.Context, kernel.Digest, func() ([]byte, error)) ([]byte, error)
}

type memberConstruction struct {
	Version int        `json:"version"`
	Stage   string     `json:"stage"`
	Text    string     `json:"text,omitempty"`
	Values  [][]string `json:"values,omitempty"`
}

func constructMember(ctx context.Context, build declarationBuild, values [][]string, cache MemberComputations) (memberConstruction, error) {
	compute := func() (memberConstruction, error) {
		result := memberConstruction{Version: 1, Stage: "member"}
		if build.declaration.Build.Kind == "text" || build.declaration.Build.Kind == "vector" {
			text, err := declarationText(build.declaration, values)
			result.Text = text
			if err != nil {
				return result, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "member %s: %v", objectmodel.AddressLabel(build.member), err)
			}
			return result, nil
		}
		result.Values = make([][]string, len(values))
		for i, row := range values {
			for _, value := range row {
				result.Values[i] = append(result.Values[i], declarationScalar(build.declaration, value))
			}
		}
		return result, nil
	}
	if cache == nil {
		return compute()
	}
	fields := make([]AccessField, len(build.inputs))
	forms := make([]string, len(build.inputs))
	for i, input := range build.inputs {
		// Inputs were resolved against the fixed adopted Schema during declaration
		// compilation. Vector-only fields intentionally have no query face.
		field := AccessField{FieldRef: input.Field, Type: input.Type}

		fields[i], forms[i] = field, input.form
	}
	key := kernel.CanonicalDigest(struct {
		Compiler string
		Catalog  string
		Inputs   []objectmodel.IndexInput
		Build    objectmodel.IndexBuild
		Fields   []AccessField
		Forms    []string
		Values   [][]string
	}{"member-construction-v1", build.declaration.Catalog, build.declaration.Inputs, build.declaration.Build, fields, forms, values})
	raw, err := cache.ComputeMember(ctx, key, func() ([]byte, error) {
		result, err := compute()
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	})
	if err != nil {
		return memberConstruction{}, err
	}
	var result memberConstruction
	if err := decodeComputationValue(raw, &result); err != nil {
		return result, err
	}

	text := build.declaration.Build.Kind == "text" || build.declaration.Build.Kind == "vector"
	if result.Version != 1 || result.Stage != "member" || (text && len(result.Values) != 0) || (!text && (result.Text != "" || len(result.Values) != len(values))) {
		return result, kernel.Fail(kernel.ErrPreconditionFailed, "member computation has incompatible construction shape")
	}
	return result, nil
}

func decodeComputationValue(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return kernel.Fail(kernel.ErrPreconditionFailed, "invalid member computation value: %v", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return kernel.Fail(kernel.ErrPreconditionFailed, "member computation has trailing data")
	}
	return nil
}
