package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func decodeJSONBody(r *http.Request) (map[string]any, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return map[string]any{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("body must be a JSON object")
	}
	return raw, nil
}

func flagsFromRequest(home string, raw map[string]any) (map[string]FlagValue, func(), error) {
	flags := map[string]FlagValue{"home": home}
	var cleanup []string
	done := func() {
		for _, path := range cleanup {
			_ = os.Remove(path)
		}
	}
	for key, value := range raw {
		if key == "home" || key == "listen" {
			continue
		}
		if key == "changeset" {
			if object, ok := value.(map[string]any); ok {
				body, err := json.Marshal(object)
				if err != nil {
					return nil, done, err
				}
				file, err := os.CreateTemp(home, "changeset-*.json")
				if err != nil {
					return nil, done, err
				}
				if _, err := file.Write(append(body, '\n')); err != nil {
					_ = file.Close()
					return nil, done, err
				}
				if err := file.Close(); err != nil {
					return nil, done, err
				}
				cleanup = append(cleanup, file.Name())
				flags[key] = file.Name()
				continue
			}
		}
		flag, err := jsonToFlag(value)
		if err != nil {
			return nil, done, err
		}
		if flag != nil {
			flags[key] = flag
		}
	}
	return flags, done, nil
}

func jsonToFlag(value any) (FlagValue, error) {
	switch item := value.(type) {
	case nil:
		return nil, nil
	case bool:
		return item, nil
	case float64:
		if item == float64(int64(item)) {
			return fmt.Sprintf("%d", int64(item)), nil
		}
		return fmt.Sprint(item), nil
	case []any:
		out := make([]string, 0, len(item))
		for _, value := range item {
			text, err := jsonToString(value)
			if err != nil {
				return nil, err
			}
			out = append(out, text)
		}
		return out, nil
	case map[string]any:
		body, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		return string(body), nil
	default:
		return fmt.Sprint(item), nil
	}
}

func jsonToString(value any) (string, error) {
	switch item := value.(type) {
	case map[string]any, []any:
		body, err := json.Marshal(item)
		return string(body), err
	default:
		flag, err := jsonToFlag(value)
		if err != nil {
			return "", err
		}
		if flag == nil {
			return "", nil
		}
		return fmt.Sprint(flag), nil
	}
}
