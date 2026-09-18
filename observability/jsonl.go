package observability

import (
	"bufio"
	"encoding/json"
	"os"
)

func readJSONLFiles[T any](paths []string) ([]T, error) {
	out := []T{}
	for _, path := range paths {
		items, err := readJSONL[T](path)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

func readJSONL[T any](path string) ([]T, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []T{}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	out := []T{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var value T
		if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, scanner.Err()
}
