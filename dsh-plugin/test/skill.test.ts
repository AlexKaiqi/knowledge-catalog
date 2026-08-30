import { describe, expect, it, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { integrationDevelopmentSkill, knowledgeCatalogSkill, apply } from '../src/skill.js';

describe('bundled Knowledge Catalog skill', () => {
  it('keeps only the operational model and hard boundaries', () => {
    expect(knowledgeCatalogSkill.name).toBe('knowledge-catalog');
    for (const phrase of [
      'The user does not need to know KC commands',
      'read-only, fixed-version knowledge',
      'the host supplies identity, Catalog',
      'the user supplies only a topic/object',
      'Derive a focused query',
      'do not present\n   a search hit as Canonical content',
      'sidebar “知识”',
      'Repository: versioned knowledge authority',
      'Catalog: registers Repositories',
      'ResolvedWorkspace',
      '`schema/*` knowledge object',
      'Source keys and the mapping from source-system identity',
      'provider/integration side',
      'does not require a Workspace first',
      'There is no public Knowledge LIST',
      '`resource read` does\n  not exist',
      '`index:none`/no provider, not no match',
      'Configure OpenSearch; never invent SQLite/memory',
      'ordinary `ls`, `find`, `rg`, and `cat`',
      '`kc local repository attach`',
      'Never write Repository files',
      'retry `FORBIDDEN`',
      'create proposal → create/validate Preview',
    ]) {
      expect(knowledgeCatalogSkill.content).toContain(phrase);
    }
    expect(Buffer.byteLength(knowledgeCatalogSkill.content)).toBeLessThan(5_000);
  });

  it('registers through ctx.skills so it exists before any Workspace does', () => {
    const unregister = vi.fn();
    const register = vi.fn(() => unregister);
    const effect = vi.fn((factory: () => () => void) => factory());
    apply({ skills: { register }, effect } as never);
    expect(register).toHaveBeenCalledWith(knowledgeCatalogSkill);
    expect(register).toHaveBeenCalledWith(integrationDevelopmentSkill);
  });

  it('keeps Collector and live access development in one integration package', () => {
    expect(integrationDevelopmentSkill.name).toBe('integration-development');
    expect(integrationDevelopmentSkill.content).toContain('Collector:');
    expect(integrationDevelopmentSkill.content).toContain('Access command');
    expect(integrationDevelopmentSkill.content).toMatch(/must not\s+invoke KC/);
    expect(integrationDevelopmentSkill.description).toContain('not for operating');
    expect(integrationDevelopmentSkill.content.length).toBeLessThan(2_000);
  });

  it('keeps a unique, explicit denominator for paid Agent acceptance', () => {
    const scenarios = JSON.parse(readFileSync(new URL('../scripts/agent-scenarios.json', import.meta.url), 'utf8'));
    expect(scenarios).toEqual({
      version: 1,
      coreRoles: ['provider', 'governor', 'consumer', 'auditor', 'recovery', 'unauthorized'],
      conceptQuestions: ['first-use-model', 'consumer-model', 'provider-model', 'troubleshooting-model'],
      extendedCompanions: [{
        id: 'DW-AGENT-01',
        spec: '.data/data-warehouse/features/agent.feature',
        purpose: 'data-warehouse provider and consumer companion',
      }],
    });
    expect(new Set([...scenarios.coreRoles, ...scenarios.conceptQuestions]).size).toBe(10);
    const companion = readFileSync(new URL('../../.data/data-warehouse/features/agent.feature', import.meta.url), 'utf8');
    expect(companion).toContain('@DW-AGENT-01');
  });

  it('installs acceptance plugins only in run-scoped DSH homes', () => {
    const env = readFileSync(new URL('../scripts/agent-env.sh', import.meta.url), 'utf8');
    expect(env).toContain('prepare_agent_profile requires prepare_ephemeral_agent_home');
    expect(env).toContain('DSH_AGENT_EPHEMERAL_HOME');
    for (const relative of [
      '../scripts/e2e-agent-roles.sh',
      '../scripts/e2e-agent-questions.sh',
      '../../.data/data-warehouse/run-agent.sh',
    ]) {
      const runner = readFileSync(new URL(relative, import.meta.url), 'utf8');
      expect(runner.indexOf('prepare_ephemeral_agent_home')).toBeGreaterThanOrEqual(0);
      expect(runner.indexOf('prepare_ephemeral_agent_home')).toBeLessThan(runner.indexOf('prepare_agent_profile'));
    }
  });
});
