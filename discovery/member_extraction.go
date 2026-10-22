package discovery

import (
	"context"
	"encoding/json"

	"kc/kernel"
	objectmodel "kc/object"
)

type memberFieldValues struct {
	Version int      `json:"version"`
	Stage   string   `json:"stage"`
	Values  []string `json:"values"`
}

// Cache only the normalized primitive. The current fixed declaration, unit
// applicability and observation proof have already been checked by the caller;
// attribution and Schema field identity are attached after this computation.
func extractFieldValues(ctx context.Context, id objectmodel.ObjectID, unit objectmodel.Address, field AccessField, raw any, cache MemberComputations) ([]string, error) {
	items := scalarValues(raw)
	if len(items) == 0 {
		return nil, nil
	}
	compute := func() (memberFieldValues, error) {
		result := memberFieldValues{Version: 1, Stage: "field", Values: make([]string, 0, len(items))}
		for _, item := range items {
			normalized, valid := NormalizeScalarValue(field.Type, item)
			if !valid {
				return result, kernel.Fail(kernel.ErrUsageInvalid, "projection %s unit %s field %s: value %v is not a valid %s scalar", id, objectmodel.AddressLabel(unit), displayFieldRef(field.FieldRef), item, field.Type)
			}
			result.Values = append(result.Values, normalized)
		}
		return result, nil
	}
	if cache == nil {
		result, err := compute()
		return result.Values, err
	}
	key := kernel.CanonicalDigest(struct {
		Compiler, Type string
		Values         []any
	}{"member-field-extraction-v1", field.Type, items})
	encoded, err := cache.ComputeMember(ctx, key, func() ([]byte, error) {
		result, err := compute()
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	})
	if err != nil {
		return nil, err
	}
	var result memberFieldValues
	if err := decodeComputationValue(encoded, &result); err != nil {
		return nil, err
	}
	if result.Version != 1 || result.Stage != "field" || len(result.Values) != len(items) {
		return nil, kernel.Fail(kernel.ErrPreconditionFailed, "member field computation has incompatible extraction shape")
	}
	return result.Values, nil
}
