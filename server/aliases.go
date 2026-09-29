package server

// Transport aliases: the server package speaks the same flag, workspace, and
// home vocabulary as the seam and the argv front end. Home assembly lives in
// kc/home; the names here keep moved handlers readable. (CAP-ORG-01 B6.)

import (
	"net/http"

	kchome "kc/home"
	inv "kc/invocation"
	"kc/invoke"
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

var _ = http.StatusOK

var _ = inv.Command{}
