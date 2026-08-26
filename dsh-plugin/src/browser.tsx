/** Browser surface for dsh-loom's read-only Workspace VFS explorer. */

import React, { useEffect, useMemo, useState, useSyncExternalStore } from 'react';
import { createPortal } from 'react-dom';
import { SyntaxPreview } from './syntax.js';

export const inject = ['slots', 'workspaces', 'sessions'];

interface LoomEntry {
  path: string;
  repository: string;
  commit: string;
}

interface LoomMount {
  path: string;
  repository: string;
  selector: string;
  subPath?: string;
  commit: string;
}

interface LoomListResponse {
  workspace: string;
  catalog?: string;
  state: 'ready' | 'uninitialized' | 'unbound' | 'unavailable';
  bindingError?: { code: string; message: string };
  available?: Array<{ catalog: string; workspace: string; revision: number }>;
  entries: LoomEntry[];
  mounts: LoomMount[];
}

interface LoomReadResponse {
  path: string;
  repository: string;
  commit: string;
  size: number;
  binary: boolean;
  truncated: boolean;
  content?: string;
}

interface TreeNode {
  name: string;
  path: string;
  type: 'file' | 'directory';
  children: TreeNode[];
  entry?: LoomEntry;
  mount?: LoomMount;
}

interface ErrorEnvelope {
  error?: { code?: string; message?: string };
}

interface SlotRegistry {
  inject(name: string, body: () => unknown): unknown;
  register(options: Record<string, unknown>, component: React.ComponentType<any>): () => void;
}

interface ClientContext {
  slots: SlotRegistry;
  workspaces: {
    create(input: { path: string }): Promise<{ workspaceId: string }>;
    connectWorkspace(id: string): Promise<string>;
    rename(id: string, title: string): Promise<unknown>;
  };
  sessions: { open(id: string): void };
}

type UseSessions = <T>(selector: (state: { current?: string; byId: Record<string, { cwd?: string }> }) => T) => T;

const API = '/api/loom/vfs';

