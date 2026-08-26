/** Agent-facing access to a ResourceDescriptor stored in the current Workspace. */

import type { Context } from '@deepseek-ai/cordis';
import { LoomControl } from './control.js';
import {
	observePinnedKnowledgeContextLifecycle,
	pinnedKnowledgeContext,
	scopedKnowledgeFlags,
	type AgentToolRunContext,
	type PinnedKnowledgeContext,
	type PinnedKnowledgeContextConfig,
} from './context.js';

export const name = 'loom-resource';
export const inject = ['tools', 'sessions'];

type JsonObject = Record<string, unknown>;
type JsonSchema = Record<string, unknown>;

interface ToolDefinition {
  name: string;
  description: string;
  parameters: JsonSchema;
  output: {
    schema: JsonSchema;
    render(args: unknown, value: unknown): Array<{ type: 'text'; text: string }>;
  };
	execute(args: unknown, exec: AgentToolRunContext): Promise<unknown>;
  isConcurrencySafe?(args: unknown): boolean;
}

interface ToolRegistry {
  register(definition: ToolDefinition): () => void;
}

export interface LoomResourceConfig extends PinnedKnowledgeContextConfig {
  /** Base URL of the platform resource-access runtime. */
  accessURL?: string;
}

export interface ResourceCall {
  /** Preferred: the object whose Aspect carries value_source.kind=binding. */
  object?: string;
  aspect?: string;
  /** Compatibility path for a directly discovered ResourceDescriptor. */
  descriptor?: string;
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
	const descriptor = typeof args.descriptor === 'string' ? args.descriptor.trim() : '';
	const object = typeof args.object === 'string' ? args.object.trim() : '';
	const aspect = typeof args.aspect === 'string' ? args.aspect.trim() : '';
	if ((!object || !aspect) && !descriptor) throw new Error('object and aspect, or descriptor, are required');
	if (descriptor && (object || aspect)) throw new Error('descriptor is mutually exclusive with object/aspect');
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
		...(descriptor ? { descriptor } : { object, aspect }),
    operation: args.operation.trim(),
    input: args.input as JsonObject | undefined,
    requestId: args.requestId as string | undefined,
  };
}

interface BindingRecord {
  repository: string;
  declarationCommit: string;
  address: JsonObject;
  declarationDigest: string;
  mode: 'state' | 'stream';
  runtime: string;
  protocol: string;
  operations: Record<string, { call: string }>;
  descriptorRef?: string;
  descriptorDigest?: string;
}

