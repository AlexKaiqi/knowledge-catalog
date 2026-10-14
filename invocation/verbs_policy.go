package invocation

import (
	"fmt"
	kchome "kc/home"
	"kc/hook"
)

func VerbHookAdd(cx *Invocation) (any, error) {
	on, err := cx.Require("on")
	if err != nil {
		return nil, err
	}
	phase, err := cx.Require("phase")
	if err != nil {
		return nil, err
	}
	binding := hook.Binding{
		On:    on,
		Phase: phase,
		Repo:  cx.Flag("repo"),
		Run:   cx.Flag("run"),
		URL:   cx.Flag("url"),
	}
	if err := hook.ValidateBinding(binding); err != nil {
		return nil, err
	}
	file, err := hook.Read(cx.Home)
	if err != nil {
		return nil, err
	}
	binding.ID = hook.NextID(file.Bindings)
	file.Bindings = append(file.Bindings, binding)
	if err := hook.Write(cx.Home, file); err != nil {
		return nil, err
	}
	return binding, nil
}

func VerbHookLs(cx *Invocation) (any, error) {
	file, err := hook.Read(cx.Home)
	if err != nil {
		return nil, err
	}
	scope := cx.BindingScope()
	out := []hook.Binding{}
	for _, b := range file.Bindings {
		if scope.Matches(b.On, b.Repo) {
			out = append(out, b)
		}
	}
	return map[string]any{"bindings": out}, nil
}

func VerbHookRm(cx *Invocation) (any, error) {
	id, err := cx.Require("id")
	if err != nil {
		return nil, err
	}
	file, err := hook.Read(cx.Home)
	if err != nil {
		return nil, err
	}
	kept, found := DropByID(file.Bindings, id, func(b hook.Binding) string { return b.ID })
	if !found {
		return nil, fmt.Errorf("unknown hook %s", id)
	}
	file.Bindings = kept
	if err := hook.Write(cx.Home, file); err != nil {
		return nil, err
	}
	return map[string]any{"revoked": id}, nil
}

// VerbObserveStores is the console/HTTP observation of ⓪ Snapshot and ③
// retrieval bindings. It is not store-ls: layout directories and secret
// environment names stay off the public surface (core-architecture secrets rule).
func VerbObserveStores(cx *Invocation) (any, error) {
	var registry *StoreBinding
	var repos []StoreBinding
	if cx.WS != nil {
		if cx.WS.Deployment != nil {
			{
				item := cx.WS.Deployment.Registry
				registry = &StoreBinding{item.ID, item.Driver, item.DSN}
			}
			for _, item := range cx.WS.Deployment.Repositories {
				repos = append(repos, StoreBinding{item.ID, item.Driver, item.DSN})
			}
		}
		return ObservePublicStores(cx.WS.Stores, cx.WS.File, registry, repos), nil
	}
	if !kchome.Ready(cx.Home) {
		return nil, kchome.Missing(cx.Home)
	}
	stores, err := ReadStores(cx.Home)
	if err != nil {
		return nil, err
	}
	file, err := kchome.ReadHome(cx.Home)
	if err != nil {
		return nil, err
	}
	return ObservePublicStores(stores, file, nil, nil), nil
}

func VerbCheckDataset(cx *Invocation) (any, error) {
	cat, err := PickDatasets(cx.WS, cx.Flags)
	if err != nil {
		return nil, err
	}
	setID, err := cx.SetID()
	if err != nil {
		return nil, err
	}
	resolved, err := ResolveOrReplay(cx.WS, cx.Home, cat, setID, cx.Flags)
	if err != nil {
		return nil, err
	}
	return cat.CheckResolved(resolved), nil
}
