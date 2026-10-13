import {userHeaders, userProof} from './sessionProof';
export type Fault = { code: string; message: string; details?: Record<string, unknown> };

export class KCError extends Error {
    constructor(readonly status: number, readonly fault?: Fault) {
        super(fault?.message ?? `请求失败（${status}）`);
        this.name = 'KCError';
    }
    get code(): string { return this.fault?.code ?? 'TRANSPORT'; }
}

// The entire console shares caller proof from the existing identity provider.
// Redirects are rejected so credentials cannot follow a different endpoint.
export type RequestMethod = 'GET' | 'POST' | 'PUT' | 'DELETE';
export async function jsonRequest<T>(path: string, body?: unknown, signal?: AbortSignal, method: RequestMethod = body === undefined ? 'GET' : 'POST'): Promise<T> {
	const epoch = userProof.version();
	const broker = /^\/identity\/v1\/(auth|authorize(?::poll)?|token)$/.test(path);
	let proof: Record<string, string> = {};
	try { if (!broker) proof = await userHeaders(); }
	catch { throw new KCError(401, {code: 'UNAUTHENTICATED', message: '认证已失效，请重新认证。'}); }
    const response = await fetch(path, {
        method,
        headers: { Accept: 'application/json', 'X-Kc-Console': '1', ...proof, ...(method === 'GET' ? {} : { 'Content-Type': 'application/json' }) },
        body: body === undefined ? undefined : JSON.stringify(body),
        credentials: 'same-origin', redirect: 'error', cache: 'no-store', signal,
    });
	if (!broker && epoch !== userProof.version()) throw new KCError(401, {code: 'UNAUTHENTICATED', message: '认证主体已变更。'});
    if (response.status === 204) return undefined as T;
    let value;
    try { value = await response.json(); }
    catch { throw new KCError(response.status, { code: 'TRANSPORT', message: '服务返回了无法读取的响应，请稍后重试。' }); }
    if (!response.ok) {
        const envelope = value && typeof value === 'object' && !Array.isArray(value) ? value : {};
        const fault = envelope.error && typeof envelope.error === 'object' && !Array.isArray(envelope.error) ? envelope.error : envelope;
        throw new KCError(response.status, {
            code: typeof fault.code === 'string' ? fault.code : `HTTP_${response.status}`,
            message: typeof fault.message === 'string' ? fault.message : '请求失败，请稍后重试。',
        });
    }
    return value as T;
}
