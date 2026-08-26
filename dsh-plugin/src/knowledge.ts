import type { Context } from '@deepseek-ai/cordis';
import { LoomError } from './client.js';
import { LoomControl } from './control.js';
import {
  observePinnedKnowledgeContextLifecycle,
  pinnedKnowledgeContext,
  scopedKnowledgeFlags,
  type AgentToolRunContext,
  type PinnedKnowledgeContextConfig,
} from './context.js';

export const name = 'loom-knowledge';
export const inject = ['tools', 'sessions'];

type JsonSchema = Record<string, unknown>;
type JsonObject = Record<string, unknown>;

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

export interface LoomKnowledgeConfig extends PinnedKnowledgeContextConfig {
  resourceAvailable?: boolean;
}

export interface KnowledgeFilter {
  op: 'eq' | 'neq' | 'in' | 'exists' | 'missing' | 'prefix' | 'gt' | 'gte' | 'lt' | 'lte' | 'match';
  field: string;
  value?: string;
  values?: string[];
}

function objectArgs(raw: unknown): JsonObject {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new Error('tool arguments must be a JSON object');
  return raw as JsonObject;
}

function requiredString(args: JsonObject, name: string): string {
  const value = args[name];
  if (typeof value !== 'string' || !value.trim()) throw new Error(`${name} must be a non-empty string`);
  return value.trim();
}

function optionalStrings(args: JsonObject, name: string): string[] | undefined {
  const value = args[name];
  if (value === undefined) return undefined;
  if (!Array.isArray(value) || value.some((item) => typeof item !== 'string' || !item.trim())) {
    throw new Error(`${name} must be an array of non-empty strings`);
  }
  return value.map((item) => String(item).trim());
}

function optionalPositiveInteger(args: JsonObject, name: string, fallback?: number, maximum?: number): number | undefined {
  const value = args[name];
  if (value === undefined) return fallback;
  if (!Number.isInteger(value) || Number(value) <= 0) throw new Error(`${name} must be a positive integer`);
  if (maximum !== undefined && Number(value) > maximum) throw new Error(`${name} must not exceed ${maximum}`);
  return Number(value);
}

function output(schema: JsonSchema = {}): ToolDefinition['output'] {
  return {
    schema,
    render: (_args, value) => [{ type: 'text', text: JSON.stringify(value, null, 2) }],
  };
}

function appendFlag(flags: Record<string, unknown>, name: string, value: string): void {
  const current = flags[name];
  flags[name] = Array.isArray(current) ? [...current, value] : [value];
}

function searchFlags(raw: unknown): Record<string, unknown> {
  const args = objectArgs(raw);
  const flags: Record<string, unknown> = {};
  if (args.query !== undefined) flags.query = requiredString(args, 'query');
  if (args.matchMode !== undefined) flags['match-mode'] = requiredString(args, 'matchMode');
  const limit = optionalPositiveInteger(args, 'limit');
  if (limit !== undefined) flags.limit = limit;
  if (args.continuation !== undefined) flags.continuation = requiredString(args, 'continuation');
  const sort = optionalStrings(args, 'sort');
  if (sort) flags.sort = sort;
  if (args.filters !== undefined) {
    if (!Array.isArray(args.filters)) throw new Error('filters must be an array');
    for (const rawFilter of args.filters) {
      const filter = objectArgs(rawFilter) as Partial<KnowledgeFilter>;
      const op = typeof filter.op === 'string' ? filter.op : '';
      const field = typeof filter.field === 'string' ? filter.field.trim() : '';
      if (!field) throw new Error('filter.field must be a non-empty string');
      if (op === 'exists' || op === 'missing') {
        appendFlag(flags, op, field);
        continue;
      }
      if (op === 'in') {
        if (!Array.isArray(filter.values) || filter.values.length === 0 || filter.values.some((value) => typeof value !== 'string')) {
          throw new Error('in filter requires non-empty string values');
        }
        appendFlag(flags, 'in', `${field}=${filter.values.join(',')}`);
        continue;
      }
      if (!['eq', 'neq', 'prefix', 'gt', 'gte', 'lt', 'lte', 'match'].includes(op)) {
        throw new Error(`unsupported filter operation ${op || '<empty>'}`);
      }
      if (typeof filter.value !== 'string') throw new Error(`${op} filter requires a string value`);
      appendFlag(flags, op, `${field}=${filter.value}`);
    }
  }
  if (flags.query === undefined && flags.filters === undefined && Object.keys(flags).every((key) => ['limit', 'continuation', 'match-mode'].includes(key))) {
    throw new Error('knowledge_search requires query or filters');
  }
  return flags;
}

