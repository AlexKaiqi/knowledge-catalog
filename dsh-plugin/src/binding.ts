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

function safeSegment(value: string): string {
  return Buffer.from(value).toString('base64url');
}

export async function readWorkspaceBinding(cwd: string | undefined): Promise<LoomWorkspaceBinding | undefined> {
  if (!cwd || !path.isAbsolute(cwd)) return undefined;
  try {
    const parsed = JSON.parse(await readFile(path.join(cwd, BINDING_FILE), 'utf8')) as Partial<StoredBinding>;
    if (parsed.version !== 1 || typeof parsed.workspace !== 'string' || !parsed.workspace.trim()) return undefined;
    return {
      workspace: parsed.workspace.trim(),
      ...(typeof parsed.catalog === 'string' && parsed.catalog.trim() ? { catalog: parsed.catalog.trim() } : {}),
    };
  } catch {
    return undefined;
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
