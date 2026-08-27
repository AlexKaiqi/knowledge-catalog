/**
 * Model-facing Knowledge Catalog control plane.
 *
 * This deliberately exposes kc's existing HTTP facade instead of duplicating
 * every command as a second TypeScript API. The Go command table remains the
 * authority for available verbs and request validation. The plugin contributes
 * transport, actor/request context, and local-service bootstrap only.
 */

import type { Context } from '@deepseek-ai/cordis';
import { execFile, spawn, type ChildProcess } from 'node:child_process';
import { access } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { resolveKcHome } from './binding.js';
import { LoomError } from './client.js';

export const name = 'loom-control';
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

export interface LoomControlConfig {
  baseURL?: string;
  /** Claimed principal forwarded as X-Kc-As. Empty means workspace owner. */
  as?: string;
  /** Gitea PAT sent as Authorization. Mutually exclusive with claimed as. */
  authToken?: string;
	/** Verified delegation subject fixed by the Agent composition. */
	onBehalfOf?: string;
  /** Persistent kc home used only when this plugin starts a local service. */
  home?: string;
  /** Optional kc executable. The packaged bootstrap helper resolves it otherwise. */
  bin?: string;
  /** Disable local auto-start when an external service is mandatory. */
  autoStart?: boolean;
  /** Stop a service started by this instance when its DSH composition exits. */
  stopOnDispose?: boolean;
  fetchImpl?: typeof fetch;
}

export interface KcCall {
  verb: string;
  flags?: Record<string, unknown>;
  requestId?: string;
}

function parseCall(raw: unknown): KcCall {
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) {
    throw new Error('kc tool arguments must be an object');
  }
  const args = raw as Record<string, unknown>;
  if (typeof args.verb !== 'string' || !args.verb.trim()) {
    throw new Error('verb must be a non-empty string');
  }
  if (args.flags !== undefined && (typeof args.flags !== 'object' || args.flags === null || Array.isArray(args.flags))) {
    throw new Error('flags must be a JSON object');
  }
  if ((args.flags as Record<string, unknown> | undefined)?.verb !== undefined) {
    throw new Error('verb belongs at the top level and must not be nested in flags');
  }
  if (args.requestId !== undefined && typeof args.requestId !== 'string') {
    throw new Error('requestId must be a string');
  }
  return {
    verb: args.verb.trim(),
    flags: args.flags as Record<string, unknown> | undefined,
    requestId: args.requestId as string | undefined,
  };
}

function requestId(): string {
  return `dsh-${globalThis.crypto?.randomUUID?.() ?? `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`}`;
}

function safeFlags(flags: Record<string, unknown> | undefined): Record<string, unknown> {
  const copy = { ...(flags ?? {}) };
  // Actor and local process coordinates come from the plugin composition, not
  // model arguments. This prevents a role from silently dropping --as and
  // falling back to the workspace owner.
  delete copy.as;
	delete copy['on-behalf-of'];
  delete copy.home;
  delete copy.listen;
  return copy;
}

function localListen(baseURL: string): string | undefined {
  const url = new URL(baseURL);
  if (url.protocol !== 'http:') return undefined;
  if (url.hostname !== '127.0.0.1' && url.hostname !== 'localhost' && url.hostname !== '::1') return undefined;
  return url.port ? `${url.hostname}:${url.port}` : `${url.hostname}:80`;
}

const execFileAsync = promisify(execFile);
const bootstrapScript = fileURLToPath(new URL('../scripts/kc-local.sh', import.meta.url));

export class LoomControl {
  private readonly baseURL: string;
  private readonly as?: string;
  private readonly authToken?: string;
	private readonly onBehalfOf?: string;
  private readonly home: string;
  private readonly configuredBin?: string;
  private readonly autoStart: boolean;
  private readonly stopOnDispose: boolean;
  private readonly fetchImpl: typeof fetch;
  private child?: ChildProcess;
  private starting?: Promise<void>;

  constructor(config: LoomControlConfig = {}) {
    this.baseURL = (config.baseURL?.trim() || process.env.KC_SERVE?.trim() || 'http://127.0.0.1:7380').replace(/\/$/, '');
    this.as = config.as?.trim() || process.env.KC_AS?.trim() || undefined;
    this.authToken = config.authToken?.trim() || process.env.KC_AUTH_TOKEN?.trim() || undefined;
    this.onBehalfOf = config.onBehalfOf?.trim() || process.env.KC_ON_BEHALF_OF?.trim() || undefined;
    if (this.as && this.authToken) {
      throw new Error('dsh-loom: as and authToken are mutually exclusive');
    }
    this.home = resolveKcHome(config.home);
    this.configuredBin = config.bin?.trim() || process.env.KC_BIN?.trim() || undefined;
    // A token belongs to an independently configured authenticated service.
    // Never silently auto-start an unauthenticated local owner facade and send
    // the same request to it.
    this.autoStart = this.authToken ? false : config.autoStart !== false;
    this.stopOnDispose = config.stopOnDispose === true;
    this.fetchImpl = config.fetchImpl ?? fetch;
  }

  private async healthy(signal?: AbortSignal): Promise<boolean> {
    try {
      const res = await this.fetchImpl(`${this.baseURL}/health`, { signal });
      return res.ok;
    } catch {
      return false;
    }
  }

  private async resolveBin(): Promise<string> {
    if (this.configuredBin) {
      await access(this.configuredBin);
      return this.configuredBin;
    }
    const { stdout } = await execFileAsync('bash', [bootstrapScript, 'path'], {
      env: process.env,
      maxBuffer: 1024 * 1024,
    });
    const bin = stdout.trim().split(/\r?\n/).at(-1) ?? '';
    if (!bin) throw new Error('kc bootstrap returned no executable path');
    return bin;
  }