const css = `
.loomVfsPage{--loom-syntax-comment:#008000;--loom-syntax-punctuation:#393a34;--loom-syntax-name:#0451a5;--loom-syntax-number:#098658;--loom-syntax-string:#a31515;--loom-syntax-keyword:#0000ff;--loom-syntax-function:#795e26;--loom-syntax-variable:#811f3f;box-sizing:border-box;width:100%;height:100%;min-height:0;background:var(--dsw-alias-bg-layer-1);color:var(--dsw-alias-label-primary);display:flex;flex-direction:column;overflow:hidden}
.loomVfsHeader{height:44px;flex:none;border-bottom:1px solid var(--dsw-alias-border-l2);background:var(--dsw-alias-bg-base);display:flex;align-items:center;gap:10px;padding:0 14px}
.loomVfsTitle{font-size:16px;font-weight:600}.loomVfsContext{min-width:0;color:var(--dsw-alias-label-tertiary);font-size:12px;white-space:nowrap;text-overflow:ellipsis;overflow:hidden}
.loomVfsHeaderActions{margin-left:auto;display:flex;align-items:center;gap:8px}
.loomVfsButton{height:30px;padding:0 11px;border:1px solid var(--dsw-alias-border-l2);border-radius:8px;background:transparent;color:var(--dsw-alias-label-secondary);font:inherit;cursor:pointer}.loomVfsButton:hover{background:var(--dsw-alias-interactive-bg-hover)}.loomVfsButton:disabled{opacity:.5;cursor:default}
.loomVfsSidebarNav{box-sizing:border-box;min-width:0;min-height:180px;max-height:48%;border-top:1px solid var(--dsw-alias-border-l2);padding-right:var(--dsh-sidebar-inline-padding,12px);background:var(--dsw-specific-sidebar-fill);flex:0 1 48%;display:flex;flex-direction:column}
.loomVfsSidebarHeader{height:34px;flex:none;display:flex;align-items:center;gap:7px;padding:2px 8px 0 4px;color:var(--dsw-alias-label-secondary)}
.loomVfsSidebarTitle{min-width:0;flex:1;font-size:13px;font-weight:600;white-space:nowrap;text-overflow:ellipsis;overflow:hidden}.loomVfsSidebarCount{color:var(--dsw-alias-label-caption);font-size:10px;font-weight:400}
.loomVfsSidebarAction{height:26px;border:0;border-radius:7px;background:transparent;color:var(--dsw-alias-label-tertiary);cursor:pointer;font:inherit;font-size:11px;padding:0 6px}.loomVfsSidebarAction:hover{background:var(--dsw-alias-interactive-bg-hover)}.loomVfsSidebarAction:disabled{opacity:.5}
.loomVfsSidebarRefresh{width:26px;padding:0;border-radius:50%;font-size:14px}
.loomVfsSearchWrap{padding:4px 8px 8px 4px;border-bottom:1px solid var(--dsw-alias-border-l1)}
.loomVfsSearch{box-sizing:border-box;width:100%;height:34px;border:1px solid var(--dsw-alias-border-l2);border-radius:9px;background:transparent;color:var(--dsw-alias-label-primary);font:inherit;padding:0 10px;outline:none}.loomVfsSearch:focus{border-color:var(--dsw-alias-state-business-primary)}
.loomVfsTree{flex:1;min-height:0;padding:8px;overflow:auto}.loomVfsRow{box-sizing:border-box;width:100%;height:29px;border:0;border-radius:7px;background:transparent;color:var(--dsw-alias-label-secondary);font:inherit;font-size:13px;text-align:left;display:flex;align-items:center;gap:6px;padding-right:8px;cursor:pointer}.loomVfsRow:hover{background:var(--dsw-alias-interactive-bg-hover)}.loomVfsRow[data-selected=true]{background:var(--dsw-alias-button-ghost-active-fill);color:var(--dsw-alias-state-business-primary)}
.loomVfsRootRow{cursor:default;color:var(--dsw-alias-label-primary);font-weight:500}.loomVfsRootRow:hover{background:transparent}
.loomVfsChevron{width:12px;flex:none;text-align:center;color:var(--dsw-alias-label-caption)}.loomVfsFileIcon{width:14px;flex:none;color:var(--dsw-alias-label-tertiary)}.loomVfsName{min-width:0;white-space:nowrap;text-overflow:ellipsis;overflow:hidden}
.loomVfsMountBadge{max-width:46%;margin-left:auto;border-radius:999px;background:var(--dsw-alias-button-ghost-active-fill);color:var(--dsw-alias-state-business-primary);padding:1px 6px;font-size:10px;white-space:nowrap;text-overflow:ellipsis;overflow:hidden}
.loomVfsEmpty{padding:20px;color:var(--dsw-alias-label-tertiary);font-size:13px;line-height:20px}
.loomVfsLaunch{padding:12px 8px 18px 4px;display:flex;flex-direction:column;gap:8px}.loomVfsLaunchText{color:var(--dsw-alias-label-tertiary);font-size:12px;line-height:18px}.loomVfsLaunch select,.loomVfsLaunch input{box-sizing:border-box;width:100%;height:32px;border:1px solid var(--dsw-alias-border-l2);border-radius:8px;background:var(--dsw-alias-bg-base);color:var(--dsw-alias-label-primary);padding:0 8px;font:inherit;font-size:12px}.loomVfsLaunchActions{display:flex;gap:6px}.loomVfsPrimary{flex:1;border:0!important;background:var(--dsw-alias-state-business-primary)!important;color:white!important}.loomVfsLaunchError{color:var(--dsw-alias-state-error-primary);font-size:11px;line-height:16px}
.loomVfsPreview{position:relative;min-width:0;flex:1;min-height:0;display:flex;flex-direction:column;background:var(--dsw-alias-markdown-code-block);overflow:hidden}
.loomVfsMeta{min-height:58px;flex:none;border-bottom:1px solid var(--dsw-alias-border-l2);background:var(--dsw-alias-bg-base);display:flex;flex-direction:column;justify-content:center;gap:3px;padding:8px 16px}.loomVfsPath{font-size:14px;font-weight:500;white-space:nowrap;text-overflow:ellipsis;overflow:hidden}.loomVfsCoordinates{color:var(--dsw-alias-label-tertiary);font:11px/16px var(--dsh-font-mono,monospace);white-space:nowrap;text-overflow:ellipsis;overflow:hidden}
.loomVfsLanguage{position:absolute;z-index:1;right:16px;top:74px;border:1px solid var(--dsw-alias-border-l2);border-radius:999px;background:var(--dsw-alias-bg-base);color:var(--dsw-alias-label-tertiary);font:10px/20px var(--dsh-font-mono,monospace);padding:0 8px;pointer-events:none}.loomVfsContent{flex:1;min-height:0;margin:0;padding:18px 20px 28px;overflow:auto;color:var(--dsw-alias-label-secondary);font:var(--dsw-font-markdown-code-block-small);white-space:pre;tab-size:2}.loomVfsContent code{font:inherit}.loomVfsContent .token.comment,.loomVfsContent .token.prolog,.loomVfsContent .token.doctype,.loomVfsContent .token.cdata{color:var(--loom-syntax-comment);font-style:italic}.loomVfsContent .token.punctuation{color:var(--loom-syntax-punctuation)}.loomVfsContent .token.namespace{opacity:.75}.loomVfsContent .token.property,.loomVfsContent .token.tag,.loomVfsContent .token.constant,.loomVfsContent .token.symbol,.loomVfsContent .token.deleted{color:var(--loom-syntax-name)}.loomVfsContent .token.boolean,.loomVfsContent .token.number{color:var(--loom-syntax-number)}.loomVfsContent .token.selector,.loomVfsContent .token.attr-name,.loomVfsContent .token.string,.loomVfsContent .token.char,.loomVfsContent .token.builtin,.loomVfsContent .token.inserted{color:var(--loom-syntax-string)}.loomVfsContent .token.operator,.loomVfsContent .token.entity,.loomVfsContent .token.url,.loomVfsContent .language-css .token.string,.loomVfsContent .style .token.string{color:var(--loom-syntax-punctuation)}.loomVfsContent .token.atrule,.loomVfsContent .token.attr-value,.loomVfsContent .token.keyword{color:var(--loom-syntax-keyword)}.loomVfsContent .token.function,.loomVfsContent .token.class-name{color:var(--loom-syntax-function)}.loomVfsContent .token.regex,.loomVfsContent .token.important,.loomVfsContent .token.variable{color:var(--loom-syntax-variable)}.loomVfsContent .token.important,.loomVfsContent .token.bold{font-weight:700}.loomVfsContent .token.italic{font-style:italic}.loomVfsStatus{padding:20px;color:var(--dsw-alias-label-tertiary);font-size:13px}.loomVfsError{color:var(--dsw-alias-state-error-primary)}
@media(prefers-color-scheme:dark){.loomVfsPage{--loom-syntax-comment:#6a9955;--loom-syntax-punctuation:#a8b0bd;--loom-syntax-name:#9cdcfe;--loom-syntax-number:#b5cea8;--loom-syntax-string:#ce9178;--loom-syntax-keyword:#c586c0;--loom-syntax-function:#dcdcaa;--loom-syntax-variable:#d16969}}
@media(max-width:760px){.loomVfsContext{display:none}}
`;

