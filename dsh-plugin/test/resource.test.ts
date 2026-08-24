import { describe, expect, it, vi } from 'vitest';
import { LoomResourceAccess } from '../src/resource.js';

function response(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });
}

describe('ResourceDescriptor access tool', () => {
  it('pins the Descriptor read and forwards fixed identity plus Agent session trace context', async () => {
    const seen: Array<{ url: string; init?: RequestInit }> = [];
    const fetchImpl = vi.fn(async (url: string, init?: RequestInit) => {
      seen.push({ url, init });
      if (url === 'http://kc/health') return response(200, { ok: true });
      if (url === 'http://kc/v1/read') {
        return response(200, [{
          objectId: 'resource/traces/payment-api', repository: 'kr://acme/payments', commit: 'abc123',
          value: {
            kind: 'ResourceDescriptor', runtime: 'payment-ops', protocol: 'resource-access/v1',
            access: { lookup: { call: 'lookup' } },
          },
        }]);
      }
      if (url === 'http://runtime/v1/access') return response(200, { traceId: 'access-1', result: { records: [] } });
      return response(404, {});
    });
    const access = new LoomResourceAccess({
      baseURL: 'http://kc', accessURL: 'http://runtime', as: 'consumer', autoStart: false,
      fetchImpl: fetchImpl as typeof fetch,
    });

    const result = await access.call(
      { descriptor: 'resource/traces/payment-api', operation: 'lookup', input: { traceId: 'trace-1' }, requestId: 'ask-1' },
      {
        signal: new AbortController().signal,
        agent: { session: { header: { id: 'session-7', agentPreset: 'dsh-loom', delegationDepth: 0 } } },
      },
      { catalog: 'kr://acme/catalog', workspace: 'payments-agent' },
    );

    expect(result).toEqual({ traceId: 'access-1', result: { records: [] } });
    const read = seen.find((call) => call.url === 'http://kc/v1/read')!;
    expect(JSON.parse(String(read.init?.body))).toEqual({
      catalog: 'kr://acme/catalog', workspace: 'payments-agent', object: 'resource/traces/payment-api',
    });
    const runtime = seen.find((call) => call.url === 'http://runtime/v1/access')!;
    expect(runtime.init?.headers).toMatchObject({
      'X-Resource-Principal': 'consumer', 'X-Resource-Request-Id': 'ask-1',
      'X-Agent-Session': 'session-7', 'X-Agent-Preset': 'dsh-loom',
    });
    expect(JSON.parse(String(runtime.init?.body))).toMatchObject({
      descriptor: { objectId: 'resource/traces/payment-api', repository: 'kr://acme/payments', commit: 'abc123' },
      runtime: 'payment-ops', protocol: 'resource-access/v1', operation: 'lookup', input: { traceId: 'trace-1' },
    });
    access.dispose();
  });

  it('rejects an operation not declared by the Descriptor before touching the runtime', async () => {
    const fetchImpl = vi.fn(async (url: string) => {
      if (url === 'http://kc/health') return response(200, { ok: true });
      if (url === 'http://kc/v1/read') {
        return response(200, [{
          objectId: 'resource/status/payment-api', repository: 'kr://acme/payments', commit: 'v1',
          value: { kind: 'ResourceDescriptor', runtime: 'payment-ops', protocol: 'resource-access/v1', access: { status: { call: 'status' } } },
        }]);
      }
      throw new Error(`unexpected runtime call ${url}`);
    });
    const access = new LoomResourceAccess({ baseURL: 'http://kc', accessURL: 'http://runtime', autoStart: false, fetchImpl: fetchImpl as typeof fetch });
    await expect(access.call(
      { descriptor: 'resource/status/payment-api', operation: 'delete' },
      { signal: new AbortController().signal },
      { workspace: 'payments-agent' },
    )).rejects.toThrow('not declared');
    expect(fetchImpl).toHaveBeenCalledTimes(2);
    access.dispose();
  });
});
