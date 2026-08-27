import { describe, expect, it, vi } from 'vitest';
import { integrationDevelopmentSkill, knowledgeCatalogSkill, apply } from '../src/skill.js';

describe('bundled Knowledge Catalog skill', () => {
  it('keeps only the operational model and hard boundaries', () => {
    expect(knowledgeCatalogSkill.name).toBe('knowledge-catalog');
    for (const phrase of [
      'Repository: versioned knowledge authority',
      'Catalog: registers Repositories',
      'ResolvedWorkspace',
      '`schema/*` knowledge object',
      'not a scalable search',
      'Never scan every page',
      'load `integration-development`',
      'both `repo` and',
      'Never write Repository files',
      'retry `FORBIDDEN`',
      '`propose -> preview -> validate/evidence -> merge`',
    ]) {
      expect(knowledgeCatalogSkill.content).toContain(phrase);
    }
    expect(knowledgeCatalogSkill.content.length).toBeLessThan(5_000);
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
});
