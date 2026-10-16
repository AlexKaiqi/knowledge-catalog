import {acceptsUserProof, AuthPairing, copyPairing, supportsBrowserLogin} from './sessionPairing';

type Proof = {mode: string; accessToken: string; principal?: string; refreshToken?: string; expiresAt?: number};
const key = () => `kc.ui.proof.v2:${window.location.origin}`;
let epoch = 0;
let renewal: {version: number; task: Promise<void>} | null = null;
let pairing: AuthPairing | null = null;
function read(): Proof | null {
    try {
        const value = JSON.parse(window.sessionStorage.getItem(key()) || 'null');
        return value && acceptsUserProof(pairing) && value.mode === pairing.mode && typeof value.accessToken === 'string' && value.accessToken && !/\s/.test(value.accessToken) ? value : null;
    } catch { return null; }
}
function requireRenewalPairing(proof: Proof, version: number): void {
    if (version !== epoch || !supportsBrowserLogin(pairing) || proof.mode !== pairing?.mode) throw new Error('认证主体或服务配对已变更，请重新认证。');
}
export const userProof = {
    version: () => epoch,
    get: read,
    bind(discovery: AuthPairing): void {
        const previous = pairing;
        const next = acceptsUserProof(discovery) ? copyPairing(discovery) : null;
        const proof = read();
        if (JSON.stringify(previous) !== JSON.stringify(next)) epoch++;
        pairing = next;
        if (proof && (!next || proof.mode !== next.mode)) window.sessionStorage.removeItem(key());
    },
    set(proof: Proof): void {
        if (!acceptsUserProof(pairing) || proof.mode !== pairing.mode || !proof.accessToken || /\s/.test(proof.accessToken)) throw new Error('请输入认证系统签发的访问令牌。');
        epoch++;
        window.sessionStorage.setItem(key(), JSON.stringify(proof));
    },
    clear(): void { epoch++; pairing = null; window.sessionStorage.removeItem(key()); },
};

async function renew(proof: Proof, version: number): Promise<void> {
    if (!proof.refreshToken || !proof.principal) throw new Error('认证已过期，请重新认证。');
    requireRenewalPairing(proof, version);
    const response = await fetch('/identity/v1/token', {method: 'POST', credentials: 'same-origin', redirect: 'error', cache: 'no-store',
        headers: {'Content-Type': 'application/json', 'X-Kc-Console': '1'}, body: JSON.stringify({grantType: 'refresh_token', refreshToken: proof.refreshToken})});
    const token = await response.json();
    if (!response.ok || typeof token.accessToken !== 'string' || !token.accessToken || /\s/.test(token.accessToken) || !(token.expiresIn > 0)) throw new Error('认证续期失败，请重新认证。');
    requireRenewalPairing(proof, version);
    const whoResponse = await fetch('/identity/v1/whoami', {credentials: 'same-origin', redirect: 'error', cache: 'no-store', headers: {Authorization: `Bearer ${token.accessToken}`, 'X-Kc-Console': '1'}});
    const who = await whoResponse.json();
    requireRenewalPairing(proof, version);
    if (!whoResponse.ok || who.principal !== proof.principal) throw new Error('认证主体已变更，请重新认证。');
    window.sessionStorage.setItem(key(), JSON.stringify({...proof, accessToken: token.accessToken, refreshToken: token.refreshToken || proof.refreshToken, expiresAt: Date.now() + token.expiresIn * 1000}));
}

export async function userHeaders(): Promise<Record<string, string>> {
    let proof = read();
    if (proof?.expiresAt && proof.expiresAt < Date.now() + 10_000) {
        const version = epoch;
        if (!renewal || renewal.version !== version) {
            const active = {version, task: renew(proof, version)};
            active.task = active.task.finally(() => {if (renewal === active) renewal = null;});
            renewal = active;
        }
        try { await renewal.task; }
        catch {
            if (version === epoch) userProof.clear();
            throw Object.assign(new Error('认证已失效，请重新认证。'), {status: 401, code: 'UNAUTHENTICATED'});
        }
        proof = read();
    }
    return proof ? {Authorization: `Bearer ${proof.accessToken}`} : {};
}
