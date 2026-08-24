import { describe, expect, it, vi } from 'vitest';
import { knowledgeCatalogSkill, apply } from '../src/skill.js';

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
    expect(knowledgeCatalogSkill.content).toContain('cmd:"read-workspace"');
    expect(knowledgeCatalogSkill.content).toContain('pre-write coordinate');
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
