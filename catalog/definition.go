package catalog

import "kc/kernel"

// ViewDefinition is the consumer recipe: which repositories to join, via selectors
// (usually a published branch). Changing it changes the next OpenView resolution.
// Publishers move those branches; consumers do not pin a second serving pointer.

type ViewSource struct {
	Repository kernel.RepositoryID `json:"repository"`
	Selector   string              `json:"selector"`
}

type ViewDefinition struct {
	ViewID   string       `json:"viewId"`
	Revision int          `json:"revision"`
	Sources  []ViewSource `json:"sources"`
	Retired  bool         `json:"retired,omitempty"`
}

func ViewObjectID(viewID string) string { return "view/" + viewID }

func (c *Catalog) DefineView(viewID string, revision int, sources []ViewSource) (ViewDefinition, error) {
	if err := c.ensureWritable(); err != nil {
		return ViewDefinition{}, err
	}
	if existing, ok := c.views[viewID]; ok && existing.Retired {
		return ViewDefinition{}, kernel.Fail(kernel.ErrViewGenerationInvalid, "view %s is retired", viewID)
	}
	seen := map[kernel.RepositoryID]struct{}{}
	for _, src := range sources {
		if _, dup := seen[src.Repository]; dup {
			return ViewDefinition{}, kernel.Fail(kernel.ErrViewGenerationInvalid, "repository %s appears twice", src.Repository)
		}
		seen[src.Repository] = struct{}{}
		if err := c.requireRepository(src.Repository); err != nil {
			return ViewDefinition{}, err
		}
	}
	def := ViewDefinition{ViewID: viewID, Revision: revision, Sources: sources}
	c.views[viewID] = def
	if err := c.persist("define-view " + viewID); err != nil {
		return ViewDefinition{}, err
	}
	return def, nil
}

func (c *Catalog) View(viewID string) (ViewDefinition, error) {
	def, ok := c.views[viewID]
	if !ok {
		return ViewDefinition{}, kernel.Fail(kernel.ErrViewGenerationInvalid, "unknown view %s", viewID)
	}
	return def, nil
}