function bindingFromResolve(raw: unknown, call: ResourceCall): BindingRecord {
  const rows = Array.isArray(raw) ? raw : [raw];
  if (rows.length === 0) throw new Error(`Binding ${call.object}/${call.aspect} was not found in the current Workspace`);
  if (rows.length > 1) throw new Error(`Binding ${call.object}/${call.aspect} is ambiguous across Workspace repositories`);
  const row = asObject(rows[0], 'resolved binding');
  const runtime = typeof row.runtime === 'string' ? row.runtime.trim() : '';
  const protocol = typeof row.protocol === 'string' ? row.protocol.trim() : '';
  const mode = row.mode === 'state' || row.mode === 'stream' ? row.mode : undefined;
  const operations = asObject(row.operations, 'resolved binding.operations') as Record<string, { call: string }>;
  if (!runtime || !protocol || !mode) throw new Error('resolved Binding is incomplete');
  const operation = operations[call.operation];
  if (!operation || typeof operation.call !== 'string' || !operation.call.trim()) {
    throw new Error(`operation ${call.operation} is not declared by Binding ${call.object}/${call.aspect}`);
  }
  return {
    repository: String(row.repository ?? ''), declarationCommit: String(row.declarationCommit ?? ''),
    address: asObject(row.address, 'resolved binding.address'), declarationDigest: String(row.declarationDigest ?? ''),
    mode, runtime, protocol, operations,
    ...(typeof row.descriptorRef === 'string' ? { descriptorRef: row.descriptorRef } : {}),
    ...(typeof row.descriptorDigest === 'string' ? { descriptorDigest: row.descriptorDigest } : {}),
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
  private readonly fetchImpl: typeof fetch;
  private readonly control: LoomControl;
	private readonly config: LoomResourceConfig;

  constructor(config: LoomResourceConfig = {}) {
	this.config = config;
    this.accessURL = (config.accessURL || process.env.KC_RESOURCE_ACCESS_URL || '').replace(/\/$/, '');
    this.fetchImpl = config.fetchImpl ?? fetch;
    this.control = new LoomControl(config);
  }

	async context(exec: AgentToolRunContext): Promise<PinnedKnowledgeContext> {
		return pinnedKnowledgeContext(this.control, this.config, exec);
	}

	async call(call: ResourceCall, exec: AgentToolRunContext, context: PinnedKnowledgeContext): Promise<unknown> {
    if (!this.accessURL) throw new Error('resource access runtime is not configured');
    const id = call.requestId?.trim() || requestId();
		let declaration: BindingRecord | undefined;
		let descriptor: DescriptorRecord | undefined;
		let declared: { runtime: string; protocol: string };
		if (call.object && call.aspect) {
			const raw = await this.control.call(
				{ verb: 'resolve-binding', flags: scopedKnowledgeFlags(context, { object: call.object, aspect: call.aspect }), requestId: `${id}:binding` },
				exec.signal,
			);
			declaration = bindingFromResolve(raw, call);
			declared = { runtime: declaration.runtime, protocol: declaration.protocol };
		} else {
			const raw = await this.control.call(
				{ verb: 'read', flags: scopedKnowledgeFlags(context, { object: call.descriptor }), requestId: `${id}:descriptor` },
				exec.signal,
			);
			descriptor = descriptorFromRead(raw, call.descriptor!);
			declared = validateDescriptor(descriptor, call.operation);
		}
    const header = exec.agent?.session.header;
    const headers: Record<string, string> = {
      'content-type': 'application/json',
		'X-Resource-Principal': context.identity.principal,
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
				...(descriptor ? { descriptor: {
          objectId: descriptor.objectId,
          repository: descriptor.repository,
          commit: descriptor.commit,
				} } : { binding: {
					repository: declaration!.repository,
					declarationCommit: declaration!.declarationCommit,
					address: declaration!.address,
					declarationDigest: declaration!.declarationDigest,
					mode: declaration!.mode,
					...(declaration!.descriptorRef ? { descriptorRef: declaration!.descriptorRef } : {}),
					...(declaration!.descriptorDigest ? { descriptorDigest: declaration!.descriptorDigest } : {}),
				} }),
        runtime: declared.runtime,
        protocol: declared.protocol,
        operation: call.operation,
				call: declaration?.operations[call.operation].call,
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

function structuredOutput() {
	return {
		schema: { type: 'object' },
		render: (_args: unknown, value: unknown) => [{ type: 'text' as const, text: JSON.stringify(value, null, 2) }],
  };
}

export function apply(ctx: Context, config: LoomResourceConfig = {}): void {
  const tools = (ctx as unknown as { tools: ToolRegistry }).tools;
  const access = new LoomResourceAccess(config);
  ctx.effect(() => {
    const stopObservingTasks = observePinnedKnowledgeContextLifecycle(ctx);
    const unregister = tools.register({
      name: 'resource',
			description: 'Observe one live Aspect through its pinned Binding declaration. A direct ResourceDescriptor remains supported for compatibility.',
      parameters: {
        type: 'object',
        properties: {
          descriptor: { type: 'string', description: 'ResourceDescriptor object_id discovered in the current Workspace.' },
					object: { type: 'string', description: 'Object ID whose Aspect declares a Binding.' },
					aspect: { type: 'string', description: 'Aspect name whose value_source is binding.' },
          operation: { type: 'string', description: 'An operation declared by the Descriptor, such as status, window, lookup, or search.' },
          input: { type: 'object', description: 'Operation input declared by the Descriptor.', additionalProperties: true },
          requestId: { type: 'string', description: 'Optional stable correlation ID for an identical retry.' },
        },
				required: ['operation'],
				oneOf: [{ required: ['object', 'aspect'] }, { required: ['descriptor'] }],
      },
			output: structuredOutput(),
      isConcurrencySafe: () => true,
			async execute(raw, exec) {
				const call = parseCall(raw);
				const context = await access.context(exec);
				return access.call(call, exec, context);
      },
    });
    return () => {
      stopObservingTasks();
      unregister();
      access.dispose();
    };
  }, 'loom-resource: resource tool');
}
