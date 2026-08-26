import React, { useMemo } from 'react';
import Prism from 'prismjs';
import 'prismjs/components/prism-bash.js';
import 'prismjs/components/prism-go.js';
import 'prismjs/components/prism-json.js';
import 'prismjs/components/prism-markdown.js';
import 'prismjs/components/prism-python.js';
import 'prismjs/components/prism-sql.js';
import 'prismjs/components/prism-toml.js';
import 'prismjs/components/prism-typescript.js';
import 'prismjs/components/prism-yaml.js';

const MAX_HIGHLIGHT_CHARACTERS = 200_000;

interface LanguageDefinition {
  id: string;
  label: string;
}

interface SyntaxSegment {
  content: string;
  language?: LanguageDefinition;
}

export interface SyntaxDefinition {
  label: string;
  highlighted: boolean;
  segments: SyntaxSegment[];
}

const extensions: Record<string, LanguageDefinition> = {
  bash: { id: 'bash', label: 'Shell' },
  css: { id: 'css', label: 'CSS' },
  go: { id: 'go', label: 'Go' },
  htm: { id: 'markup', label: 'HTML' },
  html: { id: 'markup', label: 'HTML' },
  js: { id: 'javascript', label: 'JavaScript' },
  json: { id: 'json', label: 'JSON' },
  jsonc: { id: 'json', label: 'JSON' },
  jsx: { id: 'javascript', label: 'JSX' },
  md: { id: 'markdown', label: 'Markdown' },
  mdx: { id: 'markdown', label: 'MDX' },
  py: { id: 'python', label: 'Python' },
  sh: { id: 'bash', label: 'Shell' },
  sql: { id: 'sql', label: 'SQL' },
  svg: { id: 'markup', label: 'SVG' },
  toml: { id: 'toml', label: 'TOML' },
  ts: { id: 'typescript', label: 'TypeScript' },
  tsx: { id: 'typescript', label: 'TSX' },
  xml: { id: 'markup', label: 'XML' },
  yaml: { id: 'yaml', label: 'YAML' },
  yml: { id: 'yaml', label: 'YAML' },
  zsh: { id: 'bash', label: 'Shell' },
};

function languageFor(path: string, content: string): LanguageDefinition | undefined {
  const name = path.split('/').at(-1)?.toLocaleLowerCase() ?? '';
  const extension = name.includes('.') ? name.split('.').at(-1) ?? '' : '';
  const known = extensions[extension];
  if (known) return known;
  const trimmed = content.trimStart();
  if (trimmed.startsWith('{') || trimmed.startsWith('[')) return extensions.json;
  if (trimmed.startsWith('#!') && /\b(?:ba|z)?sh\b/.test(trimmed.slice(0, 80))) return extensions.sh;
  return undefined;
}

function frontmatterEnd(content: string): number {
  if (!content.startsWith('---\n') && !content.startsWith('---\r\n')) return -1;
  const match = /\r?\n---\r?\n/.exec(content.slice(3));
  return match ? 3 + match.index + match[0].length : -1;
}

export function syntaxFor(path: string, content: string): SyntaxDefinition {
  const language = languageFor(path, content);
  const end = frontmatterEnd(content);
  const highlighted = content.length <= MAX_HIGHLIGHT_CHARACTERS;
  if (end >= 0) {
    return {
      label: language ? `${language.label} + YAML frontmatter` : 'YAML frontmatter',
      highlighted,
      segments: [
        { content: content.slice(0, end), language: extensions.yaml },
        { content: content.slice(end), language },
      ],
    };
  }
  return {
    label: language?.label ?? 'Plain text',
    highlighted,
    segments: [{ content, language }],
  };
}

function tokenClass(token: Prism.Token): string {
  const aliases = Array.isArray(token.alias) ? token.alias : token.alias ? [token.alias] : [];
  return ['token', token.type, ...aliases].join(' ');
}

function renderToken(stream: Prism.TokenStream, key: string): React.ReactNode {
  if (typeof stream === 'string') return stream;
  if (Array.isArray(stream)) {
    return stream.map((token, index) => <React.Fragment key={`${key}-${index}`}>{renderToken(token, `${key}-${index}`)}</React.Fragment>);
  }
  return <span className={tokenClass(stream)}>{renderToken(stream.content, `${key}-content`)}</span>;
}

export function SyntaxPreview({ path, content }: { path: string; content: string }): React.ReactElement {
  const syntax = useMemo(() => syntaxFor(path, content), [path, content]);
  const rendered = useMemo(() => syntax.segments.map((segment, index) => {
    const grammar = segment.language ? Prism.languages[segment.language.id] : undefined;
    const stream = syntax.highlighted && grammar ? Prism.tokenize(segment.content, grammar) : segment.content;
    return <React.Fragment key={index}>{renderToken(stream, `segment-${index}`)}</React.Fragment>;
  }), [syntax]);

  return <>
    <span className="loomVfsLanguage">{syntax.label}{syntax.highlighted ? '' : ' · 大文件纯文本'}</span>
    <pre className="loomVfsContent"><code>{rendered}</code></pre>
  </>;
}
