package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"kc/cli"
	"kc/client"
	"kc/integrationruntime"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" {
		_, err := fmt.Fprintln(out, "kc-integration build|activate|run|daemon|pause|resume|status --state-dir <absolute-dir> [--file artifact-or-manifest.json] [--id integration] [--server URL] [--as local-principal]\nUses the same kc login. Credentials remain references to the runtime environment. Pausing stops subsequent pulls; an in-flight Writer command keeps its original identity.")
		return err
	}
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	home, _ := os.UserHomeDir()
	directory := flags.String("state-dir", filepath.Join(home, ".local", "state", "kc", "integrations"), "durable runtime state")
	file := flags.String("file", "", "artifact or integration manifest JSON")
	id := flags.String("id", "", "integration id")
	server := flags.String("server", "", "KC endpoint")
	principal := flags.String("as", "", "local test identity")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	factory := func(ctx context.Context, server string) (integrationruntime.Writer, string, error) {
		kc, err := cli.NewSessionClient(ctx, server, *principal)
		if err != nil {
			return nil, "", err
		}
		identity, err := kc.IdentityService().WhoAmI(ctx, client.RequestOptions{})
		if err != nil {
			return nil, "", err
		}
		return integrationruntime.ClientWriter{Client: kc}, identity.Principal, nil
	}
	runtime, err := integrationruntime.New(*directory, factory)
	if err != nil {
		return err
	}
	encode := func(value any) error { return json.NewEncoder(out).Encode(value) }
	switch command {
	case "build":
		var spec integrationruntime.ArtifactSpec
		if err := readJSON(*file, &spec); err != nil {
			return err
		}
		value, err := runtime.Build(ctx, spec)
		if err != nil {
			return err
		}
		return encode(value)
	case "activate":
		var manifest integrationruntime.Manifest
		if err := readJSON(*file, &manifest); err != nil {
			return err
		}
		if manifest.Server == "" {
			manifest.Server = cli.ClientServerURL(*server)
		}
		value, err := runtime.Activate(ctx, manifest)
		if err != nil {
			return err
		}
		return encode(value)
	case "run":
		value, err := runtime.Run(ctx, *id)
		if err != nil {
			return err
		}
		return encode(value)
	case "pause", "resume":
		value, err := runtime.Pause(ctx, *id, command == "pause")
		if err != nil {
			return err
		}
		return encode(value)
	case "status":
		value, err := runtime.Status(*id)
		if err != nil {
			return err
		}
		return encode(value)
	case "daemon":
		for {
			values, tickErr := runtime.Tick(ctx)
			for _, value := range values {
				if err := encode(value); err != nil {
					return err
				}
			}
			if tickErr != nil && len(values) == 0 && ctx.Err() == nil {
				return tickErr
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
			}
		}
	default:
		return fmt.Errorf("unknown integration command %q", command)
	}
}

func readJSON(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return fmt.Errorf("manifest must contain one JSON document")
	}
	return nil
}
