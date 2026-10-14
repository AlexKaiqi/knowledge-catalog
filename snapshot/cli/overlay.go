package cli

import (
	"os"

	"kc/kernel"
	"kc/snapshot/dataset"
)

func clientDatasetOverlay(flags map[string]any) (any, error) {
	file, err := requireRemoteFlag(flags, "file")
	if err != nil {
		return nil, err
	}
	overlayFile, err := requireRemoteFlag(flags, "overlay")
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	recipe, err := dataset.ParseDatasetRecipe(raw)
	if err != nil {
		return nil, err
	}
	raw, err = os.ReadFile(overlayFile)
	if err != nil {
		return nil, err
	}
	overlay, err := dataset.ParseDatasetOverlay(raw)
	if err != nil {
		return nil, err
	}
	definition, err := dataset.MergeOverlay(dataset.Dataset{SetID: recipe.Name, Revision: 1, Sources: recipe.Sources(), Metadata: recipe.Metadata}, overlay)
	if err != nil {
		return nil, err
	}
	if out := flagString(flags, "out"); out != "" {
		merged, ok := dataset.RecipeFromDataset(definition)
		if !ok {
			return nil, kernel.Fail(kernel.ErrDatasetInvalid, "overlay does not produce a portable recipe")
		}
		body, err := dataset.FormatDatasetRecipe(merged)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(out, body, 0o644); err != nil {
			return nil, err
		}
		return map[string]any{"setId": definition.SetID, "out": out}, nil
	}
	return definition, nil
}
