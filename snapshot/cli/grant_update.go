package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"kc/kernel"
	"kc/snapshot/access"
	kcclient "kc/snapshot/client"
)

// A defined wire type retains Rule's fields without its compatibility decoder,
// so the CLI can reject unknown input instead of silently ignoring it.
type grantRuleJSON access.Rule

func grantUpdateRequestFromFlags(flags map[string]any) (string, kcclient.GrantUpdateRequest, error) {
	if err := rejectFlagsOutside(flags, commandFlags("grant update"), "kc grant update"); err != nil {
		return "", kcclient.GrantUpdateRequest{}, err
	}
	id, err := requireRemoteFlag(flags, "id")
	if err != nil {
		return "", kcclient.GrantUpdateRequest{}, err
	}
	paths := map[string]string{}
	for _, name := range []string{"file", "expected-file"} {
		paths[name], err = requireRemoteFlag(flags, name)
		if err != nil {
			return "", kcclient.GrantUpdateRequest{}, err
		}
	}
	observed, _, err := readGrantRuleFile(paths["expected-file"])
	if err != nil {
		return "", kcclient.GrantUpdateRequest{}, err
	}
	if observed.ID != id {
		return "", kcclient.GrantUpdateRequest{}, kernel.Fail(kernel.ErrUsageInvalid, "expected rule ID must match --id")
	}
	replacement, fields, err := readGrantRuleFile(paths["file"])
	if err != nil {
		return "", kcclient.GrantUpdateRequest{}, err
	}
	if replacement.Principal == "" || len(replacement.Actions) == 0 {
		return "", kcclient.GrantUpdateRequest{}, kernel.Fail(kernel.ErrUsageInvalid, "replacement rule requires principal and actions")
	}
	for name, values := range map[string][2]string{
		"id": {replacement.ID, id}, "sharedBy": {replacement.SharedBy, observed.SharedBy}, "shareId": {replacement.ShareID, observed.ShareID},
	} {
		if _, present := fields[name]; present && values[0] != values[1] {
			return "", kcclient.GrantUpdateRequest{}, kernel.Fail(kernel.ErrUsageInvalid, "replacement cannot change %s", name)
		}
	}
	return id, kcclient.GrantUpdateRequest{
		GrantRequest: grantRequestFromRule(replacement),
		Expected: kcclient.GrantExpected{
			GrantRequest: grantRequestFromRule(observed), ID: observed.ID, SharedBy: observed.SharedBy, ShareID: observed.ShareID,
		},
	}, nil
}

func grantRequestFromRule(rule access.Rule) kcclient.GrantRequest {
	return kcclient.GrantRequest{
		Principal: rule.Principal, Actions: rule.Actions, Repository: rule.Repo,
		Dataset: rule.Dataset, Ref: rule.Ref, Object: rule.Object, Aspect: rule.Aspect,
	}
}

func readGrantRuleFile(path string) (access.Rule, map[string]json.RawMessage, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return access.Rule{}, nil, kernel.Fail(kernel.ErrUsageInvalid, "cannot read rule file %s: %v", path, err)
	}
	fields, err := grantRuleFields(raw)
	if err != nil {
		return access.Rule{}, nil, kernel.Fail(kernel.ErrUsageInvalid, "invalid rule file %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var rule grantRuleJSON
	if err := decoder.Decode(&rule); err != nil {
		return access.Rule{}, nil, kernel.Fail(kernel.ErrUsageInvalid, "invalid rule file %s: %v", path, err)
	}
	return access.Rule(rule), fields, nil
}

func grantRuleFields(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return nil, fmt.Errorf("expected one rule JSON object")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name := key.(string)
		switch name {
		case "id", "principal", "actions", "repo", "ref", "object", "aspect", "dataset", "shareId", "sharedBy":
		default:
			return nil, fmt.Errorf("unknown rule field %s", name)
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, fmt.Errorf("duplicate rule field %s", name)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("rule field %s must not be null", name)
		}
		fields[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("rule file must contain exactly one JSON object")
	}
	return fields, nil
}
