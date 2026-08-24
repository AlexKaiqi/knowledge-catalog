package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

type address struct {
	Kind       string `json:"kind"`
	ObjectID   string `json:"objectId"`
	AspectName string `json:"aspectName,omitempty"`
}

type unit struct {
	Address   address        `json:"address"`
	Value     map[string]any `json:"value"`
	PathHint  string         `json:"pathHint,omitempty"`
	SourceKey string         `json:"sourceKey,omitempty"`
}

type observed struct {
	Address address `json:"address"`
	Digest  string  `json:"digest"`
}

type runRequest struct {
	RunID      string          `json:"runId"`
	Checkpoint json.RawMessage `json:"checkpoint"`
}

type sourceFile struct {
	SourceRef  string `json:"sourceRef"`
	CapturedAt string `json:"capturedAt"`
	Facts      []fact `json:"facts"`
}

type fact struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type checkpoint struct {
	Observed []observed `json:"observed"`
}

type output struct {
	Observation struct {
		SourceRefs     []string `json:"sourceRefs"`
		ObservedAt     string   `json:"observedAt"`
		Representation string   `json:"representation"`
		Coverage       struct {
			Kind string `json:"kind"`
		} `json:"coverage"`
	} `json:"observation"`
	Mode           string     `json:"mode"`
	Desired        []unit     `json:"desired"`
	Observed       []observed `json:"observed,omitempty"`
	NextCheckpoint checkpoint `json:"nextCheckpoint"`
	Message        string     `json:"message"`
}

func main() {
	var sourcePath string
	flag.StringVar(&sourcePath, "source", "", "source-owned JSON file")
	flag.Parse()
	if sourcePath == "" {
		fatalf("--source is required")
	}
	var request runRequest
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		fatalf("decode run request: %v", err)
	}
	var source sourceFile
	readJSON(sourcePath, &source)
	prior := checkpoint{}
	if len(request.Checkpoint) > 0 && string(request.Checkpoint) != "null" {
		if err := json.Unmarshal(request.Checkpoint, &prior); err != nil {
			fatalf("decode checkpoint: %v", err)
		}
	}
	desired, next, err := translate(source)
	if err != nil {
		fatalf("translate: %v", err)
	}
	result := output{Mode: "reconcile", Desired: desired, Observed: prior.Observed, NextCheckpoint: checkpoint{Observed: next}, Message: "observe file facts"}
	result.Observation.SourceRefs = []string{source.SourceRef}
	result.Observation.ObservedAt = source.CapturedAt
	result.Observation.Representation = "STATE"
	result.Observation.Coverage.Kind = "FULL"
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fatalf("encode output: %v", err)
	}
}

func translate(source sourceFile) ([]unit, []observed, error) {
	if strings.TrimSpace(source.SourceRef) == "" || strings.TrimSpace(source.CapturedAt) == "" {
		return nil, nil, fmt.Errorf("sourceRef and capturedAt are required")
	}
	sort.Slice(source.Facts, func(i, j int) bool { return source.Facts[i].Key < source.Facts[j].Key })
	seen := map[string]struct{}{}
	units := make([]unit, 0, len(source.Facts))
	observations := make([]observed, 0, len(source.Facts))
	for _, item := range source.Facts {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			return nil, nil, fmt.Errorf("fact key is required")
		}
		if _, ok := seen[key]; ok {
			return nil, nil, fmt.Errorf("duplicate fact key %q", key)
		}
		seen[key] = struct{}{}
		address := address{Kind: "Aspect", ObjectID: "FileFact:" + key, AspectName: "observed"}
		value := map[string]any{"key": key, "value": item.Value}
		units = append(units, unit{Address: address, Value: value, SourceKey: source.SourceRef + "#" + key, PathHint: "facts/" + key + ".json"})
		observations = append(observations, observed{Address: address, Digest: canonicalDigest(value)})
	}
	return units, observations, nil
}

func canonicalDigest(value any) string {
	body, _ := json.Marshal(value)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func readJSON(path string, value any) {
	body, err := os.ReadFile(path)
	if err != nil {
		fatalf("read source: %v", err)
	}
	if err := json.Unmarshal(body, value); err != nil {
		fatalf("decode source: %v", err)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
