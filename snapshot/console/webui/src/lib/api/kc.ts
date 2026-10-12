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
export type SourceInput = { baseRev?: string; repository: string; selector?: string; commit?: string; path?: string; subPath?: string; file?: string; target?: string };
export type DatasetDetail = DatasetSummary & { items: DatasetItem[]; sources: SourceInput[]; versions: number[]; servingRevision: number; continuation?: string };
type HistoryPage = {limit?: number; continuation?: string};
export type GrantScope = {repository?: string; catalog?: string; dataset?: string};
export type GrantConstraints = {ref?: string; object?: string; aspect?: string; dataset?: string};
export type GrantRequest = GrantScope & GrantConstraints & {principal: string; actions: string[]};
export type AllowRule = GrantConstraints & {id: string; principal: string; actions: string[]; repo?: string; catalog?: string; sharedBy?: string; shareId?: string};
export type AdmissionGrant = GrantConstraints & {id: string; actions: string[]; repository?: string; catalog?: string; sharedBy?: string};
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
const catalogPath = (catalog: string) => `/catalog/v1/catalogs/${encodeURIComponent(catalog)}`;
const datasetPath = (catalog: string, dataset: string) => `${catalogPath(catalog)}/datasets/${encodeURIComponent(dataset)}`;
function detailPath(catalog: string, dataset: string, revision?: number, history?: HistoryPage): string {
    const query = new URLSearchParams();
    if (revision !== undefined) query.set('revision', String(revision));
    if (history?.limit !== undefined) query.set('limit', String(history.limit));
    if (history?.continuation) query.set('continuation', history.continuation);
    return datasetPath(catalog, dataset) + (query.size ? `?${query}` : '');
}
const coordinate = (catalog: string, dataset: string, pin?: ResolvedPin) => ({ catalog, dataset, view: 'repository', pin });
export const kc = {
    admission: async (signal?: AbortSignal) => admissionResult(await request<unknown>('/identity/v1/admission', undefined, signal)),
    grants: (scope: GrantScope = {}, signal?: AbortSignal) => {
        const query = new URLSearchParams(Object.entries(scope).filter((entry): entry is [string, string] => !!entry[1]));
        return request<unknown>(`/admin/v1/grants${query.size ? `?${query}` : ''}`, undefined, signal).then(grantList);
    },
    addGrant: async (body: GrantRequest, signal?: AbortSignal) => addedGrant(await request<unknown>('/admin/v1/grants', body, signal)),
    removeGrant: async (id: string, signal?: AbortSignal) => revokeResult(await request<unknown>(`/admin/v1/grants/${encodeURIComponent(id)}`, undefined, signal, 'DELETE'), id),
    catalogs: (signal?: AbortSignal) => request<{ catalogs: { id: string }[] }>('/catalog/v1/catalogs', undefined, signal),
    repositories: (catalog: string, signal?: AbortSignal) => request<{ repositories: { id: string }[] }>(`${catalogPath(catalog)}/repositories`, undefined, signal),
    datasets: (catalog: string, signal?: AbortSignal) => request<{ datasets: DatasetSummary[] }>(`${catalogPath(catalog)}/datasets`, undefined, signal),
    dataset: (catalog: string, dataset: string, revision?: number, signal?: AbortSignal, history?: HistoryPage) => request<DatasetDetail>(detailPath(catalog, dataset, revision, history), undefined, signal),
    preview: (catalog: string, body: { dataset: string; revision: number; sources: SourceInput[] }, signal?: AbortSignal) => request<ResolvedPin>(`${catalogPath(catalog)}/datasets:resolve`, body, signal),
    define: (catalog: string, body: { dataset: string; revision: number; sources: SourceInput[] }, signal?: AbortSignal) => request<unknown>(`${catalogPath(catalog)}/datasets`, body, signal),
    retire: (catalog: string, dataset: string, signal?: AbortSignal) => request<unknown>(`${datasetPath(catalog, dataset)}/retire`, {}, signal),
    mounts: (catalog: string, dataset: string, pin?: ResolvedPin, signal?: AbortSignal) => request<MountsResponse>('/dataset-files/v1/mounts:list', coordinate(catalog, dataset, pin), signal),
    tree: (catalog: string, dataset: string, pin: ResolvedPin, path: string, continuation?: string, signal?: AbortSignal) =>
        request<DirectoryResponse>('/dataset-files/v1/tree:list', { ...coordinate(catalog, dataset, pin), path, continuation, limit: 100 }, signal),
    file: (catalog: string, dataset: string, pin: ResolvedPin, path: string, signal?: AbortSignal) =>
        request<FileResponse>('/dataset-files/v1/file:read', { ...coordinate(catalog, dataset, pin), path, offset: 0, length: 65536 }, signal),
};
export function decodeContent(content: string): Uint8Array {
    return Uint8Array.from(atob(content ?? ''), (character) => character.charCodeAt(0));
}
export const uiPath = {
    catalog: (catalog: string) => `/catalogs/${encodeURIComponent(catalog)}`,
    dataset: (catalog: string, dataset: string) => `/catalogs/${encodeURIComponent(catalog)}/datasets/${encodeURIComponent(dataset)}`,
    create: (catalog: string) => `/catalogs/${encodeURIComponent(catalog)}/create-dataset`,
};
