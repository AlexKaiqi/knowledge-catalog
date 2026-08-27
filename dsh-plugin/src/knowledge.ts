import { createHash } from 'node:crypto';
import type { Context } from '@deepseek-ai/cordis';
import { LoomError } from './client.js';
import { LoomControl } from './control.js';
import {
  observePinnedKnowledgeContextLifecycle,
  pinnedKnowledgeContext,
  selectedKnowledgeBinding,
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
  const guidance = error.code === 'CAPABILITY_UNSATISFIED' && operation === 'knowledge_read'
    ? 'If this object contains a State Binding, configure the Knowledge Serving State runtime; Stream Bindings require an explicit resource/window operation. Mounted files expose the fixed declaration and are not a substitute for the bound value.'
    : error.code === 'CAPABILITY_UNSATISFIED'
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

function stringProperty(value: unknown, name: string): string {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return '';
  const property = (value as JsonObject)[name];
  return typeof property === 'string' && property.trim() ? property.trim() : '';
}

function knowledgeListSummary(value: unknown): JsonObject {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
  const row = value as JsonObject;
  const body = row.value && typeof row.value === 'object' && !Array.isArray(row.value)
    ? row.value as JsonObject
    : {};
  const properties = body.properties && typeof body.properties === 'object' && !Array.isArray(body.properties)
    ? body.properties
    : {};
  const definition = body.definition && typeof body.definition === 'object' && !Array.isArray(body.definition)
    ? body.definition
    : {};
  const units = Array.isArray(row.units) ? row.units : [];
  const declarations = Array.isArray(row.declarations) ? row.declarations : [];
  const aspects = [...new Set(units.map((unit) => stringProperty(unit, 'aspectName')).filter(Boolean))];
  const schemaRefs = [...new Set(declarations.map((declaration) => stringProperty(declaration, 'schemaRef')).filter(Boolean))];
  const name = stringProperty(properties, 'name') || stringProperty(definition, 'name');
  const entityType = stringProperty(properties, 'entityType');
  const qualifiedName = stringProperty(properties, 'qualifiedName');
  const nativeKind = stringProperty(properties, 'nativeKind');
  const addressKind = stringProperty(row.address, 'kind');
  return {
    objectId: objectIDOf(row),
    ...(typeof row.repository === 'string' ? { repository: row.repository } : {}),
    ...(addressKind ? { addressKind } : {}),
    ...(entityType ? { entityType } : {}),
    ...(name ? { name } : {}),
    ...(qualifiedName ? { qualifiedName } : {}),
    ...(nativeKind ? { nativeKind } : {}),
    ...(aspects.length ? { aspects } : {}),
    ...(schemaRefs.length ? { schemaRefs } : {}),
  };
}

interface KnowledgeListPage {
  values: unknown[];
  continuation: string;
  exhausted: boolean;
}

interface StoredListContinuation {
  token: string;
  hostTaskID: string;
}

function knowledgeListPageOf(result: unknown): KnowledgeListPage {
  // Keep compatibility with a pre-pagination kc service while treating the
  // current explicit page envelope as the canonical response.
  if (Array.isArray(result)) return { values: result, continuation: '', exhausted: true };
  if (!result || typeof result !== 'object') throw new Error('list returned an invalid response');
  const page = result as JsonObject;
  if (!Array.isArray(page.values) || typeof page.exhausted !== 'boolean') {
    throw new Error('list returned an invalid response');
  }
  const continuation = typeof page.continuation === 'string' ? page.continuation : '';
  if (!page.exhausted && !continuation) {
    throw new Error('list returned a non-advancing page');
  }
  return { values: page.values, continuation, exhausted: page.exhausted };
}

function isUninitializedHome(error: unknown): error is LoomError {
  return error instanceof LoomError
    && error.code === 'USAGE_INVALID'
    && /no kc home\b/i.test(error.message);
}

export class LoomKnowledge {
  private readonly control: LoomControl;
  private readonly config: LoomKnowledgeConfig;
  private readonly listContinuations = new Map<string, StoredListContinuation>();

  constructor(config: LoomKnowledgeConfig = {}) {
    this.config = config;
    this.control = new LoomControl(config);
  }

  private hostTaskID(exec: AgentToolRunContext): string {
    return exec.agent?.session.header.id?.trim() || '';
  }

  private storeListContinuation(token: string, exec: AgentToolRunContext): string {
    const hostTaskID = this.hostTaskID(exec);
    const handle = `page-${createHash('sha256').update(`${hostTaskID}\0${token}`).digest('hex').slice(0, 16)}`;
    // Refresh insertion order and keep the process-local handle table bounded.
    this.listContinuations.delete(handle);
    this.listContinuations.set(handle, { token, hostTaskID });
    while (this.listContinuations.size > 256) {
      const oldest = this.listContinuations.keys().next().value as string | undefined;
      if (!oldest) break;
      this.listContinuations.delete(oldest);
    }
    return handle;
  }

  private loadListContinuation(handle: string, exec: AgentToolRunContext): string {
    const stored = this.listContinuations.get(handle);
    if (!stored || stored.hostTaskID !== this.hostTaskID(exec)) {
      throw new Error('continuation is unknown or belongs to another Agent task; restart knowledge_list without continuation');
    }
    return stored.token;
  }

  async context(exec: AgentToolRunContext): Promise<unknown> {
    try {
      const context = await pinnedKnowledgeContext(this.control, this.config, exec);
      return {
        state: 'ready',
        identity: context.identity,
        ...(context.catalog ? { catalog: context.catalog } : {}),
        workspace: context.workspace,
        bindingSource: context.bindingSource,
        pin: context.pin,
        capabilities: context.capabilities,
        exposedInterfaces: [
          'knowledge_list', 'knowledge_read',
          ...(context.capabilities.search === false ? [] : ['knowledge_search']),
          'knowledge_schema',
          'knowledge_relations', 'knowledge_provenance', 'host_filesystem',
          ...(this.config.resourceAvailable ? ['resource'] : []),
        ],
        guidance: context.capabilities.search === false
          ? 'This Workspace uses index:none, so do not call knowledge_search. knowledge_list is bounded enumeration, not scalable discovery. The Workspace and pin are already attached to every knowledge_* call.'
          : 'The Workspace and pin are already attached to every knowledge_* call.',
      };
    } catch (error) {
      if (!isUninitializedHome(error)) throw error;
      const binding = await selectedKnowledgeBinding(this.config, exec);
      return {
        state: 'uninitialized',
        ...(binding.catalog ? { catalog: binding.catalog } : {}),
        workspace: binding.workspace,
        bindingSource: binding.bindingSource,
        exposedInterfaces: ['kc'],
        guidance: 'This kc home has not been initialized. Use the kc control tool to initialize the Catalog, add repositories, publish knowledge, and define this Workspace before calling knowledge_* read tools.',
      };
    }
  }

  async list(raw: unknown, exec: AgentToolRunContext): Promise<unknown> {
    const args = objectArgs(raw);
    const requestedContinuation = args.continuation === undefined ? '' : requiredString(args, 'continuation');
    const limit = optionalPositiveInteger(args, 'limit', 50, 500)!;
    const context = await pinnedKnowledgeContext(this.control, this.config, exec);
    try {
      const serverContinuation = requestedContinuation
        ? this.loadListContinuation(requestedContinuation, exec)
        : '';
      const page = knowledgeListPageOf(await this.control.call({
        verb: 'list',
        flags: scopedKnowledgeFlags(context, {
          limit,
          ...(serverContinuation ? { continuation: serverContinuation } : {}),
        }),
      }, exec.signal));
      const continuation = page.continuation
        ? this.storeListContinuation(page.continuation, exec)
        : '';
      return {
        items: page.values.map(knowledgeListSummary),
        returned: page.values.length,
        exhausted: page.exhausted,
        truncated: !page.exhausted,
        ...(continuation ? { continuation } : {}),
        ...(!page.exhausted ? {
          guidance: 'Continue only while a bounded enumeration is appropriate. Use knowledge_search for scalable discovery.',
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
    if (context.capabilities.search === false) {
      throw new LoomError(
        'knowledge_search is unavailable because the current KC store profile uses index:none; knowledge_list is only bounded enumeration',
        'CAPABILITY_UNSATISFIED',
      );
    }
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
    this.listContinuations.clear();
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
        description: 'Diagnose the automatically bound identity, Workspace, immutable task pin, and available knowledge surfaces, including whether SEARCH is configured. Normal exact reads do not require this first.',
        parameters: { type: 'object', additionalProperties: false },
        output: output({ type: 'object' }),
        isConcurrencySafe: () => true,
        execute: (_raw, exec) => knowledge.context(exec),
      }),
      tools.register({
        name: 'knowledge_list',
        description: 'Enumerate one bounded page of lightweight canonical object summaries at the fixed Workspace pin. This is not scalable discovery; use knowledge_search when available. Use continuation for the next page.',
        parameters: {
          type: 'object', additionalProperties: false,
          properties: {
            limit: { type: 'integer', minimum: 1, maximum: 500, default: 50 },
            continuation: { type: 'string', description: 'Short continuation handle returned by a prior unfiltered knowledge_list call in this Agent task.' },
          },
        },
        output: output({ type: 'object' }),
        isConcurrencySafe: () => true,
        execute: (raw, exec) => knowledge.list(raw, exec),
      }),
      tools.register({
        name: 'knowledge_read',
        description: 'Read one logical knowledge object from this task’s fixed Workspace pin. Snapshot values stay pinned; State Bindings are hydrated by Knowledge Serving with an observation basis.',
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
