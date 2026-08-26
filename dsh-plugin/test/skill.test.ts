import { describe, expect, it, vi } from 'vitest';
import { integrationDevelopmentSkill, knowledgeCatalogSkill, apply } from '../src/skill.js';

describe('bundled Knowledge Catalog skill', () => {
  it('is self-contained and covers the six-role governed workflow', () => {
    expect(knowledgeCatalogSkill.name).toBe('knowledge-catalog');
    for (const phrase of ['Catalog Owner', 'Producer', 'Reviewer/Gatekeeper', 'Consumer', 'Auditor', 'Unauthorized Actor']) {
      expect(knowledgeCatalogSkill.content).toContain(phrase);
    }
    expect(knowledgeCatalogSkill.content).toContain('Authenticated Gitea mode');
    expect(knowledgeCatalogSkill.content).toContain('gitea:<numeric-user-id>');
    expect(knowledgeCatalogSkill.content).toContain('Never request, read, print, or place the token');
    expect(knowledgeCatalogSkill.content).toMatch(/Never retry a denial under a\s+different principal/);
    expect(knowledgeCatalogSkill.content).toContain('Do not reissue the proposal');
    expect(knowledgeCatalogSkill.content).toContain('"origin-kind":"SOURCE"');
    expect(knowledgeCatalogSkill.content).toContain('"command-id":"seed-1"');
    expect(knowledgeCatalogSkill.content).toContain('cmd:"read-workspace"');
    expect(knowledgeCatalogSkill.content).toContain('call `knowledge_search` or `knowledge_read` directly');
    expect(knowledgeCatalogSkill.content).toContain('Use `knowledge_schema`');
    expect(knowledgeCatalogSkill.content).toContain('Use bounded `knowledge_list`');
    expect(knowledgeCatalogSkill.content).toContain('pre-write coordinate');
    expect(knowledgeCatalogSkill.description).toContain('concept questions');
    for (const phrase of [
      'Catalog** registers Repositories',
      'composition/control plane, not a content store',
      'ResolvedWorkspace',
      'source-key-to-object-ID mapping',
      'not replace that mapping table',
      'stays in the provider/integration',
      'not automatically another Catalog knowledge object',
      'A provider publishes to one target Repository',
      '`schema/*` object',
      'audit` explains Catalog',
      'There is no `knowledge_audit` or `knowledge_log` tool',
      'local `index: none` profile',
      'SQLite or in-memory',
    ]) {
      expect(knowledgeCatalogSkill.content).toContain(phrase);
    }
    expect(knowledgeCatalogSkill.content).not.toContain('read-view');
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
    expect(integrationDevelopmentSkill.content).toContain('Collector command');
    expect(integrationDevelopmentSkill.content).toContain('Access command');
    expect(integrationDevelopmentSkill.content).toContain('must not invoke KC');
  });
});
