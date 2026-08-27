package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"kc/connector"
	"kc/kernel"
)

type manifest struct {
	Metadata struct {
		ID string `yaml:"id"`
	} `yaml:"metadata"`
	Spec struct {
		Target struct {
			Repository kernel.RepositoryID `yaml:"repository"`
			Ref        string              `yaml:"ref"`
			Scope      struct {
				Aspects      []string `yaml:"aspects"`
				AllowEntity  bool     `yaml:"allowEntity"`
				ObjectPrefix string   `yaml:"objectPrefix"`
			} `yaml:"scope"`
		} `yaml:"target"`
	} `yaml:"spec"`
}

type observation struct {
	Observation struct {
		SourceRefs []string `json:"sourceRefs"`
		ObservedAt string   `json:"observedAt"`
	} `json:"observation"`
	Mode     connector.Mode       `json:"mode"`
	Desired  []connector.Unit     `json:"desired"`
	Observed []connector.Observed `json:"observed"`
	Message  string               `json:"message"`
}

func main() {
	var manifestPath, observationPath, outputPath, baseCommit string
	flag.StringVar(&manifestPath, "manifest", "", "Connector manifest YAML")
	flag.StringVar(&observationPath, "observation", "", "Collector observation JSON")
	flag.StringVar(&outputPath, "out", "", "Preview JSON")
	flag.StringVar(&baseCommit, "base", "", "Pinned target commit")
	flag.Parse()
	if manifestPath == "" || observationPath == "" || outputPath == "" || baseCommit == "" {
		fatal("--manifest, --observation, --base and --out are required")
	}

	var connectorManifest manifest
	decodeYAML(manifestPath, &connectorManifest)
	var collected observation
	decodeJSON(observationPath, &collected)
	preview, err := connector.Preview(connector.Plan{
		ConnectorID:      connectorManifest.Metadata.ID,
		Mode:             collected.Mode,
		Scope: connector.Scope{
			Aspects:      connectorManifest.Spec.Target.Scope.Aspects,
			AllowEntity:  connectorManifest.Spec.Target.Scope.AllowEntity,
			ObjectPrefix: connectorManifest.Spec.Target.Scope.ObjectPrefix,
		},
		TargetRepository: connectorManifest.Spec.Target.Repository,
		TargetRef:        connectorManifest.Spec.Target.Ref,
		BaseCommit:       kernel.CommitID(baseCommit),
		Desired:          collected.Desired,
		Observed:         collected.Observed,
		SourceRefs:       collected.Observation.SourceRefs,
		ProducedAt:       collected.Observation.ObservedAt,
		ActorRef:         "connector/" + connectorManifest.Metadata.ID,
		Message:          collected.Message,
	})
	if err != nil {
		fatal(err.Error())
	}
	encoded, err := json.MarshalIndent(preview, "", "  ")
	if err != nil {
		fatal(err.Error())
	}
	if err := os.WriteFile(outputPath, append(encoded, '\n'), 0o644); err != nil {
		fatal(err.Error())
	}
}

func decodeYAML(path string, value any) {
	body, err := os.ReadFile(path)
	if err != nil {
		fatal(err.Error())
	}
	if err := yaml.Unmarshal(body, value); err != nil {
		fatal(err.Error())
	}
}

func decodeJSON(path string, value any) {
	body, err := os.ReadFile(path)
	if err != nil {
		fatal(err.Error())
	}
	if err := json.Unmarshal(body, value); err != nil {
		fatal(err.Error())
	}
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