function actionableError(error: unknown, operation: string): never {
  if (!(error instanceof LoomError)) throw error;
  const guidance = error.code === 'CAPABILITY_UNSATISFIED'
    ? 'Inspect searchable fields with knowledge_schema; if this Workspace intentionally has no search projection, use the mounted files with rg or browse canonical IDs with knowledge_list.'
    : error.code === 'FORBIDDEN'
      ? 'This identity is not authorized for the selected Workspace; stop instead of retrying with another identity.'
      : error.code === 'KNOWLEDGE_REF_UNRESOLVED'
        ? 'Check the object ID with knowledge_search or knowledge_list; do not guess repository paths as object IDs.'
        : '';
  if (!guidance) throw error;
  throw new LoomError(`${operation} failed: ${error.message}. ${guidance}`, error.code);
}

function objectIDOf(value: unknown): string {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return '';
  const row = value as JsonObject;
  return typeof row.objectId === 'string' ? row.objectId : '';
}

export class LoomKnowledge {
  private readonly control: LoomControl;
  private readonly config: LoomKnowledgeConfig;

  constructor(config: LoomKnowledgeConfig = {}) {
    this.config = config;
    this.control = new LoomControl(config);
  }

  async context(exec: AgentToolRunContext): Promise<unknown> {
    const context = await pinnedKnowledgeContext(this.control, this.config, exec);
    return {
      identity: context.identity,
      ...(context.catalog ? { catalog: context.catalog } : {}),
      workspace: context.workspace,
      bindingSource: context.bindingSource,
      pin: context.pin,
      exposedInterfaces: [
        'knowledge_list', 'knowledge_read', 'knowledge_search', 'knowledge_schema',
        'knowledge_relations', 'knowledge_provenance', 'host_filesystem',
        ...(this.config.resourceAvailable ? ['resource'] : []),
      ],
      guidance: 'The Workspace and pin are already attached to every knowledge_* call; call this tool only when you need scope or identity diagnostics.',
    };
  }

  async list(raw: unknown, exec: AgentToolRunContext): Promise<unknown> {
    const args = objectArgs(raw);
    const prefix = args.objectPrefix === undefined ? '' : requiredString(args, 'objectPrefix');
    const limit = optionalPositiveInteger(args, 'limit', 50, 500)!;
    const context = await pinnedKnowledgeContext(this.control, this.config, exec);
    try {
      const result = await this.control.call({ verb: 'list', flags: scopedKnowledgeFlags(context, {}) }, exec.signal);
      if (!Array.isArray(result)) throw new Error('list returned an invalid response');
      const matching = prefix ? result.filter((item) => objectIDOf(item).startsWith(prefix)) : result;
      return {
        items: matching.slice(0, limit),
        returned: Math.min(matching.length, limit),
        matching: matching.length,
        truncated: matching.length > limit,
        ...(matching.length > limit ? {
          guidance: 'Narrow objectPrefix or use knowledge_search; this browse result is intentionally bounded for Agent context.',
        } : {}),
      };
    } catch (error) {
      actionableError(error, 'knowledge_list');
    }
  }

