/**
 * Model-facing discovery over the same composite project tree used by
 * LoomFileSystem. DSH's stock glob/grep shells out to ripgrep and therefore
 * cannot see an unmaterialized mount. These replacements search local files
 * directly and remote files through the task's pinned LoomVfs client.
 */

import type { Context } from '@deepseek-ai/cordis';
import { readdir, readFile } from 'node:fs/promises';
import nodePath from 'node:path';
import { LoomVfs, type LoomFileEntry, type LoomVfsConfig } from './client.js';
import { readWorkspaceBinding } from './binding.js';
import { vfsForTask } from './session-vfs.js';
import { directChildren, joinPath, normalizePath } from './tree.js';

export const name = 'loom-search';
export const inject = ['tools'];

type JsonSchema = Record<string, unknown>;

interface ToolRunContext {
  signal: AbortSignal;
  agent?: { session: { header: { cwd?: string } } };
}

interface ToolDefinition {
  name: string;
  description: string;
  parameters: JsonSchema;
  output: {
    schema: JsonSchema;
    render(args: unknown, value: unknown): Array<{ type: 'text'; text: string }>;
  };
  execute(args: unknown, exec: ToolRunContext): Promise<unknown>;
  isConcurrencySafe?(args: unknown): boolean;
}

interface ToolRegistry {
  register(definition: ToolDefinition): () => void;
}

const MAX_GLOB_RESULTS = 100;
const MAX_GREP_MATCHES = 250;
const MAX_SEARCH_FILES = 5000;
const MAX_SEARCH_BYTES = 20_000_000;
const LOCAL_IGNORED_DIRECTORIES = new Set(['.git', '.kc', 'node_modules']);

type SearchConfig = LoomVfsConfig & { home?: string; mountPath?: string };

interface OverlayEntry extends LoomFileEntry {
  source: 'local' | 'remote';
  sourcePath: string;
}

function abortIfNeeded(signal: AbortSignal): void {
  if (signal.aborted) throw new DOMException('workspace search aborted', 'AbortError');
}

function escapeRegex(char: string): string {
  return /[\\^$.*+?()[\]{}|]/.test(char) ? `\\${char}` : char;
}

/** A bounded, dependency-free glob compiler for the MVP path vocabulary. */
export function globRegex(pattern: string): RegExp {
  let out = '^';
  for (let i = 0; i < pattern.length; i += 1) {
    const char = pattern[i];
    if (char === '*') {
      if (pattern[i + 1] === '*') {
        i += 1;
        if (pattern[i + 1] === '/') {
          i += 1;
          out += '(?:.*/)?';
        } else {
          out += '.*';
        }
      } else {
        out += '[^/]*';
      }
    } else if (char === '?') {
      out += '[^/]';
    } else if (char === '{') {
      const end = pattern.indexOf('}', i + 1);
      if (end > i + 1) {
        const alternatives = pattern
          .slice(i + 1, end)
          .split(',')
          .map((part) => part.split('').map(escapeRegex).join(''));
        out += `(?:${alternatives.join('|')})`;
        i = end;
      } else {
        out += '\\{';
      }
    } else {
      out += escapeRegex(char);
    }
  }
  return new RegExp(`${out}$`);
}

function relativeToRoot(path: string, root: string): string | undefined {
  if (!root) return path;
  if (path === root) return '';
  const prefix = `${root}/`;
  return path.startsWith(prefix) ? path.slice(prefix.length) : undefined;
}

export function matchGlob(entries: LoomFileEntry[], pattern: string, root = ''): string[] {
  const matcher = globRegex(pattern);
  const basenameOnly = !pattern.includes('/');
  const matched: string[] = [];
  for (const entry of entries) {
    const relative = relativeToRoot(entry.path, root);
    if (relative === undefined || relative === '') continue;
    const candidate = basenameOnly ? relative.slice(relative.lastIndexOf('/') + 1) : relative;
    if (matcher.test(candidate)) matched.push(entry.path);
  }
  return matched.sort();
}

function parseObject(value: unknown): Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new Error('tool arguments must be an object');
  }
  return value as Record<string, unknown>;
}

function requiredString(args: Record<string, unknown>, key: string): string {
  const value = args[key];
  if (typeof value !== 'string' || !value.trim()) throw new Error(`${key} must be a non-empty string`);
  return value;
}

function optionalString(args: Record<string, unknown>, key: string): string | undefined {
  const value = args[key];
  if (value === undefined) return undefined;
  if (typeof value !== 'string') throw new Error(`${key} must be a string`);
  return value;
}

function normalizedRoot(path: string | undefined): string {
  if (!path || path === '/') return '';
  return normalizePath(path);
}

