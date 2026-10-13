package cli

import (
	"os"

	inv "kc/invocation"
	"kc/kernel"
	"kc/snapshot/dataset"
)

func clientKnowledgeSetOverlay(flags map[string]FlagValue) (any, error) {
	if err := rejectFlagsOutside(flags, inv.FlagNames("file overlay out help"), "kc dataset overlay"); err != nil {
		return nil, err
	}
	file, err := requireFlag(flags, "file")
	if err != nil {
		return nil, err
	}
	overlayFile, err := requireFlag(flags, "overlay")
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	recipe, err := dataset.ParseKnowledgeSetRecipe(raw)
	if err != nil {
		return nil, err
	}
	raw, err = os.ReadFile(overlayFile)
	if err != nil {
		return nil, err
	}
	overlay, err := dataset.ParseKnowledgeSetOverlay(raw)
	if err != nil {
		return nil, err
	}
	definition, err := dataset.MergeOverlay(dataset.KnowledgeSet{SetID: recipe.Name, Revision: 1, Sources: recipe.Sources(), Metadata: recipe.Metadata}, overlay)
	if err != nil {
		return nil, err
	}
	if out := flagString(flags, "out"); out != "" {
		merged, ok := dataset.RecipeFromKnowledgeSet(definition)
		if !ok {
			return nil, kernel.Fail(kernel.ErrKnowledgeSetInvalid, "overlay does not produce a portable recipe")
		}
		body, err := dataset.FormatKnowledgeSetRecipe(merged)
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
