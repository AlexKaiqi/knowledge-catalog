package home

import (
	"slices"
	"sync"
)

// Home.File remains the construction snapshot. Dynamic bindings have their
// own lock, so ReadView never copies a struct concurrently being mutated.
type homeInventory struct {
	mu       sync.RWMutex
	bindings map[string]HomeRepo
}

func (ws *Home) BindingFile() HomeFile {
	file := ws.File
	file.Catalogs = append([]HomeCatalog(nil), file.Catalogs...)
	if ws.inventory == nil {
		file.Repos = append([]HomeRepo(nil), file.Repos...)
		return file
	}
	ws.inventory.mu.RLock()
	defer ws.inventory.mu.RUnlock()
	file.Repos = make([]HomeRepo, 0, len(ws.inventory.bindings))
	for _, binding := range ws.inventory.bindings {
		file.Repos = append(file.Repos, binding)
	}
	slices.SortFunc(file.Repos, func(a, b HomeRepo) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return file
}
