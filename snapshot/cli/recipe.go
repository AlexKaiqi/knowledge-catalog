package cli

import (
	"os"

	"kc/kernel"
	"kc/snapshot/dataset"
)

func remoteDatasetInput(flags map[string]any) (dataset.Dataset, error) {
	if flagString(flags, "from-repo") != "" {
		return dataset.Dataset{}, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "remote Workspace definition does not read a server repository recipe; submit explicit sources")
	}
	fileEntries, err := dataset.ParseFileMappings(flagStrings(flags, "file-source"))
	if err != nil {
		return dataset.Dataset{}, err
	}
	var sources []dataset.DatasetSource
	var inherited map[string]string
	if file := flagString(flags, "file"); file != "" {
		if len(fileEntries) > 0 {
			return dataset.Dataset{}, kernel.Fail(kernel.ErrUsageInvalid,
				"--file-source combines with --source only; a recipe file carries file entries in its files section")
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			return dataset.Dataset{}, err
		}
		recipe, err := dataset.ParseDatasetRecipe(raw)
		if err != nil {
			return dataset.Dataset{}, err
		}
		sources, inherited = recipe.Sources(), recipe.Metadata
	} else {
		if items := flagStrings(flags, "source"); len(items) > 0 {
			sources, err = dataset.ParseSourceMappings(items)
			if err != nil {
				return dataset.Dataset{}, err
			}
		}
		sources = append(sources, fileEntries...)
	}
	if len(sources) == 0 {
		return dataset.Dataset{}, kernel.Fail(kernel.ErrUsageInvalid, "at least one --source or --file-source is required")
	}
	metadata, err := datasetMetadata(flags, inherited)
	if err != nil {
		return dataset.Dataset{}, err
	}
	return dataset.Dataset{Sources: sources, Metadata: metadata}, nil
}
