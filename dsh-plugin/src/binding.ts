import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';

export interface LoomWorkspaceBinding {
  catalog?: string;
  workspace: string;
}

interface StoredBinding extends LoomWorkspaceBinding {
  version: 1;
}

const BINDING_FILE = '.dsh-loom-workspace.json';

// Control bootstrap and the human-facing Workspace bridge must resolve the
// same local home. Cordis passes empty strings for unset optional config, so
// normalize here instead of making every caller remember that detail.
export function resolveKcHome(configured?: string): string {
  return path.resolve(
    configured?.trim()
      || process.env.KC_HOME?.trim()
      || path.join(process.cwd(), '.kc-home'),
  );
}

function safeSegment(value: string): string {
  return Buffer.from(value).toString('base64url');
}

export async function readWorkspaceBinding(cwd: string | undefined): Promise<LoomWorkspaceBinding | undefined> {
  if (!cwd || !path.isAbsolute(cwd)) return undefined;
  let dir = path.resolve(cwd);
  for (;;) {
    const file = path.join(dir, BINDING_FILE);
    let content: string;
    try {
      content = await readFile(file, 'utf8');
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== 'ENOENT') {
        throw new Error(`cannot read Workspace binding ${file}: ${error instanceof Error ? error.message : String(error)}`);
      }
      const parent = path.dirname(dir);
      if (parent === dir) return undefined;
      dir = parent;
      continue;
    }
    let parsed: Partial<StoredBinding>;
    try {
      parsed = JSON.parse(content) as Partial<StoredBinding>;
    } catch (error) {
      throw new Error(`invalid Workspace binding ${file}: ${error instanceof Error ? error.message : String(error)}`);
    }
    if (parsed.version !== 1 || typeof parsed.workspace !== 'string' || !parsed.workspace.trim()) {
      throw new Error(`invalid Workspace binding ${file}: expected version 1 and a non-empty workspace`);
    }
    if (parsed.catalog !== undefined && (typeof parsed.catalog !== 'string' || !parsed.catalog.trim())) {
      throw new Error(`invalid Workspace binding ${file}: catalog must be a non-empty string when present`);
    }
    return {
      workspace: parsed.workspace.trim(),
      ...(parsed.catalog ? { catalog: parsed.catalog.trim() } : {}),
    };
  }
}

export async function ensureWorkspaceAnchor(
  home: string,
  binding: LoomWorkspaceBinding,
): Promise<string> {
  if (!home || !path.isAbsolute(home)) throw new Error('dsh-loom: an absolute KC_HOME is required');
  const identity = `${binding.catalog ?? 'default'}\0${binding.workspace}`;
  const dir = path.join(home, 'agent-workspaces', safeSegment(identity));
  await mkdir(dir, { recursive: true });
  const stored: StoredBinding = {
    version: 1,
    workspace: binding.workspace,
    ...(binding.catalog ? { catalog: binding.catalog } : {}),
  };
  await writeFile(path.join(dir, BINDING_FILE), `${JSON.stringify(stored, null, 2)}\n`, 'utf8');
  return dir;
}
