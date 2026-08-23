import { afterEach, describe, expect, it, vi } from 'vitest';
import { LoomControl } from '../src/control.js';

function response(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });
}

describe('Knowledge Catalog control tool client', () => {
  const controls: LoomControl[] = [];
  afterEach(() => controls.splice(0).forEach((control) => control.dispose()));

  it('fixes actor context, stamps a request, and strips model-supplied process/identity flags', async () => {
    const seen: Array<{ url: string; init?: RequestInit }> = [];
    const fetchImpl = vi.fn(async (url: string, init?: RequestInit) => {
      seen.push({ url, init });
      if (url.endsWith('/health')) return response(200, { ok: true });
      return response(200, { ok: true });
    });
    const control = new LoomControl({ baseURL: 'http://127.0.0.1:7380', as: 'producer', fetchImpl: fetchImpl as typeof fetch });
    controls.push(control);

    await control.call({
      verb: 'put',
      requestId: 'producer-1',
      flags: { repo: 'kr://acme/public/core', as: '', home: '/tmp/escape', listen: '0.0.0.0:1' },
    });

    const call = seen.at(-1)!;
    const headers = call.init?.headers as Record<string, string>;
    expect(headers['X-Kc-As']).toBe('producer');
    expect(headers['X-Kc-Request-Id']).toBe('producer-1');
    expect(JSON.parse(String(call.init?.body))).toEqual({ repo: 'kr://acme/public/core' });
  });

  it('preserves kc stable error codes', async () => {
    const fetchImpl = vi.fn(async (url: string) => {
      if (url.endsWith('/health')) return response(200, { ok: true });
      return response(403, { error: { code: 'FORBIDDEN', message: 'mallory is not allowed to put' } });
    });
    const control = new LoomControl({ as: 'mallory', fetchImpl: fetchImpl as typeof fetch });
    controls.push(control);
    await expect(control.call({ verb: 'put', flags: {} })).rejects.toMatchObject({ code: 'FORBIDDEN' });
  });
});
