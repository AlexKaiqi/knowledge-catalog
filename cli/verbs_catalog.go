package cli

import (
	inv "kc/invocation"
)

// Catalog application operations admit repositories and publish Workspace
// recipes. Managed creation delegates allocation and an explicit initial
// grant policy to Home; ordinary attach and Workspace composition do not grant.
// catalog show lists repository ids; README is a knowledge object, not inventory
// title/summary. The catalog/ package still does not read knowledge.

func catalogVerbs() map[string]command {
	return map[string]command{
		"catalog-list":    {Stage: stageHome, Run: inv.CatalogListOperation},
		"show":            {Stage: stageGoverned, Run: inv.ReadCatalogState},
		"dataset-define":  {Stage: stageGoverned, Run: inv.VerbDefineKnowledgeSet},
		"dataset-clone":   {Stage: stageGoverned, Run: inv.VerbDatasetCloneRequiresServer},
		"attach":          {Stage: stageGoverned, Run: inv.VerbRegister},
		"create":          {Stage: stageGoverned, Run: inv.VerbCreateManagedRepository},
		"dataset-retire":  {Stage: stageGoverned, Run: inv.VerbRetireKnowledgeSet},
		"catalog-archive": {Stage: stageGoverned, Run: inv.VerbArchiveCatalog},
		"detach":          {Stage: stageGoverned, Run: inv.VerbDetach},
		"catalog-use":     {Stage: stageHome, Run: inv.CatalogUseOperation},
	}
}