  async read(raw: unknown, exec: AgentToolRunContext): Promise<unknown> {
    const args = objectArgs(raw);
    const context = await pinnedKnowledgeContext(this.control, this.config, exec);
    const flags: Record<string, unknown> = { object: requiredString(args, 'object') };
    if (args.aspect !== undefined) flags.aspect = requiredString(args, 'aspect');
    const include = optionalStrings(args, 'include');
    const exclude = optionalStrings(args, 'exclude');
    if (include) flags.include = include;
    if (exclude) flags.exclude = exclude;
    try {
      return await this.control.call({ verb: 'read', flags: scopedKnowledgeFlags(context, flags) }, exec.signal);
    } catch (error) {
      actionableError(error, 'knowledge_read');
    }
  }

  async search(raw: unknown, exec: AgentToolRunContext): Promise<unknown> {
    const context = await pinnedKnowledgeContext(this.control, this.config, exec);
    try {
      return await this.control.call({ verb: 'search', flags: scopedKnowledgeFlags(context, searchFlags(raw)) }, exec.signal);
    } catch (error) {
      actionableError(error, 'knowledge_search');
    }
  }

  async schema(raw: unknown, exec: AgentToolRunContext): Promise<unknown> {
    const args = objectArgs(raw);
    const context = await pinnedKnowledgeContext(this.control, this.config, exec);
    const flags: Record<string, unknown> = {};
    if (args.object !== undefined) flags.object = requiredString(args, 'object');
    try {
      return await this.control.call({ verb: 'describe-schema', flags: scopedKnowledgeFlags(context, flags) }, exec.signal);
    } catch (error) {
      actionableError(error, 'knowledge_schema');
    }
  }

  async relations(raw: unknown, exec: AgentToolRunContext): Promise<unknown> {
    const args = objectArgs(raw);
    const context = await pinnedKnowledgeContext(this.control, this.config, exec);
    const flags: Record<string, unknown> = { object: requiredString(args, 'object') };
    if (args.relationType !== undefined) flags['relation-type'] = requiredString(args, 'relationType');
    if (args.role !== undefined) flags.role = requiredString(args, 'role');
    try {
      return await this.control.call({ verb: 'relations', flags: scopedKnowledgeFlags(context, flags) }, exec.signal);
    } catch (error) {
      actionableError(error, 'knowledge_relations');
    }
  }

  async provenance(raw: unknown, exec: AgentToolRunContext): Promise<unknown> {
    const args = objectArgs(raw);
    const context = await pinnedKnowledgeContext(this.control, this.config, exec);
    const flags: Record<string, unknown> = { object: requiredString(args, 'object') };
    if (args.aspect !== undefined) flags.aspect = requiredString(args, 'aspect');
    try {
      return await this.control.call({ verb: 'provenance', flags: scopedKnowledgeFlags(context, flags) }, exec.signal);
    } catch (error) {
      actionableError(error, 'knowledge_provenance');
    }
  }

  dispose(): void {
    this.control.dispose();
  }
}

const objectSelectorSchema: JsonSchema = {
  type: 'object',
  properties: {
    object: { type: 'string', description: 'Knowledge object_id in the bound Workspace.' },
    aspect: { type: 'string', description: 'Optional exact Aspect name.' },
  },
  required: ['object'],
  additionalProperties: false,
};

