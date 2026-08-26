package main

import (
	"context"
	"os"
	"time"

	"kc/cli"
	"kc/internal/telemetry"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	if parsed, err := cli.ParseArgs(argv); err == nil && parsed.Command == "serve" {
		result := cli.Run(argv)
		_, _ = os.Stdout.WriteString(result.Stdout)
		return result.Status
	}
	runtime, err := telemetry.New(telemetry.Config{ServiceName: "kc-cli", EnableOTLP: true})
	if err != nil {
		result := cli.Run(argv)
		_, _ = os.Stdout.WriteString(result.Stdout)
		return result.Status
	}
	if runtime.StartupError() != nil {
		_, _ = os.Stderr.WriteString("kc telemetry: optional OTLP trace exporter disabled\n")
	}
	result := cli.RunWithTelemetry(argv, runtime)
	_, _ = os.Stdout.WriteString(result.Stdout)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = runtime.Shutdown(ctx)
	return result.Status
}
