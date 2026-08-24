package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"kc/validation/connectorhost"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "connector-host: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	global := flag.NewFlagSet("connector-host", flag.ContinueOnError)
	home := global.String("home", ".connector-host", "host state directory")
	if err := global.Parse(args); err != nil {
		return err
	}
	rest := global.Args()
	if len(rest) == 0 {
		return usage()
	}
	store, err := connectorhost.NewStore(*home)
	if err != nil {
		return err
	}
	command := rest[0]
	args = rest[1:]
	if command == "repo-set" {
		fs := flag.NewFlagSet("repo-set", flag.ContinueOnError)
		repo := fs.String("repo", "", "authoritative public Connector Git repository URL or path")
		ref := fs.String("ref", "refs/heads/main", "Git ref synchronized by the execution service")
		syncEvery := fs.String("sync-every", "30s", "repository synchronization interval")
		kcURL := fs.String("kc-url", "http://127.0.0.1:7380", "kc serve URL")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if *repo == "" {
			return fmt.Errorf("repo-set requires --repo")
		}
		if every, err := time.ParseDuration(*syncEvery); err != nil || every <= 0 {
			return fmt.Errorf("--sync-every must be a positive duration")
		}
		config := connectorhost.HostConfig{
			Repository: *repo, Ref: *ref, RepoPath: filepath.Join(store.Home(), "repository"),
			SyncEvery: *syncEvery, KCURL: *kcURL,
		}
		if err := store.SaveConfig(config); err != nil {
			return err
		}
		config, err = store.LoadConfig()
		if err != nil {
			return err
		}
		host := connectorhost.NewHost(store, config, connectorhost.KCClient{BaseURL: *kcURL})
		state := host.Sync(context.Background())
		if state.Error != "" {
			return fmt.Errorf("initial repository sync: %s", state.Error)
		}
		loaded, err := connectorhost.InspectRepository(config.RepoPath)
		if err != nil {
			return err
		}
		invalid := 0
		for _, item := range loaded {
			if item.Error != nil {
				invalid++
			}
		}
		return printJSON(map[string]any{"repository": state, "kcUrl": *kcURL, "syncEvery": *syncEvery, "discoveryPattern": "connectors/*/connector.yaml", "connectors": len(loaded), "invalid": invalid})
	}
	host, err := connectorhost.OpenHost(store)
	if err != nil {
		return fmt.Errorf("open host (run repo-set first): %w", err)
	}
	ctx := context.Background()
	switch command {
	case "sync":
		state := host.Sync(ctx)
		if state.Error != "" {
			if err := printJSON(state); err != nil {
				return err
			}
			return fmt.Errorf("repository sync: %s", state.Error)
		}
		return printJSON(state)
	case "list":
		items, err := host.Connectors(ctx, false)
		if err != nil {
			return err
		}
		return printJSON(items)
	case "validate":
		id, _, err := connectorFlag(args)
		if err != nil {
			return err
		}
		loaded, err := host.Connector(id)
		if err != nil {
			return err
		}
		if err := connectorhost.ValidateConnector(ctx, loaded); err != nil {
			return err
		}
		return printJSON(map[string]any{"valid": true, "connectorId": id, "generation": loaded.Generation})
	case "run":
		id, preview, err := connectorFlag(args)
		if err != nil {
			return err
		}
		record, err := host.Run(ctx, id, connectorhost.RunTrigger{Kind: "manual"}, preview, false)
		if printErr := printJSON(record); printErr != nil {
			return printErr
		}
		return err
	case "activate":
		id, _, err := connectorFlag(args)
		if err != nil {
			return err
		}
		state, err := host.Activate(ctx, id)
		if err != nil {
			return err
		}
		return printJSON(state)
	case "pause":
		id, _, err := connectorFlag(args)
		if err != nil {
			return err
		}
		state, err := host.Pause(id)
		if err != nil {
			return err
		}
		return printJSON(state)
	case "history":
		id, _, err := connectorFlag(args)
		if err != nil {
			return err
		}
		runs, err := store.Runs(id, 100)
		if err != nil {
			return err
		}
		return printJSON(runs)
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		listen := fs.String("listen", "127.0.0.1:7480", "listen address")
		if err := fs.Parse(args); err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		state := host.Sync(ctx)
		if state.Error != "" {
			return fmt.Errorf("initial repository sync: %s", state.Error)
		}
		fmt.Fprintf(os.Stdout, "connector-host\n  repo    %s\n  commit  %s\n  listen  http://%s/\n", state.Repository, state.Commit, *listen)
		return connectorhost.Serve(ctx, host, *listen)
	default:
		return usage()
	}
}

func connectorFlag(args []string) (string, bool, error) {
	fs := flag.NewFlagSet("connector", flag.ContinueOnError)
	id := fs.String("connector", "", "connector id")
	preview := fs.Bool("preview", false, "preview without committing or advancing checkpoint")
	if err := fs.Parse(args); err != nil {
		return "", false, err
	}
	if *id == "" {
		return "", false, fmt.Errorf("--connector is required")
	}
	return *id, *preview, nil
}

func printJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func usage() error {
	return fmt.Errorf("usage: connector-host [--home DIR] repo-set|sync|list|validate|run|activate|pause|history|serve")
}
