package main

import (
	"os"

	"kc/cli"
)

func main() {
	os.Exit(cli.RunWorkspaceFS(os.Args[1:], os.Stdout, os.Stderr))
}
