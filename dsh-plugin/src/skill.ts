/** Embedded plugin Skill: available before a Catalog or Workspace exists. */

import type { Context } from '@deepseek-ai/cordis';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

export const name = 'loom-knowledge-catalog-skill';
export const inject = ['skills'];

interface SkillRegistration {
  name: string;
  description: string;
  source: string;
  content: string;
  resourceBase?: { kind: 'directory'; path: string };
  invocation?: { modelInvocable: boolean; userInvocable: boolean };
}

interface SkillRegistry {
  register(skill: SkillRegistration): () => void;
}

const skillDir = fileURLToPath(new URL('../skills/knowledge-catalog/', import.meta.url));
const raw = readFileSync(new URL('../skills/knowledge-catalog/SKILL.md', import.meta.url), 'utf8');

function body(markdown: string): string {
  return markdown.replace(/^---\r?\n[\s\S]*?\r?\n---\r?\n/, '');
}

export const knowledgeCatalogSkill: SkillRegistration = Object.freeze({
  name: 'knowledge-catalog',
  description: 'Operate a Knowledge Catalog from an empty workspace, including repositories, workspaces, governed publishing, role-scoped access, reads/search, audit, provenance, diagnosis, and recovery.',
  source: 'bundled',
  content: body(raw),
  resourceBase: { kind: 'directory' as const, path: skillDir },
  invocation: { modelInvocable: true, userInvocable: true },
});

export function apply(ctx: Context): void {
  const skills = (ctx as unknown as { skills: SkillRegistry }).skills;
  ctx.effect(() => skills.register(knowledgeCatalogSkill), 'dsh-loom: knowledge-catalog skill');
}