function configuredMountPath(config: SearchConfig): string | undefined {
  const raw = config.mountPath;
  if (raw === undefined || raw === '') return undefined;
  const normalized = raw.replace(/^\/+|\/+$/g, '');
  if (!normalized || normalized.includes('/') || normalized.includes('\\')) {
    throw new Error('dsh-loom: mountPath must be one project-relative directory name');
  }
  return normalized;
}

function projectPath(raw: string | undefined, cwd: string): string {
  if (!raw || raw === '/') return '';
  if (nodePath.isAbsolute(raw)) {
    const absolute = nodePath.resolve(raw);
    const root = nodePath.resolve(cwd);
    if (absolute === root) return '';
    const prefix = `${root}${nodePath.sep}`;
    if (absolute.startsWith(prefix)) return normalizePath(absolute.slice(prefix.length));
  }
  return normalizePath(raw.replace(/^[/\\]+/, ''));
}

function localAbsolute(cwd: string, relative: string): string {
  const root = nodePath.resolve(cwd);
  const target = nodePath.resolve(root, relative || '.');
  if (target !== root && !target.startsWith(`${root}${nodePath.sep}`)) {
    throw new Error(`path escapes the Agent project: ${relative}`);
  }
  return target;
}

async function walkLocal(cwd: string, mountPath: string): Promise<OverlayEntry[]> {
  const entries: OverlayEntry[] = [];
  const visit = async (relative: string): Promise<void> => {
    const children = await readdir(localAbsolute(cwd, relative), { withFileTypes: true });
    for (const child of children) {
      const childPath = joinPath(relative, child.name);
      if (relative === '' && child.name === mountPath) {
        throw new Error(`cannot mount remote knowledge at "${mountPath}": the project already contains that path`);
      }
      if (child.isDirectory()) {
        if (LOCAL_IGNORED_DIRECTORIES.has(child.name)) continue;
        await visit(childPath);
      } else if (child.isFile()) {
        entries.push({ path: childPath, repository: 'local-project', commit: 'live', source: 'local', sourcePath: childPath });
      }
    }
  };
  await visit('');
  return entries;
}

async function overlayEntries(vfs: LoomVfs, cwd: string, mountPath: string): Promise<OverlayEntry[]> {
  const local = await walkLocal(cwd, mountPath);
  const remote = (await vfs.list()).map((entry) => ({
    ...entry,
    path: joinPath(mountPath, entry.path),
    source: 'remote' as const,
    sourcePath: entry.path,
  }));
  return [...local, ...remote];
}

async function readOverlayEntry(vfs: LoomVfs, cwd: string, entry: OverlayEntry): Promise<Uint8Array> {
  if (entry.source === 'remote') return (await vfs.read(entry.sourcePath)).content;
  return new Uint8Array(await readFile(localAbsolute(cwd, entry.sourcePath)));
}

async function grepOverlay(
  vfs: LoomVfs,
  cwd: string,
  entries: OverlayEntry[],
  pattern: string,
  root: string,
  include: string | undefined,
  signal: AbortSignal,
): Promise<string> {
  const matcher = compileContentPattern(pattern);
  const includeMatcher = include ? globRegex(include) : undefined;
  const includeBasenameOnly = include !== undefined && !include.includes('/');
  const selected = entries.filter((entry) => {
    if (!root) return true;
    return entry.path === root || entry.path.startsWith(`${root}/`);
  });
  if (selected.length > MAX_SEARCH_FILES) throw new Error(`workspace search spans ${selected.length} files; narrow path or include`);
  const blocks: string[] = [];
  let matches = 0;
  let bytes = 0;
  for (const entry of selected.sort((a, b) => a.path.localeCompare(b.path))) {
    abortIfNeeded(signal);
    if (includeMatcher) {
      const candidate = includeBasenameOnly ? entry.path.slice(entry.path.lastIndexOf('/') + 1) : entry.path;
      if (!includeMatcher.test(candidate)) continue;
    }
    const content = await readOverlayEntry(vfs, cwd, entry);
    bytes += content.byteLength;
    if (bytes > MAX_SEARCH_BYTES) throw new Error('workspace grep exceeded 20 MB; narrow path or include');
    const text = new TextDecoder('utf-8', { fatal: false }).decode(content);
    if (text.includes('\0')) continue;
    const lines: string[] = [];
    text.split(/\r?\n/).forEach((line, index) => {
      if (matches >= MAX_GREP_MATCHES) return;
      matcher.lastIndex = 0;
      if (matcher.test(line)) {
        lines.push(`Line ${index + 1}: ${line.length > 2000 ? `${line.slice(0, 2000)} (line truncated)` : line}`);
        matches += 1;
      }
    });
    if (lines.length > 0) blocks.push(`${entry.path}:\n${lines.join('\n')}`);
    if (matches >= MAX_GREP_MATCHES) break;
  }
  if (blocks.length === 0) return 'No matches found';
  return blocks.join('\n\n') + (matches >= MAX_GREP_MATCHES ? '\n... match limit reached; narrow path or include' : '');
}

