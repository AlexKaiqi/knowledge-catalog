export type AuthPairing = {mode: string; localAssertion: boolean; accepts: string[]; providerURL?: string; browserLogin?: {oauth2Base: string; clientId: string}};

export function acceptsUserProof(pairing: AuthPairing | null | undefined): pairing is AuthPairing {
    return !!pairing && typeof pairing.mode === 'string' && pairing.mode.trim().length > 0 && pairing.localAssertion === false &&
        Array.isArray(pairing.accepts) && pairing.accepts.every(value => typeof value === 'string') && pairing.accepts.includes('Authorization');
}

export function supportsBrowserLogin(pairing: AuthPairing | null): boolean {
    return acceptsUserProof(pairing) && !!pairing.browserLogin &&
        typeof pairing.browserLogin.oauth2Base === 'string' && pairing.browserLogin.oauth2Base.trim().length > 0 &&
        typeof pairing.browserLogin.clientId === 'string' && pairing.browserLogin.clientId.trim().length > 0;
}

// Keep a private snapshot of the current service discovery, never capabilities
// recovered from browser storage. Browser proof remains untrusted input to KC.
export function copyPairing(pairing: AuthPairing): AuthPairing {
    return {mode: pairing.mode, localAssertion: pairing.localAssertion, accepts: [...pairing.accepts].sort(),
        ...(pairing.browserLogin ? {browserLogin: {...pairing.browserLogin}} : {})};
}
