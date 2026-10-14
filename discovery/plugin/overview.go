package plugin

import (
	"sort"

	"kc/kernel"
	"kc/object/reader"
)

// Overview uses the already authorized fixed source map; it does not query a
// plugin, refresh metadata, call an external source or claim current readiness.
func (p *IndexPlugins) Overview(pin reader.KnowledgeSetPin) ([]SourcePlugin, error) {
	out := []SourcePlugin{}
	if p == nil {
		return out, nil
	}
	ids := make([]kernel.RepositoryID, 0, len(pin.Repositories))
	for id := range pin.Repositories {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if p.unavailable[id] != nil {
			return nil, p.unavailable[id]
		}
		instance := p.byRepo[id]
		if instance == nil {
			continue
		}
		files := []string{}
		for _, item := range pin.Items {
			if item.Repository != id {
				continue
			}
			file := item.File
			if file == "" {
				file = item.Prefix
			}
			files = append(files, file)
		}
		explanation, err := p.Help(instance.selection.Name)
		item := SourcePlugin{Repository: id, Commit: pin.Repositories[id], Files: files, Name: instance.selection.Name, Instance: instance.selection.Instance}
		if err == nil {
			item.Summary, item.Help = explanation.Summary, "kc plugin "+explanation.Name+" help"
		}
		out = append(out, item)
	}
	return out, nil
}