function formatGlob(paths: string[]): string {
  if (paths.length === 0) return 'No files found';
  const visible = paths.slice(0, MAX_GLOB_RESULTS);
  const suffix = paths.length > visible.length
    ? `\n... ${paths.length - visible.length} more files omitted; narrow the pattern or path`
    : '';
  return visible.join('\n') + suffix;
}

function formatList(entries: LoomFileEntry[], root: string): string {
  const children = directChildren(entries.map((entry) => entry.path), root);
  if (children.length === 0) return 'Directory is empty';
  return children.map((child) => `${child.type === 'directory' ? 'DIR' : 'FILE'}\t${child.name}`).join('\n');
}

function formatOverlayList(entries: LoomFileEntry[], root: string, mountPath: string): string {
  const children = directChildren(entries.map((entry) => entry.path), root);
  if (root === '' && !children.some((child) => child.name === mountPath)) {
    children.push({ name: mountPath, type: 'directory' });
    children.sort((left, right) => left.name.localeCompare(right.name));
  }
  if (children.length === 0) return 'Directory is empty';
  return children.map((child) => `${child.type === 'directory' ? 'DIR' : 'FILE'}\t${child.name}`).join('\n');
}

function compileContentPattern(pattern: string): RegExp {
  try {
    return new RegExp(pattern);
  } catch (error) {
    throw new Error(`invalid grep pattern: ${error instanceof Error ? error.message : String(error)}`);
  }
}

async function selectEntries(vfs: LoomVfs, path: string | undefined): Promise<LoomFileEntry[]> {
  const root = normalizedRoot(path);
  if (!root) return vfs.list();
  try {
    const file = await vfs.read(root);
    return [{ path: file.path, repository: file.repository, commit: file.commit }];
  } catch {
    return vfs.list(`${root}/`);
  }
}

async function grepWorkspace(
  vfs: LoomVfs,
  pattern: string,
  path: string | undefined,
  include: string | undefined,
  signal: AbortSignal,
): Promise<string> {
  const matcher = compileContentPattern(pattern);
  const includeMatcher = include ? globRegex(include) : undefined;
  const includeBasenameOnly = include !== undefined && !include.includes('/');
  const entries = await selectEntries(vfs, path);
  if (entries.length > MAX_SEARCH_FILES) {
    throw new Error(`workspace search spans ${entries.length} files; narrow path or include`);
  }
  const blocks: string[] = [];
  let matches = 0;
  let bytes = 0;
  for (const entry of entries.sort((a, b) => a.path.localeCompare(b.path))) {
    abortIfNeeded(signal);
    if (includeMatcher) {
      const candidate = includeBasenameOnly
        ? entry.path.slice(entry.path.lastIndexOf('/') + 1)
        : entry.path;
      if (!includeMatcher.test(candidate)) continue;
    }
    const file = await vfs.read(entry.path);
    bytes += file.content.byteLength;
    if (bytes > MAX_SEARCH_BYTES) throw new Error('workspace grep exceeded 20 MB; narrow path or include');
    const text = new TextDecoder('utf-8', { fatal: false }).decode(file.content);
    if (text.includes('\0')) continue;
    const lines: string[] = [];
    text.split(/\r?\n/).forEach((line, index) => {
      if (matches >= MAX_GREP_MATCHES) return;
      matcher.lastIndex = 0;
      if (matcher.test(line)) {
        const preview = line.length > 2000 ? `${line.slice(0, 2000)} (line truncated)` : line;
        lines.push(`Line ${index + 1}: ${preview}`);
        matches += 1;
      }
    });
    if (lines.length > 0) blocks.push(`${entry.path}:\n${lines.join('\n')}`);
    if (matches >= MAX_GREP_MATCHES) break;
  }
  if (blocks.length === 0) return 'No matches found';
  const suffix = matches >= MAX_GREP_MATCHES ? '\n... match limit reached; narrow path or include' : '';
  return blocks.join('\n\n') + suffix;
}

function textOutput() {
  return {
    schema: { type: 'string' },
    render: (_args: unknown, value: unknown) => [{ type: 'text' as const, text: String(value) }],
  };
}