export function apply(ctx: Context, config: LoomKnowledgeConfig = {}): void {
  const tools = (ctx as unknown as { tools: ToolRegistry }).tools;
  const knowledge = new LoomKnowledge(config);
  ctx.effect(() => {
    const stopObservingTasks = observePinnedKnowledgeContextLifecycle(ctx);
    const unregister = [
      tools.register({
        name: 'knowledge_context',
        description: 'Diagnose the automatically bound identity, Workspace, immutable task pin, and available knowledge surfaces. Normal reads and searches do not require this first.',
        parameters: { type: 'object', additionalProperties: false },
        output: output({ type: 'object' }),
        isConcurrencySafe: () => true,
        execute: (_raw, exec) => knowledge.context(exec),
      }),
      tools.register({
        name: 'knowledge_list',
        description: 'Browse canonical knowledge object IDs at the fixed Workspace pin when the exact ID is unknown. Results are bounded; prefer search or an object prefix for large catalogs.',
        parameters: {
          type: 'object', additionalProperties: false,
          properties: {
            objectPrefix: { type: 'string', description: 'Optional object_id prefix, such as policy/ or schema/.' },
            limit: { type: 'integer', minimum: 1, maximum: 500, default: 50 },
          },
        },
        output: output({ type: 'object' }),
        isConcurrencySafe: () => true,
        execute: (raw, exec) => knowledge.list(raw, exec),
      }),
      tools.register({
        name: 'knowledge_read',
        description: 'Read one canonical knowledge object from this task’s fixed Workspace pin.',
        parameters: {
          ...objectSelectorSchema,
          properties: {
            ...(objectSelectorSchema.properties as JsonObject),
            include: { type: 'array', items: { type: 'string' }, description: 'Optional Aspect include selectors.' },
            exclude: { type: 'array', items: { type: 'string' }, description: 'Optional Aspect exclude selectors.' },
          },
        },
        output: output({ type: 'array' }),
        isConcurrencySafe: () => true,
        execute: (raw, exec) => knowledge.read(raw, exec),
      }),
      tools.register({
        name: 'knowledge_search',
        description: 'Search the bound Workspace at this task’s fixed pin; returned hits are canonical hydrated knowledge.',
        parameters: {
          type: 'object', additionalProperties: false,
          properties: {
            query: { type: 'string' },
            matchMode: { type: 'string', enum: ['AllTerms', 'AnyTerms', 'Phrase'] },
            filters: {
              type: 'array', items: {
                type: 'object', additionalProperties: false, required: ['op', 'field'],
                properties: {
                  op: { type: 'string', enum: ['eq', 'neq', 'in', 'exists', 'missing', 'prefix', 'gt', 'gte', 'lt', 'lte', 'match'] },
                  field: { type: 'string', description: 'Exact field identity returned by knowledge_schema; do not guess an ambiguous bare path.' },
                  value: { type: 'string' },
                  values: { type: 'array', items: { type: 'string' } },
                },
              },
            },
            sort: { type: 'array', items: { type: 'string' }, description: 'Exact sortable field identity, optionally suffixed :asc or :desc.' },
            limit: { type: 'integer', minimum: 1 }, continuation: { type: 'string' },
          },
          anyOf: [{ required: ['query'] }, { required: ['filters'] }],
        },
        output: output({ type: 'object' }),
        isConcurrencySafe: () => true,
        execute: (raw, exec) => knowledge.search(raw, exec),
      }),
      tools.register({
        name: 'knowledge_schema',
        description: 'Describe schemas and their text/filter/sort field identities at the fixed Workspace pin. Omit object to discover all visible schemas.',
        parameters: {
          type: 'object', additionalProperties: false,
          properties: {
            object: { type: 'string', description: 'Optional schema object_id or knowledge object whose referenced schemas should be described.' },
          },
        },
        output: output({ type: 'array' }),
        isConcurrencySafe: () => true,
        execute: (raw, exec) => knowledge.schema(raw, exec),
      }),
      tools.register({
        name: 'knowledge_relations',
        description: 'Read one-hop canonical relations touching an object at the fixed Workspace pin.',
        parameters: {
          type: 'object', additionalProperties: false, required: ['object'],
          properties: {
            object: { type: 'string', description: 'Endpoint object_id.' },
            relationType: { type: 'string', description: 'Optional exact relation type.' },
            role: { type: 'string', description: 'Optional endpoint role.' },
          },
        },
        output: output({ type: 'array' }),
        isConcurrencySafe: () => true,
        execute: (raw, exec) => knowledge.relations(raw, exec),
      }),
      tools.register({
        name: 'knowledge_provenance',
        description: 'Read origin envelopes for one object from this task’s fixed Workspace pin.',
        parameters: objectSelectorSchema,
        output: output({ type: 'array' }),
        isConcurrencySafe: () => true,
        execute: (raw, exec) => knowledge.provenance(raw, exec),
      }),
    ];
    return () => {
      stopObservingTasks();
      unregister.reverse().forEach((dispose) => dispose());
      knowledge.dispose();
    };
  }, 'dsh-loom: typed Knowledge consumer tools');
}
