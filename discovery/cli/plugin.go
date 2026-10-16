package cli

import (
	"context"
	"os"
	"path/filepath"
	"unicode/utf8"

	"kc/discovery/client"
	"kc/kernel"
)

func validatePlugin(flags map[string]any) error {
	if _, err := required(flags, "name"); err != nil {
		return err
	}
	task := flagString(flags, "task")
	if task != "help" && task != "describe" && task != "trial" {
		return usage("plugin task must be help, describe or trial")
	}
	for _, name := range words("repo commit ref file") {
		if task != "trial" && flagString(flags, name) != "" {
			return usage("plugin %s does not accept --%s", task, name)
		}
	}
	if task == "trial" {
		for _, name := range words("repo file") {
			if _, err := required(flags, name); err != nil {
				return err
			}
		}
		if flagString(flags, "commit") != "" && flagString(flags, "ref") != "" {
			return usage("trial chooses --commit or --ref")
		}
	}
	return nil
}

func (e *executor) executePlugin(ctx context.Context, flags map[string]any, options client.RequestOptions) (any, error) {
	var output any
	name := flagString(flags, "name")
	var err error
	switch flagString(flags, "task") {
	case "help":
		err = e.client.PluginHelp(ctx, name, options, &output)
	case "describe":
		err = e.client.PluginCapabilities(ctx, name, options, &output)
	case "trial":
		path := flagString(flags, "file")
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "cannot read declaration file: %s", readErr)
		}
		// JSON transport would replace invalid bytes and change what is judged.
		if !utf8.Valid(content) {
			return nil, kernel.Fail(kernel.ErrUsageInvalid, "declaration file %s is not valid UTF-8", filepath.Base(path))
		}
		request := client.DeclarationTrialRequest{Repository: kernel.RepositoryID(flagString(flags, "repo")), Commit: kernel.CommitID(flagString(flags, "commit")), Ref: flagString(flags, "ref"), Documents: []client.DeclarationDocument{{Path: filepath.Base(path), Content: string(content)}}}
		err = e.client.TrialDeclarations(ctx, name, request, options, &output)
	}
	return output, err
}
