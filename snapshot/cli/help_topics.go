package cli

import "maps"

// HelpTopics contributes static group introductions and concept pages. Keys
// belong to the help namespace, not the business command registry. Callers
// combine group introductions with the leaves declared by Commands().
func HelpTopics() map[string]string {
	return maps.Clone(helpTopics)
}

var helpTopics = map[string]string{
	"snapshot":    guideSnapshot,
	"repository":  guideRepository,
	"versions":    guideVersions,
	"selector":    guideVersions,
	"ref":         guideVersions,
	"commit":      guideVersions,
	"pin":         guidePin,
	"recipe":      guideRecipe,
	"metadata":    guideMetadata,
	"composition": guideComposition,
	"dataset":     guideDataset,
	"files":       guideFiles,
	"grant":       guideGrant,
	"registry":    guideRegistry,
}
