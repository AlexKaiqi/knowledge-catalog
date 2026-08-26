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
const integrationSkillDir = fileURLToPath(new URL('../skills/integration-development/', import.meta.url));
const integrationRaw = readFileSync(new URL('../skills/integration-development/SKILL.md', import.meta.url), 'utf8');

function body(markdown: string): string {
  return markdown.replace(/^---\r?\n[\s\S]*?\r?\n---\r?\n/, '');
}

export const knowledgeCatalogSkill: SkillRegistration = Object.freeze({
  name: 'knowledge-catalog',
  description: 'Answer Knowledge Catalog usage and concept questions, then operate publishing, consumption, governance, diagnosis, and recovery from an empty workspace.',
  source: 'bundled',
  content: body(raw),
  resourceBase: { kind: 'directory' as const, path: skillDir },
  invocation: { modelInvocable: true, userInvocable: true },
});

export const integrationDevelopmentSkill: SkillRegistration = Object.freeze({
  name: 'integration-development',
  description: 'Develop and test one business integration package containing a Collector and optional live resource access implementation.',
  source: 'bundled',
  content: body(integrationRaw),
  resourceBase: { kind: 'directory' as const, path: integrationSkillDir },
  invocation: { modelInvocable: true, userInvocable: true },
});

export function apply(ctx: Context): void {
  const skills = (ctx as unknown as { skills: SkillRegistry }).skills;
  ctx.effect(() => {
    const disposeCatalog = skills.register(knowledgeCatalogSkill);
    const disposeIntegration = skills.register(integrationDevelopmentSkill);
    return () => {
      disposeIntegration();
      disposeCatalog();
    };
  }, 'dsh-loom: bundled skills');
}
