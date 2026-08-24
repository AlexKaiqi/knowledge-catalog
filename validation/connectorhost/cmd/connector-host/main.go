package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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
	if command == "mount" {
		fs := flag.NewFlagSet("mount", flag.ContinueOnError)
		repo := fs.String("repo", "", "user connector repository")
		kcURL := fs.String("kc-url", "http://127.0.0.1:7380", "kc serve URL")
		principal := fs.String("as", "", "connector host principal")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if *repo == "" {
			return fmt.Errorf("mount requires --repo")
		}
		if err := store.SaveConfig(connectorhost.HostConfig{RepoPath: *repo, KCURL: *kcURL, Principal: *principal}); err != nil {
			return err
		}
		return printJSON(map[string]any{"mounted": *repo, "kcUrl": *kcURL, "principal": *principal})
	}
	host, err := connectorhost.OpenHost(store)
	if err != nil {
		return fmt.Errorf("open host (run mount first): %w", err)
	}
	ctx := context.Background()
	switch command {
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
		fmt.Fprintf(os.Stdout, "connector-host\n  repo    %s\n  listen  http://%s/\n", store.Home(), *listen)
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
	return fmt.Errorf("usage: connector-host [--home DIR] mount|list|validate|run|activate|pause|history|serve")
}
