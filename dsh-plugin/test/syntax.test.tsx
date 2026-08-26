import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { SyntaxPreview, syntaxFor } from '../src/syntax.js';

describe('Catalog syntax preview', () => {
  it('recognizes KC aspect files with YAML frontmatter and a JSON body', () => {
    const content = '---\nobject_id: Dataset:orders\naspect_name: structure\n---\n{"name":"orders","enabled":true}';
    const syntax = syntaxFor('objects/Dataset:orders/structure.json', content);

    expect(syntax.label).toBe('JSON + YAML frontmatter');
    expect(syntax.segments).toHaveLength(2);
    const html = renderToStaticMarkup(<SyntaxPreview path="objects/Dataset:orders/structure.json" content={content} />);
    expect(html).toContain('token key atrule');
    expect(html).toContain('token property');
    expect(html).toContain('token boolean');
  });

  it('falls back to escaped plain text for unknown file types', () => {
    const html = renderToStaticMarkup(<SyntaxPreview path=".kc/root" content={'<script>alert("no")</script>'} />);

    expect(html).toContain('Plain text');
    expect(html).toContain('&lt;script&gt;');
    expect(html).not.toContain('<script>');
  });

  it('disables tokenization for large previews', () => {
    const syntax = syntaxFor('large.json', `{${' '.repeat(200_001)}}`);
    expect(syntax.label).toBe('JSON');
    expect(syntax.highlighted).toBe(false);
  });
});
