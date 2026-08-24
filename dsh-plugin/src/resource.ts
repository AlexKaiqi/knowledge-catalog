/** Agent-facing access to a ResourceDescriptor stored in the current Workspace. */

import type { Context } from '@deepseek-ai/cordis';
import { readWorkspaceBinding, type LoomWorkspaceBinding } from './binding.js';
import { LoomControl, type LoomControlConfig } from './control.js';

export const name = 'loom-resource';
export const inject = ['tools'];

type JsonObject = Record<string, unknown>;
type JsonSchema = Record<string, unknown>;

interface ToolRunContext {
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

export interface LoomResourceConfig extends LoomControlConfig {
  /** Base URL of the platform resource-access runtime. */
  accessURL?: string;
}

export interface ResourceCall {
  descriptor: string;
  operation: string;
  input?: JsonObject;
  requestId?: string;
}

interface DescriptorRecord {
  objectId: string;
  repository: string;
  commit: string;
  value: JsonObject;
}

function parseCall(raw: unknown): ResourceCall {
  if (typeof raw !== 'object' || raw === null || Array.isArray(raw)) {
    throw new Error('resource arguments must be an object');
  }
  const args = raw as JsonObject;
  if (typeof args.descriptor !== 'string' || !args.descriptor.trim()) {
    throw new Error('descriptor must be a non-empty object_id');
  }
  if (typeof args.operation !== 'string' || !args.operation.trim()) {
    throw new Error('operation must be a non-empty string');
  }
  if (args.input !== undefined && (typeof args.input !== 'object' || args.input === null || Array.isArray(args.input))) {
    throw new Error('input must be a JSON object');
  }
  if (args.requestId !== undefined && typeof args.requestId !== 'string') {
    throw new Error('requestId must be a string');
  }
  return {
    descriptor: args.descriptor.trim(),
    operation: args.operation.trim(),
    input: args.input as JsonObject | undefined,
    requestId: args.requestId as string | undefined,
  };
}

function requestId(): string {
  return `resource-${globalThis.crypto?.randomUUID?.() ?? `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`}`;
}

function asObject(value: unknown, label: string): JsonObject {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new Error(`${label} must be a JSON object`);
  }
  return value as JsonObject;
}

function descriptorFromRead(raw: unknown, objectId: string): DescriptorRecord {
  const rows = Array.isArray(raw) ? raw : [raw];
  const matches: DescriptorRecord[] = [];
  for (const item of rows) {
    if (typeof item !== 'object' || item === null || Array.isArray(item)) continue;
    const row = item as JsonObject;
    const value = row.value;
    const id = typeof row.objectId === 'string' ? row.objectId : objectId;
    if (id !== objectId || typeof value !== 'object' || value === null || Array.isArray(value)) continue;
    matches.push({
      objectId: id,
      repository: typeof row.repository === 'string' ? row.repository : '',
      commit: typeof row.commit === 'string' ? row.commit : '',
      value: value as JsonObject,
    });
  }
  if (matches.length === 0) throw new Error(`ResourceDescriptor ${objectId} was not found in the current Workspace`);
  if (matches.length > 1) throw new Error(`ResourceDescriptor ${objectId} is ambiguous across Workspace repositories`);
  return matches[0];
}

function validateDescriptor(record: DescriptorRecord, operation: string): { runtime: string; protocol: string } {
  if (record.value.kind !== 'ResourceDescriptor') {
    throw new Error(`${record.objectId} is not a ResourceDescriptor`);
  }
  const runtime = typeof record.value.runtime === 'string' ? record.value.runtime.trim() : '';
  const protocol = typeof record.value.protocol === 'string' ? record.value.protocol.trim() : '';
  if (!runtime || !protocol) throw new Error(`ResourceDescriptor ${record.objectId} must declare runtime and protocol`);
  const access = asObject(record.value.access, `ResourceDescriptor ${record.objectId}.access`);
  if (!(operation in access)) throw new Error(`operation ${operation} is not declared by ResourceDescriptor ${record.objectId}`);
  asObject(access[operation], `ResourceDescriptor ${record.objectId}.access.${operation}`);
  return { runtime, protocol };
}

