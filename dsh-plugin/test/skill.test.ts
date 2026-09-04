import { describe, expect, it, vi } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { integrationDevelopmentSkill, knowledgeCatalogSkill, apply } from '../src/skill.js';

function sceneFeature(state: string, file: string): string {
  const root = fileURLToPath(new URL('../../.data/scenes/', import.meta.url));
  const matches = readdirSync(root, { recursive: true, encoding: 'utf8' })
    .filter((rel) => {
      const parts = String(rel).split('/');
      return parts.includes(state) && parts[parts.length - 1] === file;
    });
  if (matches.length !== 1) {
    throw new Error(`${state}/${file} matches=${matches.length}`);
  }
  return readFileSync(join(root, matches[0]), 'utf8');
}

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
      '`kc resource read` does not exist',
      'A bare object ID is invalid',
      "CandidateRef's repository and object",
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
      }, {
        id: 'KC-AGENT-01',
        spec: '.data/scenes',
        purpose: 'metric permission scene briefs as agent tasks; Go Then remains the protocol oracle',
      }],
    });
    expect(new Set([...scenarios.coreRoles, ...scenarios.conceptQuestions]).size).toBe(10);
    const companion = readFileSync(new URL('../../.data/data-warehouse/features/agent.feature', import.meta.url), 'utf8');
    expect(companion).toContain('@DW-AGENT-01');
    const declaredAccess = sceneFeature('knowledge-search-granted', 'probe-declared-access.feature');
    const canonicalVisible = sceneFeature('knowledge-read-granted', 'probe-canonical-visible.feature');
    const grantIsolation = sceneFeature('principals-granted', 'probe-grant-isolation.feature');
    expect(declaredAccess).toContain('@KC-AGENT-01');
    expect(declaredAccess).toContain('@P-22');
    expect(declaredAccess).toContain('Agent as bot (search-only)');
    expect(canonicalVisible).toContain('Agent as bot (search+read)');
    expect(grantIsolation).toContain('@P-23');
    expect(grantIsolation).toContain('Agent as taihu:alice (search-only)');
  });

  it('installs acceptance plugins only in run-scoped DSH homes', () => {
    const env = readFileSync(new URL('../scripts/agent-env.sh', import.meta.url), 'utf8');
    expect(env).toContain('prepare_agent_profile requires prepare_ephemeral_agent_home');
    expect(env).toContain('DSH_AGENT_EPHEMERAL_HOME');
    for (const relative of [
      '../scripts/e2e-agent-roles.sh',
      '../scripts/e2e-agent-questions.sh',
      '../scripts/e2e-agent-metric-permission.sh',
      '../../.data/data-warehouse/run-agent.sh',
    ]) {
      const runner = readFileSync(new URL(relative, import.meta.url), 'utf8');
      expect(runner.indexOf('prepare_ephemeral_agent_home')).toBeGreaterThanOrEqual(0);
      expect(runner.indexOf('prepare_ephemeral_agent_home')).toBeLessThan(runner.indexOf('prepare_agent_profile'));
    }
  });

  it('fails fast on the Agent runtime and exercises real consumer discovery', () => {
    const runner = readFileSync(new URL('../../.data/data-warehouse/run-agent.sh', import.meta.url), 'utf8');
    expect(runner.indexOf('require_agent_runtime')).toBeGreaterThanOrEqual(0);
    expect(runner.indexOf('require_agent_runtime')).toBeLessThan(runner.indexOf('run.sh'));
    expect(runner).toContain('start_acceptance_opensearch');
    expect(runner.indexOf('start_acceptance_opensearch')).toBeLessThan(runner.indexOf('configure_acceptance_opensearch'));
    expect(runner.indexOf('configure_acceptance_opensearch')).toBeLessThan(runner.indexOf('prepare_agent_profile'));

    const companion = readFileSync(new URL('../../.data/data-warehouse/features/agent.feature', import.meta.url), 'utf8');
    expect(companion).toContain('不知道任何 object ID');
    expect(companion).toContain('kc knowledge search --query');
    expect(companion).toContain('SEARCH 命中只是 CandidateRef');
    expect(companion).toContain('the Agent shell trace contains:');
    const steps = readFileSync(new URL('../../.data/data-warehouse/features/steps/agent.py', import.meta.url), 'utf8');
    expect(steps).toContain('{"skill", "bash", "shell", "todo_write"}');
  });
});
