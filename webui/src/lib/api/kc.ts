// KC API adapter.
//
// Every call targets the KC Server closed typed surface only:
//   /identity/v1/whoami, /catalog/v1/..., /dataset-files/v1/..., /knowledge/v1/...
// Errors parse as KC FaultJSON ({error:{code,message,details}}); anything else
// surfaces as a transport error. No lakeFS API paths remain.
//
// Credentials come from localStorage: `kc.ui.token` (sent as Authorization
// Bearer for --auth token deployments) or `kc.ui.as` (sent as X-Kc-As for
// --auth local deployments). The KC CLI's own client.json is a different
// process store on purpose; the console keeps an isolated session.

export type Fault = {
    code: string;
    message: string;
    details?: Record<string, unknown>;
};

export class KCError extends Error {
    readonly status: number;
    readonly fault?: Fault;

    constructor(status: number, fault?: Fault, cause?: unknown) {
        super(fault?.message ?? `KC request failed with status ${status}`);
        this.name = 'KCError';
        this.status = status;
        this.fault = fault;
        this.cause = cause;
    }

    get code(): string {
        return this.fault?.code ?? 'TRANSPORT';
    }
}

const TOKEN_KEY = 'kc.ui.token';
const AS_KEY = 'kc.ui.as';

export const kcSession = {
    token(): string {
        return localStorage.getItem(TOKEN_KEY) ?? '';
    },
    as(): string {
        return localStorage.getItem(AS_KEY) ?? '';
    },
    set(token: string, as: string): void {
        if (token.trim()) localStorage.setItem(TOKEN_KEY, token.trim()); else localStorage.removeItem(TOKEN_KEY);
        if (as.trim()) localStorage.setItem(AS_KEY, as.trim()); else localStorage.removeItem(AS_KEY);
    },
    clear(): void {
        localStorage.removeItem(TOKEN_KEY);
        localStorage.removeItem(AS_KEY);
    },
};

function headers(withBody: boolean): Record<string, string> {
    const out: Record<string, string> = {};
    if (withBody) out['Content-Type'] = 'application/json';
    const token = kcSession.token();
    if (token) out['Authorization'] = `Bearer ${token}`;
    const as = kcSession.as();
    if (as && !token) out['X-Kc-As'] = as;
    return out;
}

async function parseFault(response: Response): Promise<KCError> {
    try {
        const body = await response.json();
        // KC FaultJSON carries the failure at the top level or under `error`.
        const raw = (body?.code ? body : body?.error) ?? {};
        return new KCError(response.status, {
            code: String(raw.code ?? `HTTP_${response.status}`),
            message: String(raw.message ?? response.statusText),
            details: raw.details,
        });
    } catch (err) {
        return new KCError(response.status, undefined, err);
    }
}

async function request<T>(path: string, init: RequestInit): Promise<T> {
    const response = await fetch(path, init);
    if (!response.ok) throw await parseFault(response);
    if (response.status === 204) return undefined as T;
    return (await response.json()) as T;
}

const get = <T,>(path: string): Promise<T> =>
    request<T>(path, { method: 'GET', headers: headers(false) });

const post = <T,>(path: string, body: unknown): Promise<T> =>
    request<T>(path, { method: 'POST', headers: headers(true), body: JSON.stringify(body) });

// ---- shapes (mirrors of the Go types; server stays the source of truth) ----

export type DatasetItem = {
    target: string;
    repository: string;
    commit: string;
    kind: 'prefix' | 'file';
    prefix?: string;
    file?: string;
};

export type ResolvedPin = {
    setId: string;
    revision: number;
    ref?: string;
    repositories: Record<string, string>;
    items?: DatasetItem[];
    pinId: string;
};

export type VirtualMount = {
    path: string;
    repository: string;
    selector: string;
    subPath?: string;
    commit: string;
};

export type DirectoryEntry = { name: string; kind: 'file' | 'directory' };

export type MountsResponse = { pin: ResolvedPin; mounts: VirtualMount[] };
export type DirectoryResponse = {
    pin: ResolvedPin;
    mount: VirtualMount;
    entries: DirectoryEntry[];
    continuation?: string;
    exhausted: boolean;
};

export type DatasetSummary = {
    id: string;
    revision: number;
    repositories: string[];
    itemCount: number;
    retired?: boolean;
};

export type DatasetDetail = DatasetSummary & { items: DatasetItem[] };

export type SourceInput = {
    repository: string;
    selector?: string;
    path?: string;
    subPath?: string;
    file?: string;
    target?: string;
};

// ---- endpoints ----

export const kc = {
    whoami: (): Promise<{ principal: string }> => get('/identity/v1/whoami'),

    catalogs: (): Promise<{ catalogs: { id: string }[] }> =>
        get('/catalog/v1/catalogs'),

    datasets: (catalogId: string): Promise<{ catalogId: string; datasets: DatasetSummary[] }> =>
        get(`/catalog/v1/catalogs/${encodeURIComponent(catalogId)}/datasets`),

    dataset: (catalogId: string, dataset: string): Promise<DatasetDetail> =>
        get(`/catalog/v1/catalogs/${encodeURIComponent(catalogId)}/datasets/${encodeURIComponent(dataset)}`),

    define: (catalogId: string, body: { dataset: string; revision: number; sources: SourceInput[] }): Promise<unknown> =>
        post(`/catalog/v1/catalogs/${encodeURIComponent(catalogId)}/datasets`, body),

    retire: (catalogId: string, dataset: string): Promise<unknown> =>
        post(`/catalog/v1/catalogs/${encodeURIComponent(catalogId)}/datasets/${encodeURIComponent(dataset)}/retire`, {}),

    mounts: (catalogId: string, dataset: string, pin?: ResolvedPin): Promise<MountsResponse> =>
        post('/dataset-files/v1/mounts:list', {
            catalog: catalogId,
            dataset,
            view: 'repository',
            pin,
        }),

    tree: (catalogId: string, dataset: string, pin: ResolvedPin | undefined, path: string): Promise<DirectoryResponse> =>
        post('/dataset-files/v1/tree:list', {
            catalog: catalogId,
            dataset,
            view: 'repository',
            pin,
            path,
        }),

    // file:read returns raw bytes in `content`; small text previews only.
    readFile: async (catalogId: string, dataset: string, pin: ResolvedPin | undefined, path: string): Promise<Uint8Array> => {
        const body = await post<{ content: number[] }>('/dataset-files/v1/file:read', {
            catalog: catalogId,
            dataset,
            view: 'repository',
            pin,
            path,
        });
        return Uint8Array.from(body.content ?? []);
    },
};
