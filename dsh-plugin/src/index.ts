import type { Context } from '@deepseek-ai/cordis';
import { applyMountController, type MountControllerConfig } from './mount.js';

// The package root owns only the host mount lifecycle. It registers no model
// tools; Agents see mounted files through DSH's stock filesystem provider.
export const name = 'dsh-loom';
export function apply(ctx: Context, config: MountControllerConfig): void {
  applyMountController(ctx, config);
}
export default apply;

export { MountController, applyMountController } from './mount.js';
export type { MountControllerConfig } from './mount.js';
export { apply as applySkill, integrationDevelopmentSkill, knowledgeCatalogSkill } from './skill.js';
export { apply as applyWeb } from './web.js';
export type { LoomBrowserConfig, LoomBrowserList, LoomBrowserRead } from './web.js';