export function apply(ctx: Context, config: SearchConfig): void {
  const tools = (ctx as unknown as { tools: ToolRegistry }).tools;
  const searchContext = async (exec: ToolRunContext) => {
    const binding = await readWorkspaceBinding(exec.agent?.session.header.cwd);
    const workspace = binding?.workspace || config.workspace;
    if (!workspace) throw new Error('choose or create a Catalog Workspace before starting the Agent');
    const vfs = vfsForTask(config, {
      workspace,
      catalog: binding?.catalog || config.catalog || undefined,
    }, exec.signal);
    const mountPath = configuredMountPath(config);
    const cwd = exec.agent?.session.header.cwd || process.cwd();
    return { vfs, mountPath, cwd };
  };

  tools.register({
    name: 'list',
    description: 'List direct file and directory children in the mounted virtual Workspace.',
    parameters: {
      type: 'object',
      properties: {
        path: { type: 'string', description: 'Optional Workspace-relative directory; defaults to root.' },
      },
    },
    output: textOutput(),
    isConcurrencySafe: () => true,
    async execute(raw, exec) {
      abortIfNeeded(exec.signal);
      const args = parseObject(raw);
      const context = await searchContext(exec);
      const root = context.mountPath
        ? projectPath(optionalString(args, 'path'), context.cwd)
        : normalizedRoot(optionalString(args, 'path'));
      const entries = context.mountPath
        ? await overlayEntries(context.vfs, context.cwd, context.mountPath)
        : await context.vfs.list(root ? `${root}/` : undefined);
      return context.mountPath
        ? formatOverlayList(entries, root, context.mountPath)
        : formatList(entries, root);
    },
  });

  tools.register({
    name: 'glob',
    description: 'Discover files by glob pattern in the mounted virtual Workspace. Use this instead of shell find.',
    parameters: {
      type: 'object',
      properties: {
        pattern: { type: 'string', description: 'Glob pattern; * matches basenames at any depth and ** spans directories.' },
        path: { type: 'string', description: 'Optional Workspace-relative directory root.' },
      },
      required: ['pattern'],
    },
    output: textOutput(),
    isConcurrencySafe: () => true,
    async execute(raw, exec) {
      abortIfNeeded(exec.signal);
      const args = parseObject(raw);
      const pattern = requiredString(args, 'pattern');
      const context = await searchContext(exec);
      const root = context.mountPath
        ? projectPath(optionalString(args, 'path'), context.cwd)
        : normalizedRoot(optionalString(args, 'path'));
      const entries = context.mountPath
        ? await overlayEntries(context.vfs, context.cwd, context.mountPath)
        : await context.vfs.list(root ? `${root}/` : undefined);
      return formatGlob(matchGlob(entries, pattern, root));
    },
  });

  tools.register({
    name: 'grep',
    description: 'Search file contents with a regular expression in the mounted virtual Workspace. Read matched files for context.',
    parameters: {
      type: 'object',
      properties: {
        pattern: { type: 'string', description: 'JavaScript-compatible regular expression.' },
        path: { type: 'string', description: 'Optional Workspace-relative file or directory.' },
        include: { type: 'string', description: 'Optional positive glob filter such as *.md or **/*.go.' },
      },
      required: ['pattern'],
    },
    output: textOutput(),
    isConcurrencySafe: () => true,
    async execute(raw, exec) {
      abortIfNeeded(exec.signal);
      const args = parseObject(raw);
      const context = await searchContext(exec);
      if (context.mountPath) {
        return grepOverlay(
          context.vfs,
          context.cwd,
          await overlayEntries(context.vfs, context.cwd, context.mountPath),
          requiredString(args, 'pattern'),
          projectPath(optionalString(args, 'path'), context.cwd),
          optionalString(args, 'include'),
          exec.signal,
        );
      }
      return grepWorkspace(
        context.vfs,
        requiredString(args, 'pattern'),
        optionalString(args, 'path'),
        optionalString(args, 'include'),
        exec.signal,
      );
    },
  });

  tools.register({
    name: 'rg',
    description: 'Ripgrep-style regular-expression content search over the mounted virtual Workspace.',
    parameters: {
      type: 'object',
      properties: {
        pattern: { type: 'string', description: 'JavaScript-compatible regular expression.' },
        path: { type: 'string', description: 'Optional Workspace-relative file or directory.' },
        include: { type: 'string', description: 'Optional positive glob filter such as *.md or **/*.go.' },
      },
      required: ['pattern'],
    },
    output: textOutput(),
    isConcurrencySafe: () => true,
    async execute(raw, exec) {
      abortIfNeeded(exec.signal);
      const args = parseObject(raw);
      const context = await searchContext(exec);
      if (context.mountPath) {
        return grepOverlay(
          context.vfs,
          context.cwd,
          await overlayEntries(context.vfs, context.cwd, context.mountPath),
          requiredString(args, 'pattern'),
          projectPath(optionalString(args, 'path'), context.cwd),
          optionalString(args, 'include'),
          exec.signal,
        );
      }
      return grepWorkspace(
        context.vfs,
        requiredString(args, 'pattern'),
        optionalString(args, 'path'),
        optionalString(args, 'include'),
        exec.signal,
      );
    },
  });
}
