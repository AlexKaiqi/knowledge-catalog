import type {AdmissionResult, AllowRule} from './kc';
import {KCError} from './transport';

const strings = (value: unknown): value is string[] => Array.isArray(value) && value.every(item => typeof item === 'string');
function invalid(): never {throw new KCError(502, {code: 'TRANSPORT', message: '服务返回的授权信息不完整，请刷新或联系管理员。'});}
function record(value: unknown): Record<string, unknown> {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return invalid();
    return value as Record<string, unknown>;
}
function rule(value: unknown, principalRequired: boolean, actionsRequired: boolean): Record<string, unknown> {
    const item = record(value);
    if (Object.keys(item).some(key => key.toLowerCase() === 'catalog')) return invalid();
    if (typeof item.id !== 'string' || principalRequired && typeof item.principal !== 'string') return invalid();
    if (item.actions === undefined ? actionsRequired : !strings(item.actions)) return invalid();
    for (const key of ['repo', 'repository', 'ref', 'object', 'aspect', 'dataset', 'sharedBy', 'shareId']) {
        if (item[key] !== undefined && typeof item[key] !== 'string') return invalid();
    }
    if (item.canManage !== undefined && typeof item.canManage !== 'boolean') return invalid();
    return {...item, actions: item.actions ?? []};
}
export function grantList(value: unknown): {rules: AllowRule[]} {
    const result = record(value);
    if (!Array.isArray(result.rules)) return invalid();
    return {rules: result.rules.map(item => rule(item, true, false) as AllowRule)};
}
export function addedGrant(value: unknown): AllowRule {
    const result = rule(value, true, true);
    if (!result.id || !(result.actions as string[]).length) return invalid();
    return result as AllowRule;
}
export function admissionResult(value: unknown): AdmissionResult {
    const result = record(value), request = record(result.request);
    if (typeof result.principal !== 'string' || !Array.isArray(result.grants) || !strings(request.administrators) || request.url !== undefined && typeof request.url !== 'string') return invalid();
    return {...result, grants: result.grants.map(item => rule(item, false, true))} as AdmissionResult;
}
export function revokeResult(value: unknown, id: string): {revoked: string} {
    const result = record(value);
    if (result.revoked !== id) return invalid();
    return {revoked: id};
}
