package main

import (
	"os"

	"kc/cli"
)

func main() {
	result := cli.Run(os.Args[1:])
	_, _ = os.Stdout.WriteString(result.Stdout)
	os.Exit(result.Status)
}
