import { describe, expect, it, vi } from 'vitest';
import { knowledgeCatalogSkill, apply } from '../src/skill.js';

describe('bundled Knowledge Catalog skill', () => {
  it('is self-contained and covers the six-role governed workflow', () => {
    expect(knowledgeCatalogSkill.name).toBe('knowledge-catalog');
    for (const phrase of ['Catalog Owner', 'Producer', 'Reviewer/Gatekeeper', 'Consumer', 'Auditor', 'Unauthorized Actor']) {
      expect(knowledgeCatalogSkill.content).toContain(phrase);
    }
    expect(knowledgeCatalogSkill.content).toContain('There is no authentication yet');
    expect(knowledgeCatalogSkill.content).toMatch(/Never retry a denied command\s+without the configured principal/);
    expect(knowledgeCatalogSkill.content).toContain('Do not reissue the proposal');
    expect(knowledgeCatalogSkill.content).toContain('"origin-kind":"SOURCE"');
    expect(knowledgeCatalogSkill.content).toContain('cmd:"read-workspace"');
    expect(knowledgeCatalogSkill.content).not.toContain('read-view');
  });

  it('registers through ctx.skills so it exists before any Workspace does', () => {
    const unregister = vi.fn();
    const register = vi.fn(() => unregister);
    const effect = vi.fn((factory: () => () => void) => factory());
    apply({ skills: { register }, effect } as never);
    expect(register).toHaveBeenCalledWith(knowledgeCatalogSkill);
  });
});
