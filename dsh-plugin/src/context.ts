import { createHash } from 'node:crypto';
import type { Context } from '@deepseek-ai/cordis';
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

export interface KnowledgeCapabilities {
  // null keeps compatibility with an older KC service that does not expose
  // store-ls. false is authoritative and lets the Agent skip a doomed SEARCH.
  search: boolean | null;
  index?: string;
}

// A task-local client context. This is not a KC protocol resource: the server
// receives the complete ResolvedWorkspace pin and identity on every request.
export interface PinnedKnowledgeContext {
  catalog?: string;
  workspace: string;
  bindingSource: 'directory' | 'configuration';
  pin: ResolvedWorkspacePin;
  identity: KnowledgeIdentity;
  capabilities: KnowledgeCapabilities;
}

export interface PinnedKnowledgeContextConfig extends LoomControlConfig {
  catalog?: string;
  workspace?: string;
}

export interface AgentToolRunContext {
  signal: AbortSignal;
  agent?: {
    // DSH owns this host lifecycle object. KC uses its id only to keep a pin
    // stable for one Agent task and to release the local cache on disposal.
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

interface CachedContext {
  hostTaskID: string;
  pending: Promise<PinnedKnowledgeContext>;
}

const contexts = new Map<string, CachedContext>();

function configuredBinding(config: PinnedKnowledgeContextConfig): LoomWorkspaceBinding | undefined {
  const workspace = config.workspace?.trim() || process.env.KC_WORKSPACE?.trim();
  if (!workspace) return undefined;
  const catalog = config.catalog?.trim() || process.env.KC_CATALOG?.trim();
  return { workspace, ...(catalog ? { catalog } : {}) };
}

interface LocatedBinding extends LoomWorkspaceBinding {
  bindingSource: PinnedKnowledgeContext['bindingSource'];
}

export async function selectedKnowledgeBinding(
  config: PinnedKnowledgeContextConfig,
  exec: AgentToolRunContext,
): Promise<LocatedBinding> {
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

function contextKey(
  config: PinnedKnowledgeContextConfig,
  binding: LoomWorkspaceBinding,
  hostTaskID: string,
): string {
  const endpoint = (config.baseURL?.trim() || process.env.KC_SERVE?.trim() || 'http://127.0.0.1:7380').replace(/\/$/, '');
  const token = config.authToken?.trim() || process.env.KC_AUTH_TOKEN?.trim();
  const identity = token
    ? `token:${secretFingerprint(token)}`
    : `as:${config.as?.trim() || process.env.KC_AS?.trim() || 'owner'}`;
  const onBehalfOf = config.onBehalfOf?.trim() || process.env.KC_ON_BEHALF_OF?.trim() || '';
  return [endpoint, identity, `obo:${onBehalfOf}`, hostTaskID, binding.catalog || '', binding.workspace].join('\0');
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

function resolvedCapabilities(raw: unknown): KnowledgeCapabilities {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return { search: null };
  const index = typeof (raw as Record<string, unknown>).index === 'string'
    ? String((raw as Record<string, unknown>).index).trim()
    : '';
  if (!index) return { search: null };
  return { search: index === 'opensearch', index };
}

async function loadCapabilities(control: LoomControl, signal: AbortSignal): Promise<KnowledgeCapabilities> {
  try {
    return resolvedCapabilities(await control.call({ verb: 'store-ls' }, signal));
  } catch {
    // Capability discovery was added after the original plugin contract. Do
    // not make READ unavailable against an older KC service merely because it
    // cannot answer store-ls; SEARCH will retain its explicit service error.
    return { search: null };
  }
}

function resolveContext(
  control: LoomControl,
  binding: LocatedBinding,
  exec: AgentToolRunContext,
): Promise<PinnedKnowledgeContext> {
  return Promise.all([
    control.call({ verb: 'resolve', flags: {
      ...(binding.catalog ? { catalog: binding.catalog } : {}), workspace: binding.workspace,
    } }, exec.signal),
    control.call({ verb: 'whoami' }, exec.signal),
    loadCapabilities(control, exec.signal),
  ]).then(([pin, identity, capabilities]) => ({
    ...binding,
    pin: resolvedPin(pin, binding),
    identity: resolvedIdentity(identity),
    capabilities,
  }));
}

export async function pinnedKnowledgeContext(
  control: LoomControl,
  config: PinnedKnowledgeContextConfig,
  exec: AgentToolRunContext,
): Promise<PinnedKnowledgeContext> {
  const binding = await selectedKnowledgeBinding(config, exec);
  const hostTaskID = exec.agent?.session.header.id?.trim();
  // Direct library callers may not run inside a DSH task. Without a stable
  // lifecycle identity, resolve per operation instead of leaking a global entry.
  if (!hostTaskID) return resolveContext(control, binding, exec);

  const key = contextKey(config, binding, hostTaskID);
  const cached = contexts.get(key);
  if (cached) return cached.pending;

  let pending: Promise<PinnedKnowledgeContext>;
  pending = resolveContext(control, binding, exec).catch((error) => {
    if (contexts.get(key)?.pending === pending) contexts.delete(key);
    throw error;
  });
  contexts.set(key, { hostTaskID, pending });
  return pending;
}

export function scopedKnowledgeFlags(context: PinnedKnowledgeContext, flags: Record<string, unknown>): Record<string, unknown> {
  return {
    ...flags,
    ...(context.catalog ? { catalog: context.catalog } : {}),
    workspace: context.workspace,
    pin: context.pin,
  };
}

function releasePinnedKnowledgeContexts(hostTaskID: string): void {
  if (!hostTaskID) return;
  for (const [key, cached] of contexts) {
    if (cached.hostTaskID === hostTaskID) contexts.delete(key);
  }
}

interface HostTaskLifecycleContext {
  on(
    event: 'session/disposed',
    listener: (task: { header: { id?: string } }) => void,
    options?: { global?: boolean },
  ): () => boolean;
}

// DSH calls this hook at the host task boundary. The event name belongs to DSH;
// it does not create a KC WorkspaceSession or a server-side session resource.
export function observePinnedKnowledgeContextLifecycle(ctx: Context): () => boolean {
  return (ctx as unknown as HostTaskLifecycleContext).on('session/disposed', (task) => {
    releasePinnedKnowledgeContexts(task.header.id?.trim() || '');
  }, { global: true });
}

export function clearPinnedKnowledgeContextsForTests(): void {
  contexts.clear();
}
