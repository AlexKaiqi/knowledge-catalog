/**
 * Directory semantics over a flat path listing. Loom's vfs-list returns
 * every raw file path in the composed tree (docs/COMPOSITION.md) — there is
 * no separate "directory" object anywhere upstream, git trees do not have
 * empty directories either. This module is the pure logic that turns that
 * flat listing into the directory-shaped tree ctx.fs's stat/listDir expect,
 * kept framework-free so it is unit-testable without a live server.
 */

import { existsSync, statSync } from 'node:fs';
import posix from 'node:path/posix';

function isExistingDir(path: string): boolean {
  try {
    return existsSync(path) && statSync(path).isDirectory();
  } catch {
    return false;
  }
}

/** dsh-tool-fs and skill-filesystem pass the agent's session cwd, which on a
 * real dsh process is the host working directory (an existing absolute path).
 * That is not a Loom virtual path — the composed tree's root is "/". */
function virtualBase(cwd?: string): string {
  if (!cwd) return '/';
  if (posix.isAbsolute(cwd) && isExistingDir(cwd)) return '/';
  return posix.resolve('/', cwd);
}

/** skill-filesystem probes `.dsh/skills` as `join(hostCwd, ".dsh/skills")`.
 * Strip that host prefix so the probe lands on the virtual tree. */
function underHostCwd(path: string, cwd?: string): string | undefined {
  if (!cwd || !posix.isAbsolute(path) || !posix.isAbsolute(cwd) || !isExistingDir(cwd)) {
    return undefined;
  }
  const cwdN = posix.normalize(cwd);
  const pathN = posix.normalize(path);
  if (pathN === cwdN) return '';
  const prefix = cwdN.endsWith('/') ? cwdN : `${cwdN}/`;
  if (!pathN.startsWith(prefix)) return undefined;
  return pathN.slice(prefix.length);
}

/** Normalizes a caller-supplied path into Loom's canonical form: no leading
 * or trailing slash, "." collapsed, ".." resolved — "" means root. cwd (when
 * given) is joined first, matching dsh-fs's resolve(path, {cwd}) contract. */
export function normalizePath(path: string, cwd?: string): string {
  const hosted = underHostCwd(path, cwd);
  const effective = hosted !== undefined ? hosted : path;
  const base = virtualBase(cwd);
  const joined = posix.isAbsolute(effective) ? effective : posix.join(base, effective);
  const resolved = posix.resolve('/', joined);
  return resolved === '/' ? '' : resolved.slice(1);
}

export function dirname(path: string): string {
  if (path === '') return '';
  const parent = posix.dirname(path);
  return parent === '.' ? '' : parent;
}

export function basename(path: string): string {
  return posix.basename(path);
}

export function joinPath(parent: string, name: string): string {
  return parent === '' ? name : `${parent}/${name}`;
}

/** True when child is dirPath itself or nested under it. */
export function isUnder(dirPath: string, child: string): boolean {
  if (dirPath === '') return true;
  return child === dirPath || child.startsWith(`${dirPath}/`);
}

export type TreeEntry = { name: string; type: 'file' | 'directory' };

/** Direct children of dirPath among paths: a path exactly one segment below
 * dirPath is a file entry; a path with more segments below it contributes a
 * (deduplicated) directory entry for its first segment. Order is stable
 * (insertion order of first appearance), sorted by name for determinism. */
export function directChildren(paths: string[], dirPath: string): TreeEntry[] {
  const files = new Set<string>();
  const dirs = new Set<string>();
  for (const path of paths) {
    if (!isUnder(dirPath, path)) continue;
    const rel = dirPath === '' ? path : path.slice(dirPath.length + 1);
    if (rel === '') continue; // dirPath itself is not its own child
    const slash = rel.indexOf('/');
    if (slash === -1) {
      files.add(rel);
    } else {
      dirs.add(rel.slice(0, slash));
    }
  }
  const entries: TreeEntry[] = [];
  for (const name of files) entries.push({ name, type: 'file' });
  for (const name of dirs) entries.push({ name, type: 'directory' });
  entries.sort((a, b) => a.name.localeCompare(b.name));
  return entries;
}

/** Whether dirPath exists as a directory in paths: root always does (even
 * with zero files, it is the tree's own existence), any other path needs at
 * least one entry strictly nested under it. */
export function directoryExists(paths: string[], dirPath: string): boolean {
  if (dirPath === '') return true;
  const prefix = `${dirPath}/`;
  return paths.some((p) => p.startsWith(prefix));
}
