package cli

const datasetFSHelp = `kcfs plans or mounts a Dataset as a frozen read-only file tree.

Usage:
  kcfs plan  --dataset <id> --root <project>
  kcfs mount --dataset <id> --root <project>

plan resolves the Dataset, checks the file-view inputs, prints a mount manifest,
then exits without mounting. mount prints the same manifest and serves until
SIGINT or SIGTERM. Each directory mapping becomes a read-only FUSE mount below
--root in an existing project; a mountpoint must be absent or empty.

The default --view repository delivers selected original bytes without
interpreting their contents. --view semantic is an application extension with
its own object support requirements. Datasets with per-file entries are not
supported by this file view; use the file gateway instead.

kcfs uses the configured KC Server and the same kc login. It resolves the Dataset
once at plan/mount time and keeps that commit set for the process. The file
gateway checks current authorization; Repository machine credentials stay at
the service.

mount requires Linux, /dev/fuse and fusermount3 (usually the distro fuse3 package).
`
