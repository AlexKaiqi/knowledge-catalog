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
      'ResolvedKnowledgeSet',
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
      '`kc attach`',
      'does not create or modify its source',
      '`kc deployment init --config`',
      'Never write Repository files',
      'retry `FORBIDDEN`',
      'create proposal → create/validate Preview',
    ]) {
      expect(knowledgeCatalogSkill.content).toContain(phrase);
    }
    expect(Buffer.byteLength(knowledgeCatalogSkill.content)).toBeLessThan(5_000);
    expect(knowledgeCatalogSkill.content).not.toContain("kc local ");
    expect(knowledgeCatalogSkill.content).not.toContain("catalog repo register");
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
    expect(integrationDevelopmentSkill.content).toContain('Resource Access:');
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
        id: 'KC-AGENT-01',
        spec: '.data/scenes',
        purpose: 'metric permission scene briefs as agent tasks; Go Then remains the protocol oracle',
      }],
    });
    expect(new Set([...scenarios.coreRoles, ...scenarios.conceptQuestions]).size).toBe(10);
    const declaredAccess = sceneFeature('knowledge-search-granted', 'schema-search-enforces-declared-access.feature');
    const canonicalVisible = sceneFeature('knowledge-search-granted', 'read-grant-returns-canonical.feature');
    const grantIsolation = sceneFeature('dataset-query-principals-granted', 'dataset-grants-do-not-transfer-principals.feature');
    expect(declaredAccess).toContain('@KC-AGENT-01');
    expect(declaredAccess).toContain('@P-22');
    expect(declaredAccess).toContain('Agent as searcher (search-only)');
    expect(canonicalVisible).toContain('Agent as searcher (search+read)');
    expect(grantIsolation).toContain('@P-23');
    expect(grantIsolation).toContain('taihu:alice');
  });

  it('installs acceptance plugins only in run-scoped DSH homes', () => {
    const env = readFileSync(new URL('../scripts/agent-env.sh', import.meta.url), 'utf8');
    expect(env).toContain('prepare_agent_profile requires prepare_ephemeral_agent_home');
    expect(env).toContain('DSH_AGENT_EPHEMERAL_HOME');
    for (const relative of [
      '../scripts/e2e-agent-roles.sh',
      '../scripts/e2e-agent-questions.sh',
      '../scripts/e2e-agent-metric-permission.sh',
    ]) {
      const runner = readFileSync(new URL(relative, import.meta.url), 'utf8');
      expect(runner.indexOf('prepare_ephemeral_agent_home')).toBeGreaterThanOrEqual(0);
      expect(runner.indexOf('prepare_ephemeral_agent_home')).toBeLessThan(runner.indexOf('prepare_agent_profile'));
    }
  });

  it('fails fast on the Agent runtime and exercises real consumer discovery', () => {
    const runner = readFileSync(new URL('../scripts/e2e-agent-metric-permission.sh', import.meta.url), 'utf8');
    expect(runner.indexOf('dsh executable not found')).toBeGreaterThanOrEqual(0);
    expect(runner.indexOf('KC_TEST_OPENSEARCH_URL')).toBeGreaterThanOrEqual(0);
    expect(runner.indexOf('KC_TEST_OPENSEARCH_URL')).toBeLessThan(runner.indexOf('serve --config'));

    const companion = sceneFeature('knowledge-search-granted', 'schema-search-enforces-declared-access.feature');
    expect(companion).toContain('Agent as searcher (search-only)');
    expect(companion).toContain('kc search --as searcher --repo kr://scene/knowledge --query');
    expect(companion).toContain('@KC-AGENT-01');
    const guard = readFileSync(new URL('../scripts/e2e_agent_metric_permission.py', import.meta.url), 'utf8');
    expect(guard).toContain('.data/data-warehouse');
    expect(guard).toContain('warehouse-agent');
  });
});
