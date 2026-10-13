package cli

import (
	"os"

	inv "kc/invocation"
	"kc/kernel"
	"kc/snapshot/dataset"
)

func remoteDatasetInput(flags map[string]FlagValue) (dataset.KnowledgeSet, error) {
	if flagString(flags, "from-repo") != "" {
		return dataset.KnowledgeSet{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote Workspace definition does not read a server repository recipe; submit explicit sources")
	}
	fileEntries, err := inv.FileSourcesFrom(flagStrings(flags, "file-source"))
	if err != nil {
		return dataset.KnowledgeSet{}, err
	}
	var sources []dataset.KnowledgeSetSource
	var inherited map[string]string
	if file := flagString(flags, "file"); file != "" {
		if len(fileEntries) > 0 {
			return dataset.KnowledgeSet{}, kernel.Fail(kernel.ErrUsageInvalid,
				"--file-source combines with --source only; a recipe file carries file entries in its files section")
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			return dataset.KnowledgeSet{}, err
		}
		recipe, err := dataset.ParseKnowledgeSetRecipe(raw)
		if err != nil {
			return dataset.KnowledgeSet{}, err
		}
		sources, inherited = recipe.Sources(), recipe.Metadata
	} else {
		if items := flagStrings(flags, "source"); len(items) > 0 {
			sources, err = inv.WorkspaceSourcesFrom(items)
			if err != nil {
				return dataset.KnowledgeSet{}, err
			}
		}
		sources = append(sources, fileEntries...)
	}
	if len(sources) == 0 {
		return dataset.KnowledgeSet{}, kernel.Fail(kernel.ErrUsageInvalid, "at least one --source or --file-source is required")
	}
	metadata, err := inv.DatasetMetadata(flags, inherited)
	if err != nil {
		return dataset.KnowledgeSet{}, err
	}
	return dataset.KnowledgeSet{Sources: sources, Metadata: metadata}, nil
}
