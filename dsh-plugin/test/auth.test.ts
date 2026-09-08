import { createHash } from 'node:crypto';
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { resolveBrowserIdentity, resolveBrowserServer } from '../src/auth.js';

describe('KC shared client login', () => {
  let root: string;
  beforeEach(async () => {
    root = await mkdtemp(path.join(os.tmpdir(), 'kc-browser-auth-'));
    vi.stubEnv('KC_CONFIG_DIR', root);
    vi.stubEnv('KC_SERVER_URL', '');
    vi.stubEnv('KC_AUTH_TOKEN', '');
    vi.stubEnv('KC_AS', '');
  });
  afterEach(async () => { vi.unstubAllEnvs(); await rm(root, { recursive: true, force: true }); });

  it('inherits the delivered endpoint and only its matching saved credential', async () => {
    await writeFile(path.join(root, 'client.json'), JSON.stringify({ server: 'http://kc-a/' }));
    const directory = path.join(root, 'sessions', createHash('sha256').update('http://kc-a').digest('hex'));
    await mkdir(directory, { recursive: true });
    await writeFile(path.join(directory, 'session-taihu.json'), JSON.stringify({ server: 'http://kc-a', principal: 'alice', access_token: 'alice-token', expires_at: '0001-01-01T00:00:00Z' }));
    expect(resolveBrowserServer()).toBe('http://kc-a');
    expect(await resolveBrowserIdentity({})).toEqual({ server: 'http://kc-a', principal: '', authorization: 'Bearer alice-token' });
    expect(await resolveBrowserIdentity({ server: 'http://kc-b' })).toEqual({ server: 'http://kc-b', principal: '' });
  });

  it('rejects expired sessions without falling back to a local identity', async () => {
    await writeFile(path.join(root, 'session-taihu.json'), JSON.stringify({ server: 'http://kc-a', access_token: 'expired', expires_at: '2020-01-01T00:00:00Z' }));
    vi.stubEnv('KC_AS', 'forged');
    await expect(resolveBrowserIdentity({ server: 'http://kc-a' })).rejects.toMatchObject({ code: 'UNAUTHENTICATED' });
    expect(await resolveBrowserIdentity({ server: 'http://kc-b' })).toEqual({ server: 'http://kc-b', principal: 'forged' });
  });

  it('rejects mixed credentials and legacy tokens with no matching server', async () => {
    await writeFile(path.join(root, 'session-taihu.json'), JSON.stringify({ access_token: 'unbound-secret' }));
    expect(await resolveBrowserIdentity({ server: 'http://kc-a' })).toEqual({ server: 'http://kc-a', principal: '' });
    vi.stubEnv('KC_AUTH_TOKEN', 'explicit');
    vi.stubEnv('KC_AS', 'alice');
    await expect(resolveBrowserIdentity({ server: 'http://kc-a' })).rejects.toMatchObject({ code: 'USAGE_INVALID' });
  });
});
