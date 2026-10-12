import { jsonRequest, KCError, RequestMethod } from './transport';
import { session } from './session';
import {addedGrant, admissionResult, grantList, revokeResult} from './kcAccess';
export { KCError } from './transport';
export type { Fault } from './transport';

export type DatasetItem = {
    target: string; repository: string; commit: string; kind: 'prefix' | 'file'; prefix?: string; file?: string;
};
export type ResolvedPin = {
    setId: string; revision: number; ref?: string; repositories: Record<string, string>; items?: DatasetItem[]; pinId?: string;
};
export type VirtualMount = { path: string; repository: string; selector: string; subPath?: string; commit: string };
export type DirectoryEntry = { name: string; kind: 'file' | 'directory' };
export type MountsResponse = { pin: ResolvedPin; mounts: VirtualMount[] };
export type DirectoryResponse = { pin: ResolvedPin; entries: DirectoryEntry[]; continuation?: string; exhausted: boolean };
export type FileResponse = { content: string; totalBytes: number; eof: boolean; offset: number };
export type DatasetSummary = { id: string; revision: number; repositories: string[]; itemCount: number; retired?: boolean };
export type RegisteredRepository = {id: string; consoleRepository?: string};
export type SourceInput = { baseRev?: string; repository: string; selector?: string; commit?: string; path?: string; subPath?: string; file?: string; target?: string };
export type DatasetDetail = DatasetSummary & { items: DatasetItem[]; sources: SourceInput[]; versions: number[]; servingRevision: number; continuation?: string };
type HistoryPage = {limit?: number; continuation?: string};
export type GrantScope = {repository?: string; dataset?: string};
export type GrantConstraints = {ref?: string; object?: string; aspect?: string; dataset?: string};
export type GrantRequest = GrantScope & GrantConstraints & {principal: string; actions: string[]};
export type AllowRule = GrantConstraints & {id: string; principal: string; actions: string[]; repo?: string; sharedBy?: string; shareId?: string; canManage?: boolean};
export type AdmissionGrant = GrantConstraints & {id: string; actions: string[]; repository?: string; sharedBy?: string; shareId?: string; canManage?: boolean};
export type AdmissionResult = {principal: string; grants: AdmissionGrant[]; request: {url?: string; administrators: string[]}};

async function request<T>(path: string, body?: unknown, signal?: AbortSignal, method?: RequestMethod): Promise<T> {
    const version = session.version();
    try {
        session.require();
        const value = await jsonRequest<T>(path, body, signal, method);
        if (version !== session.version()) throw new KCError(401, { code: 'UNAUTHENTICATED', message: '登录已变更，请重新登录。' });
        return value;
    } catch (error) {
        if (version === session.version() && error instanceof KCError && error.status === 401) session.logout();
        throw error;
    }
}
const datasetPath = (dataset: string) => `/snapshot/v1/datasets/${encodeURIComponent(dataset)}`;
function detailPath(dataset: string, revision?: number, history?: HistoryPage): string {
    const query = new URLSearchParams();
    if (revision !== undefined) query.set('revision', String(revision));
    if (history?.limit !== undefined) query.set('limit', String(history.limit));
    if (history?.continuation) query.set('continuation', history.continuation);
    return datasetPath(dataset) + (query.size ? `?${query}` : '');
}
const coordinate = (dataset: string, pin?: ResolvedPin) => ({ dataset, view: 'repository', pin });
export const kc = {
    admission: async (signal?: AbortSignal) => admissionResult(await request<unknown>('/identity/v1/admission', undefined, signal)),
    grants: (scope: GrantScope = {}, signal?: AbortSignal) => {
        const query = new URLSearchParams(Object.entries(scope).filter((entry): entry is [string, string] => !!entry[1]));
        return request<unknown>(`/admin/v1/grants${query.size ? `?${query}` : ''}`, undefined, signal).then(grantList);
    },
    addGrant: async (body: GrantRequest, signal?: AbortSignal) => addedGrant(await request<unknown>('/admin/v1/grants', body, signal)),
    updateGrant: async (rule: AllowRule, body: GrantRequest, signal?: AbortSignal) => {
        const {id, principal, actions, repo: repository, ref, object, aspect, dataset, sharedBy, shareId} = rule;
        const saved = addedGrant(await request<unknown>(`/admin/v1/grants/${encodeURIComponent(id)}`, {...body, expected: {id, principal, actions, repository, ref, object, aspect, dataset, sharedBy, shareId}}, signal, 'PUT'));
        if (saved.id !== id) throw new KCError(502, {code: 'TRANSPORT', message: '授权保存返回了不同的规则标识，请刷新核对。'});
        return saved;
    },
    removeGrant: async (id: string, signal?: AbortSignal) => revokeResult(await request<unknown>(`/admin/v1/grants/${encodeURIComponent(id)}`, undefined, signal, 'DELETE'), id),
    repositories: (signal?: AbortSignal) => request<{ repositories: RegisteredRepository[] }>('/snapshot/v1/repositories', undefined, signal),
    datasets: (signal?: AbortSignal) => request<{ datasets: DatasetSummary[] }>('/snapshot/v1/datasets', undefined, signal),
    dataset: (dataset: string, revision?: number, signal?: AbortSignal, history?: HistoryPage) => request<DatasetDetail>(detailPath(dataset, revision, history), undefined, signal),
    preview: (body: { dataset: string; revision: number; sources: SourceInput[] }, signal?: AbortSignal) => request<ResolvedPin>('/snapshot/v1/datasets:resolve', body, signal),
    define: (body: { dataset: string; revision: number; sources: SourceInput[] }, signal?: AbortSignal) => request<unknown>('/snapshot/v1/datasets', body, signal),
    retire: (dataset: string, signal?: AbortSignal) => request<unknown>(`${datasetPath(dataset)}/retire`, {}, signal),
    mounts: (dataset: string, pin?: ResolvedPin, signal?: AbortSignal) => request<MountsResponse>('/snapshot/v1/files/mounts:list', coordinate(dataset, pin), signal),
    tree: (dataset: string, pin: ResolvedPin, path: string, continuation?: string, signal?: AbortSignal) =>
        request<DirectoryResponse>('/snapshot/v1/files/tree:list', { ...coordinate(dataset, pin), path, continuation, limit: 100 }, signal),
    file: (dataset: string, pin: ResolvedPin, path: string, signal?: AbortSignal) =>
        request<FileResponse>('/snapshot/v1/files/file:read', { ...coordinate(dataset, pin), path, offset: 0, length: 65536 }, signal),
};
export function decodeContent(content: string): Uint8Array {
    return Uint8Array.from(atob(content ?? ''), (character) => character.charCodeAt(0));
}
export const uiPath = {
    dataset: (dataset: string) => `/datasets/${encodeURIComponent(dataset)}`,
    create: () => '/datasets/actions/create',
};