  private async startLocal(): Promise<void> {
    if (await this.healthy()) return;
    if (!this.autoStart) throw new Error(`kc service is unavailable at ${this.baseURL}`);
    const listen = localListen(this.baseURL);
    if (!listen) throw new Error(`refusing to auto-start kc for non-local URL ${this.baseURL}`);
    const bin = await this.resolveBin();
    this.child = spawn(bin, ['--home', this.home, 'serve', '--listen', listen], {
      stdio: 'ignore',
      env: process.env,
      detached: !this.stopOnDispose,
    });
    if (!this.stopOnDispose) this.child.unref();
    const deadline = Date.now() + 15_000;
    while (Date.now() < deadline) {
      if (this.child.exitCode !== null) {
        throw new Error(`kc serve exited before becoming healthy (code ${this.child.exitCode})`);
      }
      if (await this.healthy()) return;
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
    throw new Error(`kc serve did not become healthy at ${this.baseURL}`);
  }

  private async ensureService(): Promise<void> {
    if (await this.healthy()) return;
    if (!this.starting) this.starting = this.startLocal().finally(() => { this.starting = undefined; });
    return this.starting;
  }

	async call(call: KcCall, signal?: AbortSignal): Promise<unknown> {
    await this.ensureService();
    const headers: Record<string, string> = {
      'content-type': 'application/json',
      'X-Kc-Request-Id': call.requestId?.trim() || requestId(),
    };
    if (this.as) headers['X-Kc-As'] = this.as;
    if (this.authToken) headers.Authorization = `Bearer ${this.authToken}`;
	if (this.onBehalfOf) headers['X-Kc-On-Behalf-Of'] = this.onBehalfOf;
    const flags = safeFlags(call.flags);
    const res = await this.fetchImpl(`${this.baseURL}/v1/${encodeURIComponent(call.verb)}`, {
      method: 'POST',
      headers,
      body: JSON.stringify(flags),
      signal,
    });
    const json = await res.json().catch(() => ({}));
    if (!res.ok) {
      const fault = (json as { error?: { code?: string; message?: string } }).error ?? {};
      throw new LoomError(
        String(fault.message ?? `${call.verb} failed with status ${res.status}`),
        String(fault.code ?? 'UNKNOWN'),
      );
    }
    return json;
  }

  dispose(): void {
    if (this.stopOnDispose && this.child && this.child.exitCode === null) this.child.kill();
  }
}

function textOutput() {
  return {
    schema: { type: 'string' },
    render: (_args: unknown, value: unknown) => [{ type: 'text' as const, text: String(value) }],
  };
}

function renderResult(verb: string, result: unknown): string {
  const envelope: Record<string, unknown> = { result };
  if (verb === 'propose') {
    envelope.agentGuidance = 'Proposal succeeded. The response intentionally omits the provenance body; if origin-kind/source-ref/actor-ref were in this successful request, they were accepted. Do not issue the same proposal-id again.';
  } else if (verb === 'gate-add') {
    envelope.agentGuidance = 'Gate configuration succeeded and the returned rule is authoritative. Use gate-ls through this tool if you need to inspect effective rules; never read KC internal files.';
  } else if (verb === 'merge') {
    envelope.agentGuidance = 'Merge succeeded. The receipt identifies the repository, target ref, Preview basis, and satisfied required checks. Do not inspect internal control or gate files.';
  } else if (['put', 'remove', 'commit', 'define-workspace', 'allow', 'revoke'].includes(verb)) {
    envelope.agentGuidance = 'Mutation succeeded. Do not repeat it merely to inspect omitted request fields; use the corresponding read, status, allowed, audit, log, or provenance command.';
  }
  return JSON.stringify(envelope, null, 2);
}

export function apply(ctx: Context, config: LoomControlConfig = {}): void {
  const tools = (ctx as unknown as { tools: ToolRegistry }).tools;
  const control = new LoomControl(config);
  ctx.effect(() => {
    const unregister = tools.register({
      name: 'kc',
      description: 'Execute one KC verb with JSON flags. Provider setup uses init, repo-add, ingest, commit, define-workspace with numeric revision, then resolve. Help topics are only provider, consumer, or governor. Put verb only at the top level. Actor identity is fixed by this composition. Catalog current state is read with flags {catalog:true} and no workspace/object; Workspace read requires object.',
      parameters: {
        type: 'object',
        properties: {
          verb: { type: 'string', description: 'kc verb such as init, repo-add, allow, put, propose, preview, validate, merge, resolve, read, search, audit, log, or provenance.' },
          flags: {
            type: 'object',
            description: 'CLI flag names without --; keep hyphens such as command-id and expected-target-commit. Arrays represent repeated flags such as source. verb is not a flag.',
            propertyNames: { not: { enum: ['verb', 'as', 'on-behalf-of', 'home', 'listen'] } },
            additionalProperties: true,
          },
          requestId: { type: 'string', description: 'Optional correlation token. A unique dsh-* token is generated when omitted.' },
        },
        required: ['verb'],
      },
      output: textOutput(),
      isConcurrencySafe: () => false,
      async execute(raw, exec) {
        const call = parseCall(raw);
		const result = await control.call(call, exec.signal);
        return renderResult(call.verb, result);
      },
    });
    return () => {
      unregister();
      control.dispose();
    };
  }, 'loom-control: kc tool');
}
