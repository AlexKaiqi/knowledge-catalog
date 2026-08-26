import type { Context } from '@deepseek-ai/cordis';

// The package-root plugin is intentionally a no-op. Its presence activates the
// paired browser bundle, while host files remain owned by DSH's stock
// filesystem provider. Workspace paths arrive through Linux kcfs mounts and
// are therefore equally visible to the user, shell, IDE, and Agent.
export const name = 'dsh-loom';
export function apply(_ctx: Context): void {}
export default apply;

export { LoomVfs, LoomError } from './client.js';
export type { LoomVfsConfig, LoomFileEntry, LoomMount, LoomVfsListing, LoomFileRead, LoomWriteResult, ResolvedWorkspace } from './client.js';
export { LoomControl, apply as applyControl } from './control.js';
export type { LoomControlConfig } from './control.js';
export { LoomKnowledge, apply as applyKnowledge } from './knowledge.js';
export type { LoomKnowledgeConfig, KnowledgeFilter } from './knowledge.js';
export { LoomResourceAccess, apply as applyResource } from './resource.js';
export type { LoomResourceConfig, ResourceCall } from './resource.js';
export { apply as applySkill, integrationDevelopmentSkill, knowledgeCatalogSkill } from './skill.js';
export { LoomBrowserApi, apply as applyWeb } from './web.js';
export type { LoomBrowserConfig, LoomBrowserList, LoomBrowserRead } from './web.js';