function installStyle(): void {
  if (document.querySelector('style[data-plugin-css="dsh-loom/vfs-browser"]')) return;
  const style = document.createElement('style');
  style.dataset.plugin = 'dsh-loom';
  style.dataset.pluginCss = 'dsh-loom/vfs-browser';
  style.textContent = css;
  document.head.append(style);
}

function IconTree(): React.ReactElement {
  return <svg aria-hidden="true" width="18" height="18" viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth="1.4"><path d="M5 3.5h8v4H5zM2.5 11h5v3.5h-5zM10.5 11h5v3.5h-5zM9 7.5v2M5 9.5h8M5 9.5V11M13 9.5V11" /></svg>;
}

function buildTree(entries: LoomEntry[], mounts: LoomMount[] = []): TreeNode[] {
  const root: TreeNode = { name: '', path: '', type: 'directory', children: [] };
  const ensure = (parts: string[], file: boolean, entry?: LoomEntry): TreeNode => {
    let parent = root;
    for (let index = 0; index < parts.length; index += 1) {
      const name = parts[index];
      const path = parts.slice(0, index + 1).join('/');
      const leafFile = file && index === parts.length - 1;
      let node = parent.children.find((child) => child.name === name);
      if (!node) {
        node = { name, path, type: leafFile ? 'file' : 'directory', children: [] };
        parent.children.push(node);
      }
      if (leafFile) node.entry = entry;
      parent = node;
    }
    return parent;
  };
  for (const mount of mounts) {
    const parts = mount.path.split('/').filter(Boolean);
    if (parts.length > 0) ensure(parts, false).mount = mount;
  }
  for (const entry of entries) {
    const parts = entry.path.split('/').filter(Boolean);
    ensure(parts, true, entry);
  }
  const sort = (nodes: TreeNode[]) => {
    nodes.sort((a, b) => a.type === b.type ? a.name.localeCompare(b.name) : a.type === 'directory' ? -1 : 1);
    nodes.forEach((node) => sort(node.children));
  };
  sort(root.children);
  return root.children;
}

