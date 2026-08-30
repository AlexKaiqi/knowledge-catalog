package cli

const workspaceFSHelp = `kcfs mounts a fixed Knowledge Catalog Workspace pin into an existing Linux project.

Usage:
  kcfs plan  --home <dir> [--catalog <id>] --workspace <id> [--pin <file>] --root <project>
  kcfs mount --home <dir> [--catalog <id>] --workspace <id> [--pin <file>] --root <project>
  kcfs mount --server <url> [--catalog <id>] --workspace <id> [--pin <file>] --as <principal> --root <project>

Each Workspace source Path becomes an independent read-only FUSE mount below
--root. Remote mode uses the typed Workspace File Gateway and never receives
Repository machine credentials. Without --pin the process resolves selectors once; with --pin it replays
the supplied ResolvedWorkspace. It prints the pin and mount manifest, then serves
until SIGINT or SIGTERM. A mountpoint must be absent or empty.

Linux requirements: /dev/fuse and fusermount3 (usually the distro fuse3 package).
`
