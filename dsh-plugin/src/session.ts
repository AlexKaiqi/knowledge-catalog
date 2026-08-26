import { createHash } from 'node:crypto';
import { readWorkspaceBinding, type LoomWorkspaceBinding } from './binding.js';
import { LoomControl, type LoomControlConfig } from './control.js';

export interface ResolvedWorkspacePin {
  workspaceId: string;
  revision: number;
  repositories: Record<string, string>;
  pinId?: string;
}

export interface KnowledgeIdentity {
  principal: string;
  onBehalfOf?: string;
  provider?: string;
  subject?: string;
  login?: string;
}

export interface KnowledgeSession {
  catalog?: string;
  workspace: string;
  bindingSource: 'directory' | 'configuration';
  pin: ResolvedWorkspacePin;
  identity: KnowledgeIdentity;
}

export interface KnowledgeSessionConfig extends LoomControlConfig {
  catalog?: string;
  workspace?: string;
}

export interface AgentToolRunContext {
  signal: AbortSignal;
  agent?: {
    session: {
      header: {
        id?: string;
        cwd?: string;
        parentSession?: string;
        delegationDepth?: number;
        agentPreset?: string;
      };
    };
  };
}

const sessions = new Map<string, Promise<KnowledgeSession>>();

function configuredBinding(config: KnowledgeSessionConfig): LoomWorkspaceBinding | undefined {
  const workspace = config.workspace?.trim() || process.env.KC_WORKSPACE?.trim();
  if (!workspace) return undefined;
  const catalog = config.catalog?.trim() || process.env.KC_CATALOG?.trim();
  return { workspace, ...(catalog ? { catalog } : {}) };
}

interface LocatedBinding extends LoomWorkspaceBinding {
  bindingSource: KnowledgeSession['bindingSource'];
}

async function bindingFor(config: KnowledgeSessionConfig, exec: AgentToolRunContext): Promise<LocatedBinding> {
  const fromDirectory = await readWorkspaceBinding(exec.agent?.session.header.cwd);
  const configured = configuredBinding(config);
  const binding = fromDirectory
    ? { ...fromDirectory, bindingSource: 'directory' as const }
    : configured
      ? { ...configured, bindingSource: 'configuration' as const }
      : undefined;
  if (!binding) {
    throw new Error('no Knowledge Workspace is selected; open one from the Knowledge Catalog panel or set KC_WORKSPACE before starting the Agent');
  }
  return binding;
}

function secretFingerprint(value: string): string {
  return createHash('sha256').update(value).digest('hex').slice(0, 16);
}

function sessionKey(config: KnowledgeSessionConfig, binding: LoomWorkspaceBinding, exec: AgentToolRunContext): string {
  const header = exec.agent?.session.header;
  const agent = header?.id || header?.cwd || 'default-agent-session';
  const endpoint = (config.baseURL?.trim() || process.env.KC_SERVE?.trim() || 'http://127.0.0.1:7380').replace(/\/$/, '');
  const token = config.authToken?.trim() || process.env.KC_AUTH_TOKEN?.trim();
  const identity = token
    ? `token:${secretFingerprint(token)}`
    : `as:${config.as?.trim() || process.env.KC_AS?.trim() || 'owner'}`;
  const onBehalfOf = config.onBehalfOf?.trim() || process.env.KC_ON_BEHALF_OF?.trim() || '';
  return [endpoint, identity, `obo:${onBehalfOf}`, agent, binding.catalog || '', binding.workspace].join('\0');
}

function resolvedPin(raw: unknown, binding: LoomWorkspaceBinding): ResolvedWorkspacePin {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    throw new Error('resolve returned an invalid Workspace pin');
  }
  const value = raw as Record<string, unknown>;
  const repositories = value.repositories;
  if (!repositories || typeof repositories !== 'object' || Array.isArray(repositories)) {
    throw new Error('resolve returned no repository coordinates');
  }
  const workspaceId = String(value.workspaceId ?? '');
  if (workspaceId !== binding.workspace) {
    throw new Error(`resolve returned Workspace ${workspaceId || '<empty>'}, expected ${binding.workspace}`);
  }
  return {
    workspaceId,
    revision: Number(value.revision ?? 0),
    repositories: repositories as Record<string, string>,
    ...(typeof value.pinId === 'string' && value.pinId ? { pinId: value.pinId } : {}),
  };
}

function resolvedIdentity(raw: unknown): KnowledgeIdentity {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    throw new Error('whoami returned an invalid identity');
  }
  const value = raw as Record<string, unknown>;
  const principal = typeof value.principal === 'string' ? value.principal : '';
  if (!principal) throw new Error('whoami returned no principal');
  return {
    principal,
    ...(typeof value.onBehalfOf === 'string' ? { onBehalfOf: value.onBehalfOf } : {}),
    ...(typeof value.provider === 'string' ? { provider: value.provider } : {}),
    ...(typeof value.subject === 'string' ? { subject: value.subject } : {}),
    ...(typeof value.login === 'string' ? { login: value.login } : {}),
  };
}

export async function knowledgeSession(
  control: LoomControl,
  config: KnowledgeSessionConfig,
  exec: AgentToolRunContext,
): Promise<KnowledgeSession> {
  const binding = await bindingFor(config, exec);
  const key = sessionKey(config, binding, exec);
  let pending = sessions.get(key);
  if (!pending) {
    pending = Promise.all([
      control.call({ verb: 'resolve', flags: {
        ...(binding.catalog ? { catalog: binding.catalog } : {}), workspace: binding.workspace,
      } }, exec.signal),
      control.call({ verb: 'whoami' }, exec.signal),
    ]).then(([pin, identity]) => ({
      ...binding,
      pin: resolvedPin(pin, binding),
      identity: resolvedIdentity(identity),
    })).catch((error) => {
      sessions.delete(key);
      throw error;
    });
    sessions.set(key, pending);
  }
  return pending;
}

export function scopedKnowledgeFlags(session: KnowledgeSession, flags: Record<string, unknown>): Record<string, unknown> {
  return {
    ...flags,
    ...(session.catalog ? { catalog: session.catalog } : {}),
    workspace: session.workspace,
    pin: session.pin,
  };
}

export function clearKnowledgeSessionsForTests(): void {
  sessions.clear();
}