async function getJson<T>(url: string): Promise<T> {
  const response = await fetch(url, { headers: { accept: 'application/json' } });
  const body = await response.json().catch(() => ({})) as T & ErrorEnvelope;
  if (!response.ok) {
    throw new Error(`${body.error?.code ?? response.status}: ${body.error?.message ?? 'request failed'}`);
  }
  return body;
}

function formatBytes(size: number): string {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

function mountTitle(mount: LoomMount): string {
  return `${mount.repository} · ${mount.selector}${mount.subPath ? ` · subPath ${mount.subPath}` : ''} · ${mount.commit}`;
}

function repoTail(repository: string): string {
  return repository.split('/').at(-1) ?? repository;
}

function workspaceChoice(catalog: string, workspace: string): string {
  return `${catalog}\0${workspace}`;
}

function TreeRows({ nodes, depth, expanded, selected, onToggle, onSelect }: {
  nodes: TreeNode[];
  depth: number;
  expanded: Set<string>;
  selected?: string;
  onToggle(path: string): void;
  onSelect(path: string): void;
}): React.ReactElement {
  return <>{nodes.map((node) => {
    const open = expanded.has(node.path);
    return <React.Fragment key={node.path}>
      <button
        type="button"
        className="loomVfsRow"
        data-selected={node.type === 'file' && selected === node.path}
        style={{ paddingLeft: 7 + depth * 15 }}
        title={node.path}
        onClick={() => node.type === 'directory' ? onToggle(node.path) : onSelect(node.path)}
      >
        <span className="loomVfsChevron">{node.type === 'directory' ? (open ? '⌄' : '›') : ''}</span>
        <span className="loomVfsFileIcon">{node.type === 'directory' ? '▱' : '·'}</span>
        <span className="loomVfsName">{node.name}</span>
        {node.mount && <span className="loomVfsMountBadge" title={mountTitle(node.mount)}>{repoTail(node.mount.repository)}</span>}
      </button>
      {node.type === 'directory' && open && <TreeRows nodes={node.children} depth={depth + 1} expanded={expanded} selected={selected} onToggle={onToggle} onSelect={onSelect} />}
    </React.Fragment>;
  })}</>;
}

interface BrowserState {
  cwd?: string;
  listing?: LoomListResponse;
  file?: LoomReadResponse;
  selected?: string;
  loading: boolean;
  reading: boolean;
  error?: string;
}

let browserState: BrowserState = { loading: false, reading: false };
let refreshGeneration = 0;
const browserListeners = new Set<() => void>();

function updateBrowserState(patch: Partial<BrowserState>): void {
  browserState = { ...browserState, ...patch };
  browserListeners.forEach((listener) => listener());
}

function subscribeBrowser(listener: () => void): () => void {
  browserListeners.add(listener);
  return () => browserListeners.delete(listener);
}

function useBrowserState(): BrowserState {
  return useSyncExternalStore(subscribeBrowser, () => browserState, () => browserState);
}

function apiUrl(cwd: string | undefined, path?: string): string {
  const query = new URLSearchParams();
  if (cwd) query.set('cwd', cwd);
  if (path !== undefined) query.set('path', path);
  return `${API}?${query.toString()}`;
}

async function postJson<T>(body: Record<string, unknown>): Promise<T> {
  const response = await fetch(API, { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify(body) });
  const value = await response.json().catch(() => ({})) as T & ErrorEnvelope;
  if (!response.ok) throw new Error(`${value.error?.code ?? response.status}: ${value.error?.message ?? 'request failed'}`);
  return value;
}

async function refreshBrowser(cwd?: string): Promise<void> {
  const generation = ++refreshGeneration;
  if (browserState.cwd !== cwd) updateBrowserState({ cwd, listing: undefined, selected: undefined, file: undefined });
  updateBrowserState({ loading: true, error: undefined, cwd });
  try {
    const listing = await getJson<LoomListResponse>(apiUrl(cwd));
    if (generation !== refreshGeneration) return;
    const selected = browserState.selected;
    updateBrowserState({ listing });
    if (selected && listing.entries.some((entry) => entry.path === selected)) {
      const file = await getJson<LoomReadResponse>(apiUrl(cwd, selected));
      if (browserState.selected === selected) updateBrowserState({ file });
    } else if (selected) {
      updateBrowserState({ selected: undefined, file: undefined });
    }
  } catch (cause) {
    if (generation !== refreshGeneration) return;
    updateBrowserState({ error: cause instanceof Error ? cause.message : String(cause) });
  } finally {
    if (generation === refreshGeneration) updateBrowserState({ loading: false });
  }
}

function activateCatalogView(): void {
  const tab = Array.from(document.querySelectorAll<HTMLButtonElement>('[role="tab"]'))
    .find((button) => button.textContent?.trim() === 'Catalog');
  if (tab && tab.getAttribute('aria-selected') !== 'true') tab.click();
}

async function selectBrowserPath(path: string): Promise<void> {
  activateCatalogView();
  updateBrowserState({ selected: path, file: undefined, reading: true, error: undefined });
  try {
    const file = await getJson<LoomReadResponse>(apiUrl(browserState.cwd, path));
    if (browserState.selected === path) updateBrowserState({ file });
  } catch (cause) {
    if (browserState.selected === path) {
      updateBrowserState({ error: cause instanceof Error ? cause.message : String(cause) });
    }
  } finally {
    if (browserState.selected === path) updateBrowserState({ reading: false });
  }
}

function CatalogNavigation({ useSessions, launch }: { useSessions: UseSessions; launch(anchor: string, title: string): Promise<void> }): React.ReactElement {
  const { listing, selected, loading, error } = useBrowserState();
  const cwd = useSessions((state) => state.current ? state.byId[state.current]?.cwd : undefined);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [didSeedExpanded, setDidSeedExpanded] = useState(false);
  const [query, setQuery] = useState('');
  const [creating, setCreating] = useState(false);
  const [switching, setSwitching] = useState(false);
  const [busy, setBusy] = useState(false);
  const [launchError, setLaunchError] = useState<string>();
  const [choice, setChoice] = useState('');
  const [catalog, setCatalog] = useState('kr://acme/catalog');
  const [workspace, setWorkspace] = useState('warehouse');
  const [repository, setRepository] = useState('kr://acme/warehouse/public');

  useEffect(() => { void refreshBrowser(cwd); }, [cwd]);
  const choices = useMemo(() => {
    const available = listing?.available ?? [];
    if (listing?.state !== 'ready' && listing?.state !== 'unavailable') return available;
    const current = workspaceChoice(listing.catalog ?? '', listing.workspace);
    return available.filter((item) => workspaceChoice(item.catalog, item.workspace) !== current);
  }, [listing]);
  useEffect(() => {
    if (!choices.some((item) => workspaceChoice(item.catalog, item.workspace) === choice)) {
      const first = choices[0];
      setChoice(first ? workspaceChoice(first.catalog, first.workspace) : '');
    }
  }, [choices, choice]);

  const tree = useMemo(() => buildTree(listing?.entries ?? [], listing?.mounts ?? []), [listing]);
  useEffect(() => {
    if (didSeedExpanded) return;
    const topDirectories = tree.filter((node) => node.type === 'directory').map((node) => node.path);
    if (topDirectories.length > 0) {
      setExpanded(new Set(topDirectories));
      setDidSeedExpanded(true);
    }
  }, [tree, didSeedExpanded]);
  const matches = useMemo(() => {
    const value = query.trim().toLocaleLowerCase();
    return value ? (listing?.entries ?? []).filter((entry) => entry.path.toLocaleLowerCase().includes(value)) : [];
  }, [listing, query]);
  const toggle = (path: string) => setExpanded((current) => {
    const next = new Set(current);
    if (next.has(path)) next.delete(path); else next.add(path);
    return next;
  });
  const rootMount = listing?.mounts.find((mount) => mount.path === '');
  const uninitialized = listing?.state === 'uninitialized';
  const unbound = listing?.state === 'unbound';
  const unavailable = listing?.state === 'unavailable';
  const launchVisible = uninitialized || unbound || unavailable || switching;
  const enter = async (action: 'activate' | 'create') => {
    setBusy(true); setLaunchError(undefined);
    try {
      let selectedCatalog = catalog;
      let selectedWorkspace = workspace;
      if (action === 'activate') [selectedCatalog, selectedWorkspace] = choice.split('\0');
      const result = await postJson<{ anchor: string }>({
        action, catalog: selectedCatalog, workspace: selectedWorkspace,
        ...(action === 'create' ? { repository } : {}),
      });
      await launch(result.anchor, selectedWorkspace);
    } catch (cause) {
      setLaunchError(cause instanceof Error ? cause.message : String(cause));
    } finally { setBusy(false); }
  };

  return <section className="loomVfsSidebarNav" aria-label="Catalog Workspace files">
    <header className="loomVfsSidebarHeader">
      <IconTree />
      <span className="loomVfsSidebarTitle">Catalog · {listing?.workspace ?? '…'} <span className="loomVfsSidebarCount">{listing?.entries.length ?? 0}</span></span>
      {listing?.state === 'ready' && <button type="button" className="loomVfsSidebarAction" title="切换 Catalog Workspace" aria-label="切换 Catalog Workspace" onClick={() => { setCreating(false); setSwitching((value) => !value); }}>{switching ? '返回' : '切换'}</button>}
      <button type="button" className="loomVfsSidebarAction loomVfsSidebarRefresh" title="刷新 Catalog" aria-label="刷新 Catalog" disabled={loading} onClick={() => void refreshBrowser(cwd)}>↻</button>
    </header>
    {!uninitialized && !launchVisible && <div className="loomVfsSearchWrap"><input className="loomVfsSearch" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="过滤 Catalog 路径…" aria-label="过滤 Catalog 路径" /></div>}
    <div className="loomVfsTree">
      {launchVisible ? <div className="loomVfsLaunch">
        <div className="loomVfsLaunchText">{uninitialized ? '新建第一个知识工作区后，Agent 才会启动在对应的目录和权限上下文中。' : unavailable ? `当前绑定不可用（${listing?.bindingError?.message ?? '未知错误'}）。请选择另一个知识工作区恢复。` : switching ? '选择另一个知识工作区；它会在独立的 Agent 会话中打开，当前会话保持不变。' : '选择一个知识工作区进入；每个工作区会启动独立的 Agent 会话。'}</div>
        {!creating && (unbound || unavailable || switching) && choices.length > 0 && <>
          <select value={choice} onChange={(event) => setChoice(event.target.value)} aria-label="选择 Catalog Workspace">
            {choices.map((item) => <option key={workspaceChoice(item.catalog, item.workspace)} value={workspaceChoice(item.catalog, item.workspace)}>{item.workspace} · {item.catalog}</option>)}
          </select>
          <div className="loomVfsLaunchActions"><button className="loomVfsButton loomVfsPrimary" disabled={busy || !choice} onClick={() => void enter('activate')}>{switching || unavailable ? '切换并打开' : '进入工作区'}</button><button className="loomVfsButton" onClick={() => setCreating(true)}>新建</button>{switching && <button className="loomVfsButton" onClick={() => setSwitching(false)}>取消</button>}</div>
        </>}
        {!creating && switching && choices.length === 0 && <div className="loomVfsLaunchText">没有其他可用的 Workspace。你可以新建一个，或返回当前 Workspace。</div>}
        {(creating || uninitialized || (!switching && !choices.length)) && <>
          <input value={catalog} onChange={(event) => setCatalog(event.target.value)} placeholder="Catalog，例如 kr://acme/catalog" />
          <input value={workspace} onChange={(event) => setWorkspace(event.target.value)} placeholder="Workspace，例如 warehouse" />
          <input value={repository} onChange={(event) => setRepository(event.target.value)} placeholder="根知识仓，例如 kr://acme/warehouse/public" />
          <div className="loomVfsLaunchActions"><button className="loomVfsButton loomVfsPrimary" disabled={busy} onClick={() => void enter('create')}>{busy ? '创建中…' : '新建并进入'}</button>{(unbound || unavailable || switching) && <button className="loomVfsButton" onClick={() => setCreating(false)}>取消</button>}</div>
        </>}
        {!creating && switching && choices.length === 0 && <div className="loomVfsLaunchActions"><button className="loomVfsButton loomVfsPrimary" onClick={() => setCreating(true)}>新建 Workspace</button><button className="loomVfsButton" onClick={() => setSwitching(false)}>返回</button></div>}
        {launchError && <div className="loomVfsLaunchError">{launchError}</div>}
      </div> : <>
        {rootMount && <div className="loomVfsRow loomVfsRootRow" title={mountTitle(rootMount)}><span className="loomVfsChevron" /><span className="loomVfsFileIcon">▱</span><span className="loomVfsName">/</span><span className="loomVfsMountBadge">{repoTail(rootMount.repository)}</span></div>}
        {query.trim() ? matches.map((entry) => <button key={entry.path} type="button" className="loomVfsRow" data-selected={selected === entry.path} title={entry.path} onClick={() => void selectBrowserPath(entry.path)}><span className="loomVfsChevron" /><span className="loomVfsFileIcon">·</span><span className="loomVfsName">{entry.path}</span></button>)
          : tree.length > 0 ? <TreeRows nodes={tree} depth={0} expanded={expanded} selected={selected} onToggle={toggle} onSelect={(path) => void selectBrowserPath(path)} />
            : <div className={`loomVfsEmpty${error ? ' loomVfsError' : ''}`}>{error ?? (loading ? '正在读取 Workspace…' : 'Workspace 中没有可浏览的文件。')}</div>}
      </>}
    </div>
  </section>;
}

function CatalogView({ useSessions, sessionId }: { useSessions: UseSessions; sessionId: string }): React.ReactElement {
  const { listing, file, loading, reading, error } = useBrowserState();
  const cwd = useSessions((state) => state.byId[sessionId]?.cwd);
  useEffect(() => { void refreshBrowser(cwd); }, [cwd]);

  return <section className="loomVfsPage" aria-label="Catalog VFS">
      <header className="loomVfsHeader">
        <IconTree />
        <span className="loomVfsTitle">Catalog VFS</span>
        <span className="loomVfsContext">Workspace: {listing?.workspace ?? '…'}{listing?.catalog ? ` · ${listing.catalog}` : ''} · {listing?.entries.length ?? 0} files · {listing?.mounts.length ?? 0} mounts</span>
        <div className="loomVfsHeaderActions">
          <button type="button" className="loomVfsButton" disabled={loading} onClick={() => void refreshBrowser(cwd)}>{loading ? '刷新中…' : '刷新'}</button>
        </div>
      </header>
        <main className="loomVfsPreview">
          {file ? <>
            <div className="loomVfsMeta"><div className="loomVfsPath">{file.path} · {formatBytes(file.size)}{file.truncated ? ' · 预览已截断' : ''}</div><div className="loomVfsCoordinates">{file.repository} @ {file.commit}</div></div>
            {file.binary ? <div className="loomVfsStatus">二进制文件，不提供文本预览。</div> : <SyntaxPreview path={file.path} content={file.content ?? ''} />}
          </> : <div className={`loomVfsStatus${error || listing?.bindingError ? ' loomVfsError' : ''}`}>{error ?? (listing?.state === 'uninitialized' ? '请先从左侧新建第一个知识工作区。' : listing?.state === 'unbound' ? '请先从左侧选择一个知识工作区，Agent 会在新会话中绑定它。' : listing?.state === 'unavailable' ? `当前绑定不可用：${listing.bindingError?.message ?? '未知错误'}。请从左侧切换 Workspace。` : reading ? '正在读取文件…' : '从左侧 Workspace 导航选择一个 Catalog 文件。')}</div>}
        </main>
    </section>;
}

function SidebarCatalogPortal({ useSessions, launch }: { useSessions: UseSessions; launch(anchor: string, title: string): Promise<void> }): React.ReactElement | null {
  const [target, setTarget] = useState<Element | null>(null);
  const [wide, setWide] = useState(false);

  useEffect(() => {
    let watched: Node = document.body;
    const locate = () => {
      const next = document.querySelector('[data-slot="sidebar.workspaces"]');
      setTarget((current) => current === next ? current : next);
      if (next?.parentElement && watched !== next.parentElement) {
        observer.disconnect();
        watched = next.parentElement;
        observer.observe(watched, { childList: true, subtree: true });
      }
    };
    const observer = new MutationObserver(locate);
    observer.observe(watched, { childList: true, subtree: true });
    locate();
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    if (!target?.parentElement) return;
    const parent = target.parentElement;
    const measure = () => setWide(parent.getBoundingClientRect().width > 100);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(parent);
    return () => observer.disconnect();
  }, [target]);

  return target && wide ? createPortal(<CatalogNavigation useSessions={useSessions} launch={launch} />, target) : null;
}

export function apply(ctx: ClientContext): void {
  installStyle();
  const launch = async (anchor: string, title: string) => {
    const workspace = await ctx.workspaces.create({ path: anchor });
    await ctx.workspaces.rename(workspace.workspaceId, title);
    const session = await ctx.workspaces.connectWorkspace(workspace.workspaceId);
    ctx.sessions.open(session);
  };
  ctx.slots.inject('shell.overlay', () => ctx.slots.register({
    name: 'shell.overlay',
    id: 'loom-catalog-sidebar',
    order: 20,
  }, (props: { useSessions: UseSessions }) => <SidebarCatalogPortal {...props} launch={launch} />));
  ctx.slots.inject('conversation.view', () => ctx.slots.register({
    name: 'conversation.view',
    id: 'catalog',
    order: 20,
    label: 'Catalog',
  }, CatalogView));
}
