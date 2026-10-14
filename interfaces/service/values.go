package service

import (
	"encoding/json"
	"kc/interfaces"

	"kc/kernel"
)

// isolateInterfaceCall prevents installed handlers and authorization ports from
// modifying caller input, repository-owned declarations, or each other's view.
// The scoped reader is a capability and is retained, not serialized as data.
func isolateInterfaceCall(call interfaces.InterfaceCall) (interfaces.InterfaceCall, error) {
	implementation, err := copyInterfaceJSON[interfaces.ResolvedInterface](call.Implementation)
	if err != nil {
		return interfaces.InterfaceCall{}, err
	}
	input, err := copyInterfaceJSON[any](call.Input)
	if err != nil {
		return interfaces.InterfaceCall{}, err
	}
	call.Implementation = implementation
	call.Input = input
	return call, nil
}

func copyInterfaceJSON[T any](value T) (T, error) {
	var copy T
	raw, err := json.Marshal(value)
	if err != nil {
		return copy, kernel.Fail(kernel.ErrUsageInvalid, "interface value must be serializable knowledge: %v", err)
	}
	if err := kernel.UnmarshalJSON(raw, &copy); err != nil {
		return copy, kernel.Fail(kernel.ErrUsageInvalid, "interface value cannot be decoded: %v", err)
	}
	return copy, nil
}
