import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { LoomError } from '../src/client.js';
import { LoomKnowledge } from '../src/knowledge.js';
import { LoomResourceAccess } from '../src/resource.js';
import {
  clearPinnedKnowledgeContextsForTests,
  observePinnedKnowledgeContextLifecycle,
} from '../src/context.js';

function response(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });
}

describe('typed Agent knowledge tools', () => {
  beforeEach(() => clearPinnedKnowledgeContextsForTests());
	afterEach(() => vi.unstubAllEnvs());

  it('resolves identity and Workspace once, then reuses one pin across read, search, and provenance', async () => {
    const seen: Array<{ url: string; body: Record<string, unknown> }> = [];
    const pin = {
      workspaceId: 'agent', revision: 2, pinId: 'pin-2',
      repositories: { 'kr://acme/public/core': 'commit-2' },
    };
    const fetchImpl = vi.fn(async (url: string, init?: RequestInit) => {
      if (url.endsWith('/health')) return response(200, { ok: true });
      const body = JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>;
      seen.push({ url, body });
      if (url.endsWith('/v1/store-ls')) return response(200, { index: 'opensearch' });
      if (url.endsWith('/v1/resolve')) return response(200, pin);
      if (url.endsWith('/v1/whoami')) return response(200, { principal: 'agent:consumer', onBehalfOf: 'user:alice' });
      if (url.endsWith('/v1/read')) return response(200, [{ objectId: 'runbook/oncall', commit: 'commit-2' }]);
      if (url.endsWith('/v1/search')) return response(200, { searchView: { snapshots: pin.repositories }, hits: [] });
      if (url.endsWith('/v1/provenance')) return response(200, [{ objectId: 'runbook/oncall', commit: 'commit-2' }]);
      return response(404, {});
    });
    const knowledge = new LoomKnowledge({
      baseURL: 'http://kc', catalog: 'kr://acme/catalog', workspace: 'agent', autoStart: false,
      fetchImpl: fetchImpl as typeof fetch,
    });
    const exec = { signal: new AbortController().signal, agent: { session: { header: { id: 'session-1', cwd: '/tmp/project' } } } };

    const context = await knowledge.context(exec) as Record<string, unknown>;
    await knowledge.read({ object: 'runbook/oncall' }, exec);
    await knowledge.search({ query: 'freeze window', filters: [{ op: 'eq', field: 'status', value: 'active' }], limit: 5 }, exec);
    await knowledge.provenance({ object: 'runbook/oncall' }, exec);

    expect(context).toMatchObject({
      identity: { principal: 'agent:consumer', onBehalfOf: 'user:alice' },
      catalog: 'kr://acme/catalog', workspace: 'agent', bindingSource: 'configuration', pin,
      capabilities: { search: true, index: 'opensearch' },
    });
    expect(seen.filter((call) => call.url.endsWith('/v1/resolve'))).toHaveLength(1);
    expect(seen.filter((call) => ['/v1/read', '/v1/search', '/v1/provenance'].some((path) => call.url.endsWith(path))))
      .toHaveLength(3);
    for (const call of seen.filter((item) => !item.url.endsWith('/v1/resolve') && !item.url.endsWith('/v1/whoami') && !item.url.endsWith('/v1/store-ls'))) {
      expect(call.body).toMatchObject({ catalog: 'kr://acme/catalog', workspace: 'agent', pin });
    }
    const search = seen.find((call) => call.url.endsWith('/v1/search'))!;
    expect(search.body).toMatchObject({ query: 'freeze window', eq: ['status=active'], limit: 5 });
    knowledge.dispose();
  });

  it('fails before knowledge access when no Workspace is bound or configured', async () => {
	vi.stubEnv('KC_WORKSPACE', '');
	vi.stubEnv('KC_CATALOG', '');
    const fetchImpl = vi.fn(async (url: string) => url.endsWith('/health') ? response(200, { ok: true }) : response(500, {}));
    const knowledge = new LoomKnowledge({ baseURL: 'http://kc', autoStart: false, fetchImpl: fetchImpl as typeof fetch });
    await expect(knowledge.context({ signal: new AbortController().signal }))
      .rejects.toThrow('no Knowledge Workspace is selected');
    expect(fetchImpl).not.toHaveBeenCalled();
    knowledge.dispose();
  });

  it('reports an uninitialized kc home as first-run context instead of a failed Agent tool', async () => {
    const fetchImpl = vi.fn(async (url: string) => {
      if (url.endsWith('/health')) return response(200, { ok: true });
      if (url.endsWith('/v1/resolve')) return response(400, {
        error: { code: 'USAGE_INVALID', message: 'no kc home at /tmp/empty; run: kc init --home /tmp/empty' },
      });
      if (url.endsWith('/v1/whoami')) return response(200, { principal: 'owner' });
      return response(404, {});
    });
    const knowledge = new LoomKnowledge({
      baseURL: 'http://kc', catalog: 'kr://acme/catalog', workspace: 'agent', autoStart: false,
      fetchImpl: fetchImpl as typeof fetch,
    });

    await expect(knowledge.context({ signal: new AbortController().signal })).resolves.toEqual({
      state: 'uninitialized',
      catalog: 'kr://acme/catalog',
      workspace: 'agent',
      bindingSource: 'configuration',
      exposedInterfaces: ['kc'],
      guidance: expect.stringContaining('initialize'),
    });
    knowledge.dispose();
  });

  it('shares the same fixed task context with live resource access', async () => {
		const calls: string[] = [];
		const fetchImpl = vi.fn(async (url: string) => {
			calls.push(url);
			if (url.endsWith('/health')) return response(200, { ok: true });
			if (url.endsWith('/v1/resolve')) return response(200, {
				workspaceId: 'agent', revision: 1, pinId: 'shared-pin', repositories: { 'kr://acme/core': 'c1' },
			});
			if (url.endsWith('/v1/whoami')) return response(200, { principal: 'consumer' });
			return response(404, {});
		});
		const config = { baseURL: 'http://kc', catalog: 'kr://acme/catalog', workspace: 'agent', autoStart: false, fetchImpl: fetchImpl as typeof fetch };
		const knowledge = new LoomKnowledge(config);
		const resource = new LoomResourceAccess({ ...config, accessURL: 'http://runtime' });
		const exec = { signal: new AbortController().signal, agent: { session: { header: { id: 'shared-session' } } } };

		await knowledge.context(exec);
		const resourceContext = await resource.context(exec);
		expect(resourceContext.pin.pinId).toBe('shared-pin');
		expect(calls.filter((url) => url.endsWith('/v1/resolve'))).toHaveLength(1);
		expect(calls.filter((url) => url.endsWith('/v1/whoami'))).toHaveLength(1);
		knowledge.dispose();
		resource.dispose();
	});

  it('releases every cached context when the DSH host task is disposed', async () => {
    let revision = 0;
    const fetchImpl = vi.fn(async (url: string) => {
      if (url.endsWith('/health')) return response(200, { ok: true });
      if (url.endsWith('/v1/resolve')) {
        revision++;
        return response(200, {
          workspaceId: 'agent', revision,
          repositories: { 'kr://acme/core': `commit-${revision}` },
        });
      }
      if (url.endsWith('/v1/whoami')) return response(200, { principal: 'consumer' });
      return response(404, {});
    });
    const knowledge = new LoomKnowledge({
      baseURL: 'http://kc', workspace: 'agent', autoStart: false,
      fetchImpl: fetchImpl as typeof fetch,
    });
    const exec = { signal: new AbortController().signal, agent: { session: { header: { id: 'task-to-release' } } } };

    await expect(knowledge.context(exec)).resolves.toMatchObject({
      pin: { revision: 1, repositories: { 'kr://acme/core': 'commit-1' } },
    });
    await expect(knowledge.context(exec)).resolves.toMatchObject({ pin: { revision: 1 } });

    let notifyDisposed: ((task: { header: { id?: string } }) => void) | undefined;
    const stopObserving = observePinnedKnowledgeContextLifecycle({
      on(event: string, listener: (task: { header: { id?: string } }) => void, options?: { global?: boolean }) {
        expect(event).toBe('session/disposed');
        expect(options).toEqual({ global: true });
        notifyDisposed = listener;
        return () => true;
      },
    } as never);
    notifyDisposed!({ header: { id: 'task-to-release' } });

    await expect(knowledge.context(exec)).resolves.toMatchObject({
      pin: { revision: 2, repositories: { 'kr://acme/core': 'commit-2' } },
    });
    expect(fetchImpl.mock.calls.filter(([url]) => String(url).endsWith('/v1/resolve'))).toHaveLength(2);
    stopObserving();
    knowledge.dispose();
  });

  it('does not retain a global context for direct calls without a DSH task id', async () => {
    let revision = 0;
    const fetchImpl = vi.fn(async (url: string) => {
      if (url.endsWith('/health')) return response(200, { ok: true });
      if (url.endsWith('/v1/resolve')) return response(200, {
        workspaceId: 'agent', revision: ++revision,
        repositories: { 'kr://acme/core': `commit-${revision}` },
      });
      if (url.endsWith('/v1/whoami')) return response(200, { principal: 'consumer' });
      return response(404, {});
    });
    const knowledge = new LoomKnowledge({
      baseURL: 'http://kc', workspace: 'agent', autoStart: false,
      fetchImpl: fetchImpl as typeof fetch,
    });

    await expect(knowledge.context({ signal: new AbortController().signal })).resolves.toMatchObject({ pin: { revision: 1 } });
    await expect(knowledge.context({ signal: new AbortController().signal })).resolves.toMatchObject({ pin: { revision: 2 } });
    knowledge.dispose();
  });

  it('never shares a cached identity or pin across authentication tokens', async () => {
    const resolvePrincipals: string[] = [];
    const fetchImpl = vi.fn(async (url: string, init?: RequestInit) => {
      if (url.endsWith('/health')) return response(200, { ok: true });
      const authorization = (init?.headers as Record<string, string> | undefined)?.Authorization ?? '';
      const principal = authorization.endsWith('token-a') ? 'user:a' : 'user:b';
      if (url.endsWith('/v1/resolve')) {
        resolvePrincipals.push(principal);
        return response(200, {
          workspaceId: 'agent', revision: 1,
          repositories: { 'kr://acme/core': principal === 'user:a' ? 'commit-a' : 'commit-b' },
        });
      }
      if (url.endsWith('/v1/whoami')) return response(200, { principal });
      return response(404, {});
    });
    const shared = { baseURL: 'http://kc', workspace: 'agent', autoStart: false, fetchImpl: fetchImpl as typeof fetch };
    const a = new LoomKnowledge({ ...shared, authToken: 'token-a' });
    const b = new LoomKnowledge({ ...shared, authToken: 'token-b' });
    const exec = { signal: new AbortController().signal, agent: { session: { header: { id: 'same-agent-session' } } } };

    await expect(a.context(exec)).resolves.toMatchObject({
      identity: { principal: 'user:a' }, pin: { repositories: { 'kr://acme/core': 'commit-a' } },
    });
    await expect(b.context(exec)).resolves.toMatchObject({
      identity: { principal: 'user:b' }, pin: { repositories: { 'kr://acme/core': 'commit-b' } },
    });
    expect(resolvePrincipals).toEqual(['user:a', 'user:b']);
    a.dispose();
    b.dispose();
  });

  it('allows direct discovery without a mandatory context call', async () => {
    const calls: Array<{ url: string; body: Record<string, unknown> }> = [];
    const fetchImpl = vi.fn(async (url: string, init?: RequestInit) => {
      const body = JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>;
      calls.push({ url, body });
      if (url.endsWith('/health')) return response(200, { ok: true });
      if (url.endsWith('/v1/store-ls')) return response(200, { index: 'none' });
      if (url.endsWith('/v1/resolve')) return response(200, {
        workspaceId: 'agent', revision: 1, repositories: { 'kr://acme/core': 'c1' },
      });
      if (url.endsWith('/v1/whoami')) return response(200, { principal: 'consumer' });
      if (url.endsWith('/v1/list')) {
        if (body.continuation === 'page-2') return response(200, {
          values: [
            {
              objectId: 'policy/retention', repository: 'kr://acme/core', address: { kind: 'Entity' },
              value: { properties: { entityType: 'Policy', name: 'Retention', qualifiedName: 'policy.retention' } },
              units: [{ aspectName: 'properties' }], declarations: [{ schemaRef: 'schema/policy.properties' }],
            },
            { objectId: 'runbook/oncall', value: { properties: { name: 'On-call' } } },
          ],
          exhausted: true,
        });
        return response(200, {
          values: [{ objectId: 'column/orders/id', value: { properties: { name: 'id', entityType: 'Column' } } }],
          continuation: 'page-2',
          exhausted: false,
        });
      }
      if (url.endsWith('/v1/describe-schema')) return response(200, [{ schemas: [{ objectId: 'schema/policy' }] }]);
      if (url.endsWith('/v1/relations')) return response(200, [{ objectId: 'relation/policy-owner' }]);
      return response(404, {});
    });
    const knowledge = new LoomKnowledge({
      baseURL: 'http://kc', workspace: 'agent', autoStart: false, fetchImpl: fetchImpl as typeof fetch,
    });
    const exec = { signal: new AbortController().signal, agent: { session: { header: { id: 'discover-session' } } } };

    const first = await knowledge.list({ objectPrefix: '', limit: 3 }, exec) as Record<string, unknown>;
    expect(first).toMatchObject({
      returned: 1, exhausted: false, truncated: true,
      items: [{ objectId: 'column/orders/id', name: 'id', entityType: 'Column' }],
    });
    expect(first.continuation).toMatch(/^page-[0-9a-f]{16}$/);
    const continued = await knowledge.list({ continuation: first.continuation, limit: 3 }, exec) as Record<string, unknown>;
    expect(continued).toMatchObject({
      returned: 2, exhausted: true, truncated: false,
      items: [{
        objectId: 'policy/retention', repository: 'kr://acme/core', addressKind: 'Entity',
        entityType: 'Policy', name: 'Retention', qualifiedName: 'policy.retention',
        aspects: ['properties'], schemaRefs: ['schema/policy.properties'],
      }, { objectId: 'runbook/oncall', name: 'On-call' }],
    });
    expect(JSON.stringify(continued)).not.toContain('"value"');
    await expect(knowledge.list({ continuation: 'page-does-not-exist', limit: 3 }, exec))
      .rejects.toThrow('continuation is unknown or belongs to another Agent task');
    await expect(knowledge.list({ objectPrefix: 'policy/', limit: 1 }, exec)).resolves.toMatchObject({
      returned: 1, matching: 1, truncated: false, items: [{ objectId: 'policy/retention' }],
    });
    await knowledge.schema({}, exec);
    await knowledge.relations({ object: 'policy/retention', relationType: 'owned-by' }, exec);

    expect(calls.filter((call) => call.url.endsWith('/v1/resolve'))).toHaveLength(1);
    expect(calls.filter((call) => call.url.endsWith('/v1/list')).map((call) => call.body))
      .toEqual([
        expect.objectContaining({ workspace: 'agent', limit: 3 }),
        expect.objectContaining({ workspace: 'agent', limit: 3, continuation: 'page-2' }),
        expect.objectContaining({ workspace: 'agent', limit: 1000 }),
        expect.objectContaining({ workspace: 'agent', limit: 1000, continuation: 'page-2' }),
      ]);
    expect(calls.find((call) => call.url.endsWith('/v1/describe-schema'))?.body).toMatchObject({ workspace: 'agent' });
    expect(calls.find((call) => call.url.endsWith('/v1/relations'))?.body).toMatchObject({
      workspace: 'agent', object: 'policy/retention', 'relation-type': 'owned-by',
    });
    knowledge.dispose();
  });

  it('turns stable service failures into actionable Agent guidance', async () => {
    const fetchImpl = vi.fn(async (url: string) => {
      if (url.endsWith('/health')) return response(200, { ok: true });
      if (url.endsWith('/v1/resolve')) return response(200, {
        workspaceId: 'agent', revision: 1, repositories: { 'kr://acme/core': 'c1' },
      });
      if (url.endsWith('/v1/whoami')) return response(200, { principal: 'consumer' });
      if (url.endsWith('/v1/search')) return response(400, {
        error: { code: 'CAPABILITY_UNSATISFIED', message: 'SEARCH requires an OpenSearch projection' },
      });
      return response(404, {});
    });
    const knowledge = new LoomKnowledge({
      baseURL: 'http://kc', workspace: 'agent', autoStart: false, fetchImpl: fetchImpl as typeof fetch,
    });

    const error = await knowledge.search({ query: 'retention' }, { signal: new AbortController().signal })
      .catch((caught: unknown) => caught);
    expect(error).toBeInstanceOf(LoomError);
    expect(error).toMatchObject({ code: 'CAPABILITY_UNSATISFIED' });
    expect((error as Error).message).toContain('knowledge_schema');
    expect((error as Error).message).toContain('mounted files with rg');
    knowledge.dispose();
  });

  it('reports index:none and rejects SEARCH locally without a doomed service call', async () => {
    const calls: string[] = [];
    const fetchImpl = vi.fn(async (url: string) => {
      calls.push(url);
      if (url.endsWith('/health')) return response(200, { ok: true });
      if (url.endsWith('/v1/store-ls')) return response(200, { index: 'none' });
      if (url.endsWith('/v1/resolve')) return response(200, {
        workspaceId: 'agent', revision: 1, repositories: { 'kr://acme/core': 'c1' },
      });
      if (url.endsWith('/v1/whoami')) return response(200, { principal: 'consumer' });
      return response(404, {});
    });
    const knowledge = new LoomKnowledge({
      baseURL: 'http://kc', workspace: 'agent', autoStart: false, fetchImpl: fetchImpl as typeof fetch,
    });
    const exec = { signal: new AbortController().signal, agent: { session: { header: { id: 'index-none' } } } };

    await expect(knowledge.context(exec)).resolves.toMatchObject({
      capabilities: { search: false, index: 'none' },
      exposedInterfaces: expect.not.arrayContaining(['knowledge_search']),
      guidance: expect.stringContaining('do not call knowledge_search'),
    });
    await expect(knowledge.search({ query: 'retention' }, exec)).rejects.toMatchObject({
      code: 'CAPABILITY_UNSATISFIED',
    });
    expect(calls.some((url) => url.endsWith('/v1/search'))).toBe(false);
    knowledge.dispose();
  });

  it('explains a bound read capability failure as a missing State runtime, not a search failure', async () => {
    const fetchImpl = vi.fn(async (url: string) => {
      if (url.endsWith('/health')) return response(200, { ok: true });
      if (url.endsWith('/v1/resolve')) return response(200, {
        workspaceId: 'agent', revision: 1, repositories: { 'kr://acme/core': 'c1' },
      });
      if (url.endsWith('/v1/whoami')) return response(200, { principal: 'consumer' });
      if (url.endsWith('/v1/read')) return response(400, {
        error: { code: 'CAPABILITY_UNSATISFIED', message: 'State Binding requires a Materialization Runtime' },
      });
      return response(404, {});
    });
    const knowledge = new LoomKnowledge({
      baseURL: 'http://kc', workspace: 'agent', autoStart: false, fetchImpl: fetchImpl as typeof fetch,
    });

    const error = await knowledge.read({ object: 'Service:payments' }, { signal: new AbortController().signal })
      .catch((caught: unknown) => caught);
    expect(error).toBeInstanceOf(LoomError);
    expect(error).toMatchObject({ code: 'CAPABILITY_UNSATISFIED' });
    expect((error as Error).message).toContain('State runtime');
    expect((error as Error).message).toContain('Mounted files expose the fixed declaration');
    expect((error as Error).message).not.toContain('knowledge_schema');
    knowledge.dispose();
  });
});
