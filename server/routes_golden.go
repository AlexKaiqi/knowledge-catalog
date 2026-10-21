package server

import (
	_ "embed"
	"sort"
	"strings"
)

// routesGolden is the exported golden manifest of the closed HTTP surface:
// one "METHOD /pattern" entry per line, in lexical order. scripts and tests
// outside this package (validation-inventory, the CLI evidence inventory)
// read this file so the route surface has exactly one owner. Changing a route
// means changing routes.go, the registrations, and this manifest together;
// routes_anchor_test.go fails when any of them drifts.
//
//go:embed routes_golden.txt
var routesGolden string

// GoldenRoutes returns the golden manifest entries as sorted
// "METHOD /pattern" strings. It is the exported form of the closed set for
// callers that cannot import this package.
func GoldenRoutes() []string {
	entries := strings.Split(strings.TrimSuffix(routesGolden, "\n"), "\n")
	sort.Strings(entries)
	return entries
}
