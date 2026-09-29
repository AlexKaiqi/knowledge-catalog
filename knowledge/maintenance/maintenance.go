// Package maintenance is the public shim for the maintenance walk/scan engine
// (CAP-ORG-01 B11, ruling C): the implementation lives in
// knowledge/internal/maintenanceimpl; cross-capability consumers (index,
// app seam, testkit) keep this stable path.
package maintenance

import "kc/knowledge/internal/maintenanceimpl"

const DefaultScanLimit = maintenanceimpl.DefaultScanLimit

type (
	// FileScopePager pages object ids for one file-scoped walk.
	FileScopePager = maintenanceimpl.FileScopePager
	// ScanPage is one page of a snapshot scan.
	ScanPage = maintenanceimpl.ScanPage
	// ScanRequest describes a scan window.
	ScanRequest = maintenanceimpl.ScanRequest
	// SnapshotScanner is the protocol port a scanner must expose.
	SnapshotScanner = maintenanceimpl.SnapshotScanner
)

var (
	ChangedObjectIDs   = maintenanceimpl.ChangedObjectIDs
	NormalizeScanLimit = maintenanceimpl.NormalizeScanLimit
	RequireScanner     = maintenanceimpl.RequireScanner
	WalkFileScope      = maintenanceimpl.WalkFileScope
	WalkRepository     = maintenanceimpl.WalkRepository
	WalkScope          = maintenanceimpl.WalkScope
	WalkSnapshot       = maintenanceimpl.WalkSnapshot
)