export class LoomResourceAccess {
  private readonly accessURL: string;
  private readonly principal: string;
  private readonly fetchImpl: typeof fetch;
  private readonly control: LoomControl;

  constructor(config: LoomResourceConfig = {}) {
    this.accessURL = (config.accessURL || process.env.KC_RESOURCE_ACCESS_URL || '').replace(/\/$/, '');
    this.principal = config.as?.trim() || 'local-owner';
    this.fetchImpl = config.fetchImpl ?? fetch;
    this.control = new LoomControl(config);
  }

  async call(call: ResourceCall, exec: ToolRunContext, binding: LoomWorkspaceBinding): Promise<unknown> {
    if (!this.accessURL) throw new Error('resource access runtime is not configured');
    const id = call.requestId?.trim() || requestId();
    const raw = await this.control.call(
      { verb: 'read', flags: { object: call.descriptor }, requestId: `${id}:descriptor` },
      exec.signal,
      binding,
    );
    const descriptor = descriptorFromRead(raw, call.descriptor);
    const declared = validateDescriptor(descriptor, call.operation);
    const header = exec.agent?.session.header;
    const headers: Record<string, string> = {
      'content-type': 'application/json',
      'X-Resource-Principal': this.principal,
      'X-Resource-Request-Id': id,
    };
    if (header?.id) headers['X-Agent-Session'] = header.id;
    headers['X-Agent-Preset'] = header?.agentPreset || 'dsh';
    if (header?.parentSession) headers['X-Agent-Parent-Session'] = header.parentSession;
    if (header?.delegationDepth !== undefined) headers['X-Agent-Delegation-Depth'] = String(header.delegationDepth);
    const res = await this.fetchImpl(`${this.accessURL}/v1/access`, {
      method: 'POST',
      headers,
      body: JSON.stringify({
        descriptor: {
          objectId: descriptor.objectId,
          repository: descriptor.repository,
          commit: descriptor.commit,
        },
        runtime: declared.runtime,
        protocol: declared.protocol,
        operation: call.operation,
        input: call.input ?? {},
      }),
      signal: exec.signal,
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) {
      const message = (body as { error?: { message?: string } }).error?.message;
      throw new Error(message || `resource access failed with status ${res.status}`);
    }
    return body;
  }

  dispose(): void {
    this.control.dispose();
  }
}

function textOutput() {
  return {
    schema: { type: 'string' },
    render: (_args: unknown, value: unknown) => [{ type: 'text' as const, text: String(value) }],
  };
}

export function apply(ctx: Context, config: LoomResourceConfig = {}): void {
  const tools = (ctx as unknown as { tools: ToolRegistry }).tools;
  const access = new LoomResourceAccess(config);
  ctx.effect(() => {
    const unregister = tools.register({
      name: 'resource',
      description: 'Access one live resource through a ResourceDescriptor in the current Knowledge Catalog Workspace. Identity and Agent session context are supplied by the DSH composition.',
      parameters: {
        type: 'object',
        properties: {
          descriptor: { type: 'string', description: 'ResourceDescriptor object_id discovered in the current Workspace.' },
          operation: { type: 'string', description: 'An operation declared by the Descriptor, such as status, window, lookup, or search.' },
          input: { type: 'object', description: 'Operation input declared by the Descriptor.', additionalProperties: true },
          requestId: { type: 'string', description: 'Optional stable correlation ID for an identical retry.' },
        },
        required: ['descriptor', 'operation'],
      },
      output: textOutput(),
      isConcurrencySafe: () => true,
      async execute(raw, exec) {
        const call = parseCall(raw);
        const binding = await readWorkspaceBinding(exec.agent?.session.header.cwd);
        if (!binding) throw new Error('resource access requires a DSH session bound to a Catalog Workspace');
        return JSON.stringify(await access.call(call, exec, binding), null, 2);
      },
    });
    return () => {
      unregister();
      access.dispose();
    };
  }, 'loom-resource: resource tool');
}
