import { createHash, randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { mkdir, readFile, rename, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';

type Endpoint = { server?: string };
type TokenSession = Endpoint & { principal?: string; access_token?: string; refresh_token?: string; expires_at?: string };
type LocalSession = Endpoint & { principal?: string };
type TokenResponse = { accessToken?: string; refreshToken?: string; expiresIn?: number };

export class BrowserIdentityError extends Error {
  constructor(readonly code: string, message: string) { super(message); }
}

function configDir(): string { return process.env.KC_CONFIG_DIR?.trim() || path.join(os.homedir(), '.config', 'kc'); }
function normalizeServer(value: string): string { return value.trim().replace(/\/+$/, ''); }
function sessionPath(server: string, file: string): string {
  return path.join(configDir(), 'sessions', createHash('sha256').update(normalizeServer(server)).digest('hex'), file);
}

// The same non-secret entrypoint configuration and precedence as kc. Keep the
// resolved URL with a task pin so a later login cannot silently move its source.
export function resolveBrowserServer(input?: string): string {
  const configured = input?.trim() || process.env.KC_SERVER_URL?.trim();
  if (configured) return normalizeServer(configured);
  for (const filename of ['client.json', 'session-taihu.json', 'session-local.json']) {
    try {
      const value = JSON.parse(readFileSync(path.join(configDir(), filename), 'utf8')) as Endpoint;
      if (value.server?.trim()) return normalizeServer(value.server);
    } catch { /* Missing packaging config does not prevent local file viewing. */ }
  }
  return '';
}

async function readSession<T extends Endpoint>(server: string, filename: string): Promise<T | undefined> {
  for (const file of [sessionPath(server, filename), path.join(configDir(), filename)]) {
    let content: string;
    try { content = await readFile(file, 'utf8'); }
    catch (error) {
      if ((error as NodeJS.ErrnoException).code === 'ENOENT') continue;
      throw new BrowserIdentityError('UNAUTHENTICATED', '无法读取 KC 登录状态，请重新执行 kc login。');
    }
    let value: T;
    try { value = JSON.parse(content) as T; }
    catch { throw new BrowserIdentityError('UNAUTHENTICATED', 'KC 登录状态无效，请重新执行 kc login。'); }
    if (value.server?.trim() && normalizeServer(value.server) === normalizeServer(server)) return value;
  }
  return undefined;
}

const refreshing = new Map<string, Promise<TokenSession>>();

async function refreshSession(server: string, session: TokenSession): Promise<TokenSession> {
  const existing = refreshing.get(server);
  if (existing) return existing;
  const pending = (async () => {
    if (!session.refresh_token) throw new BrowserIdentityError('UNAUTHENTICATED', 'KC 登录已过期，请执行 kc login 后重试。');
    const response = await fetch(`${server}/identity/v1/token`, {
      method: 'POST', redirect: 'error', signal: AbortSignal.timeout(30_000),
      headers: { 'content-type': 'application/json', accept: 'application/json' },
      body: JSON.stringify({ grantType: 'refresh_token', refreshToken: session.refresh_token }),
    });
    if (!response.ok) throw new BrowserIdentityError('UNAUTHENTICATED', 'KC 登录刷新失败，请重新执行 kc login。');
    const token = await response.json() as TokenResponse;
    if (!token.accessToken || !token.expiresIn || token.expiresIn <= 0) throw new BrowserIdentityError('UNAUTHENTICATED', 'KC 登录刷新未返回有效凭证，请重新执行 kc login。');
    const whoami = await fetch(`${server}/identity/v1/whoami`, {
      redirect: 'error', signal: AbortSignal.timeout(30_000),
      headers: { authorization: `Bearer ${token.accessToken}`, accept: 'application/json' },
    });
    if (!whoami.ok) throw new BrowserIdentityError('UNAUTHENTICATED', 'KC 无法验证刷新后的身份，请重新执行 kc login。');
    const identity = await whoami.json() as { principal?: string };
    if (!identity.principal?.trim() || (session.principal && identity.principal !== session.principal)) throw new BrowserIdentityError('UNAUTHENTICATED', 'KC 刷新后的身份不匹配，请重新执行 kc login。');
    const updated: TokenSession = {
      ...session, server, principal: identity.principal, access_token: token.accessToken,
      refresh_token: token.refreshToken || session.refresh_token,
      expires_at: new Date(Date.now() + token.expiresIn * 1000).toISOString(),
    };
    const file = sessionPath(server, 'session-taihu.json');
    await mkdir(path.dirname(file), { recursive: true, mode: 0o700 });
    const temporary = `${file}.${randomUUID()}.tmp`;
    try { await writeFile(temporary, JSON.stringify(updated, null, 2), { mode: 0o600 }); await rename(temporary, file); }
    finally { await rm(temporary, { force: true }); }
    return updated;
  })();
  refreshing.set(server, pending);
  try { return await pending; } finally { refreshing.delete(server); }
}

// Called in the host on each request. Credentials never become browser JSON,
// task context, mount argv, or a pin; Bearer pairing does not assert principal.
export async function resolveBrowserIdentity(config: { server?: string; principal?: string }): Promise<{ server: string; principal: string; authorization?: string }> {
  const server = resolveBrowserServer(config.server);
  const principal = config.principal?.trim() || process.env.KC_AS?.trim() || '';
  let token = process.env.KC_AUTH_TOKEN?.trim();
  if (!token && server) {
    let session = await readSession<TokenSession>(server, 'session-taihu.json');
    if (session) {
      if (!session.access_token) throw new BrowserIdentityError('UNAUTHENTICATED', 'KC 登录凭证缺失，请重新执行 kc login。');
      const expiry = session.expires_at ? Date.parse(session.expires_at) : 0;
      // Go's zero time marks a PAT with no advertised expiry.
      if (session.expires_at && !session.expires_at.startsWith('0001-') && (!Number.isFinite(expiry) || expiry <= Date.now())) session = await refreshSession(server, session);
      token = session.access_token;
    }
  }
  if (token) {
    if (principal) throw new BrowserIdentityError('USAGE_INVALID', 'Bearer 登录不能同时设置 KC_AS 或 principal。');
    return { server, principal: '', authorization: /^Bearer\s+/i.test(token) ? token.replace(/^Bearer\s+/i, 'Bearer ') : `Bearer ${token}` };
  }
  if (principal || !server) return { server, principal };
  const local = await readSession<LocalSession>(server, 'session-local.json');
  return { server, principal: local?.principal?.trim() || '' };
}
