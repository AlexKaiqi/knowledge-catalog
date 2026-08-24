/**
 * Model-facing discovery over the same remote Workspace used by LoomFileSystem.
 * DSH's stock glob/grep plugin shells out to ripgrep in the host cwd, which
 * cannot see an unmaterialized virtual filesystem. These tools deliberately
 * query kc serve instead, so discovery, exact reads, authorization, and mount
 * routing all observe one Workspace data plane.
 */

import type { Context } from '@deepseek-ai/cordis';
import { LoomVfs, type LoomFileEntry, type LoomVfsConfig } from './client.js';
import { readWorkspaceBinding } from './binding.js';
import { directChildren, normalizePath } from './tree.js';

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

export function apply(ctx: Context, config: LoomVfsConfig & { home?: string }): void {
  const tools = (ctx as unknown as { tools: ToolRegistry }).tools;
  const vfsFor = async (exec: ToolRunContext) => {
    const binding = await readWorkspaceBinding(exec.agent?.session.header.cwd);
    const workspace = binding?.workspace || config.workspace;
    if (!workspace) throw new Error('choose or create a Catalog Workspace before starting the Agent');
    return new LoomVfs({
      baseURL: config.baseURL || 'http://127.0.0.1:7380', workspace,
      catalog: binding?.catalog || config.catalog || undefined,
      as: config.as || undefined, authToken: config.authToken || undefined, fetchImpl: config.fetchImpl,
    });
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
      const root = normalizedRoot(optionalString(args, 'path'));
      const entries = await (await vfsFor(exec)).list(root ? `${root}/` : undefined);
      return formatList(entries, root);
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
      const root = normalizedRoot(optionalString(args, 'path'));
      const entries = await (await vfsFor(exec)).list(root ? `${root}/` : undefined);
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
      return grepWorkspace(
        await vfsFor(exec),
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
      return grepWorkspace(
        await vfsFor(exec),
        requiredString(args, 'pattern'),
        optionalString(args, 'path'),
        optionalString(args, 'include'),
        exec.signal,
      );
    },
  });
}
