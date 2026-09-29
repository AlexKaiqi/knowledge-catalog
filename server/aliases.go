package server

// Transport aliases: the server package speaks the same flag, workspace, and
// home vocabulary as the seam and the argv front end. Home assembly lives in
// kc/home; the names here keep moved handlers readable. (CAP-ORG-01 B6.)

import (
	"net/http"

	"kc/catalog"
	kchome "kc/home"
	inv "kc/invocation"
	"kc/invoke"
	"kc/snapshot"
)

type FlagValue = invoke.FlagValue

type command = inv.Command

type Home = kchome.Home

type HomeFile = kchome.HomeFile

var Open = kchome.Open

var ReadHome = kchome.ReadHome

var ReadStores = kchome.ReadStores

var LayoutPath = kchome.LayoutPath

var StoresPath = kchome.StoresPath

var NormalizeIndexDriver = kchome.NormalizeIndexDriver

var OpenCatalogs = kchome.OpenCatalogs

var Ready = kchome.Ready

func homeReady(dir string) bool { return kchome.Ready(dir) }

func layoutPath(dir string) string { return kchome.LayoutPath(dir) }

func storesPath(dir string) string { return kchome.StoresPath(dir) }

func normalizeIndexDriver(raw string) string { return kchome.NormalizeIndexDriver(raw) }

func openCatalogs(dir string, file HomeFile, store *snapshot.Registry) (map[string]*catalog.Catalog, map[string]*catalog.Registry, error) {
	return kchome.OpenCatalogs(dir, file, store)
}

var _ = http.StatusOK

var _ = inv.Command{}
