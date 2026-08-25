import { LoomVfs, type LoomVfsConfig } from './client.js';
import type { LoomWorkspaceBinding } from './binding.js';

/**
 * One model task (one DSH tool-execution signal) must observe one Workspace
 * pin, even when it mixes filesystem Read with list/glob/grep.  A later task
 * gets a different signal and therefore resolves the Workspace again.
 *
 * The cache is module-global because loom-fs and loom-search are separate
 * Cordis plugins but participate in the same Agent task. Weak keys ensure a
 * completed task cannot be retained by this module.
 */
const taskClients = new WeakMap<AbortSignal, Map<string, LoomVfs>>();

function identity(config: LoomVfsConfig, binding: LoomWorkspaceBinding): string {
  return JSON.stringify([
    config.baseURL.replace(/\/$/, ''),
    binding.catalog ?? '',
    binding.workspace,
    config.as ?? '',
    config.authToken ?? '',
  ]);
}

function create(config: LoomVfsConfig, binding: LoomWorkspaceBinding): LoomVfs {
  return new LoomVfs({
    baseURL: config.baseURL || 'http://127.0.0.1:7380',
    workspace: binding.workspace,
    catalog: binding.catalog,
    as: config.as || undefined,
    authToken: config.authToken || undefined,
    fetchImpl: config.fetchImpl,
  });
}

export function vfsForTask(
  config: LoomVfsConfig,
  binding: LoomWorkspaceBinding,
  signal?: AbortSignal,
): LoomVfs {
  if (!signal) return create(config, binding);
  let clients = taskClients.get(signal);
  if (!clients) {
    clients = new Map<string, LoomVfs>();
    taskClients.set(signal, clients);
  }
  const key = identity(config, binding);
  let client = clients.get(key);
  if (!client) {
    client = create(config, binding);
    clients.set(key, client);
  }
  return client;
}
