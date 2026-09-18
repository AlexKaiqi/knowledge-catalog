package cli

const datasetFSHelp = `kcfs mounts a Knowledge Catalog Dataset into an existing Linux project as a frozen read-only tree.

Usage:
  kcfs plan  --dataset <id> --root <project>
  kcfs mount --dataset <id> --root <project>

Each Dataset source Path becomes an independent read-only FUSE mount below
--root. kcfs always uses the typed Knowledge Set File Gateway and never receives
Repository machine credentials. It resolves the Dataset once at plan/mount time
and keeps that commit set for the process. It prints the mount manifest, then
serves until SIGINT or SIGTERM. A mountpoint must be absent or empty.

Linux requirements: /dev/fuse and fusermount3 (usually the distro fuse3 package).
`
