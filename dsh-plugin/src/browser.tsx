/** Optional VFS FolderTree mounted below the host Workspace navigation. */

import React, { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import RcTreePackage, { type BasicDataNode, type TreeNodeProps, type TreeProps } from 'rc-tree';
import { SyntaxPreview } from './syntax.js';

export const inject = ['slots'];

interface LoomEntry {
  path: string;
  repository: string;
  commit: string;
  kind: 'file' | 'directory';
}

interface LoomMount {
  path: string;
  repository: string;
  selector: string;
  subPath?: string;
  commit: string;
}

interface LoomPin {
  workspaceId: string;
  revision: number;
  repositories: Record<string, string>;
  pinId?: string;
}

interface LoomListResponse {
  workspace: string;
  catalog?: string;
  state: 'ready' | 'unbound' | 'unavailable';
  bindingError?: { code: string; message: string };
  pin?: LoomPin;
  vfs: {
    enabled: boolean;
    state: 'disabled' | 'collapsed' | 'ready' | 'not-configured' | 'unavailable';
    error?: { code: string; message: string };
    entries: LoomEntry[];
    mounts: LoomMount[];
    continuation?: string;
  };
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

interface VfsTreeNode extends BasicDataNode {
  key: string;
  title: string;
  path: string;
  kind: 'directory' | 'file';
  entry?: LoomEntry;
  children?: VfsTreeNode[];
}

type TreeSelectInfo = Parameters<NonNullable<TreeProps<VfsTreeNode>['onSelect']>>[1];

interface ErrorEnvelope {
  error?: { code?: string; message?: string };
}

interface SlotRegistry {
  inject(name: string, body: () => unknown): unknown;
  register(options: Record<string, unknown>, component: React.ComponentType<any>): () => void;
}

interface ClientContext {
  slots: SlotRegistry;
}

type UseSessions = <T>(selector: (state: { current?: string; byId: Record<string, { cwd?: string }> }) => T) => T;

const API = '/api/loom/vfs';
const RcTree = RcTreePackage.default;

const css = `
.loomVfsSidebar,.loomVfsDrawer{--loom-syntax-comment:#008000;--loom-syntax-punctuation:#393a34;--loom-syntax-name:#0451a5;--loom-syntax-number:#098658;--loom-syntax-string:#a31515;--loom-syntax-keyword:#0000ff;--loom-syntax-function:#795e26;--loom-syntax-variable:#811f3f}
.loomVfsSidebar{box-sizing:border-box;min-width:0;border-top:1px solid var(--dsw-alias-border-l2);padding:5px var(--dsh-sidebar-inline-padding,12px) 8px 0;color:var(--dsw-alias-label-secondary);background:var(--dsw-specific-sidebar-fill);flex:none}
.loomVfsSectionHeader{box-sizing:border-box;width:100%;height:32px;border:0;border-radius:7px;background:transparent;color:inherit;display:flex;align-items:center;gap:7px;padding:0 7px;font:inherit;cursor:pointer;text-align:left}.loomVfsSectionHeader:hover{background:var(--dsw-alias-interactive-bg-hover)}
.loomVfsDisclosure{width:12px;flex:none;color:var(--dsw-alias-label-caption);font-size:10px;transition:transform .12s}.loomVfsDisclosure[data-expanded=true]{transform:rotate(90deg)}
.loomVfsSectionIcon{width:16px;height:16px;flex:none}.loomVfsSectionTitle{min-width:0;flex:1;font-size:12px;font-weight:600;white-space:nowrap;text-overflow:ellipsis;overflow:hidden}.loomVfsSectionCount{color:var(--dsw-alias-label-caption);font-size:10px;font-weight:400}
.loomVfsSwitch{box-sizing:border-box;position:relative;width:28px;height:16px;flex:none;border:0;border-radius:999px;background:var(--dsw-alias-interactive-bg-hover);padding:0;cursor:pointer}.loomVfsSwitch::after{content:"";position:absolute;top:2px;left:2px;width:12px;height:12px;border-radius:50%;background:var(--dsw-alias-label-caption);transition:transform .15s,background .15s}.loomVfsSwitch[aria-checked=true]{background:var(--dsw-alias-state-business-primary)}.loomVfsSwitch[aria-checked=true]::after{background:#fff;transform:translateX(12px)}.loomVfsSwitch:disabled{opacity:.45;cursor:default}
.loomVfsBody{min-height:0;padding:4px 0 2px}.loomVfsToolbar{height:28px;display:flex;align-items:center;gap:6px;padding:0 4px 3px 7px}.loomVfsWorkspace{min-width:0;flex:1;color:var(--dsw-alias-label-tertiary);font-size:10px;white-space:nowrap;text-overflow:ellipsis;overflow:hidden}.loomVfsRefresh{width:25px;height:25px;border:0;border-radius:6px;background:transparent;color:var(--dsw-alias-label-tertiary);font:14px/1 inherit;cursor:pointer}.loomVfsRefresh:hover{background:var(--dsw-alias-interactive-bg-hover)}.loomVfsRefresh:disabled{opacity:.4}
.loomFolderTree{height:min(36vh,360px);min-height:80px;overflow:hidden;color:var(--dsw-alias-label-secondary);font-size:12px}.loomFolderTree-list{height:100%}.loomFolderTree-list-holder{overflow:auto!important;overscroll-behavior:contain}.loomFolderTree-list-holder-inner{min-width:max-content}.loomFolderTree-treenode{box-sizing:border-box;min-width:100%;height:28px;display:flex;align-items:center;padding:0 4px;border-radius:6px;outline:none}.loomFolderTree-treenode:hover{background:var(--dsw-alias-interactive-bg-hover)}.loomFolderTree-treenode-selected{background:var(--dsw-alias-button-ghost-active-fill)!important;color:var(--dsw-alias-state-business-primary)}.loomFolderTree-indent{display:flex;align-self:stretch}.loomFolderTree-indent-unit{width:14px}.loomFolderTree-switcher{box-sizing:border-box;width:18px;height:28px;display:flex;align-items:center;justify-content:center;flex:none;color:var(--dsw-alias-label-caption);cursor:pointer}.loomFolderTree-switcher-noop{cursor:default}.loomFolderTree-node-content-wrapper{height:28px;min-width:0;flex:1;display:flex;align-items:center;border-radius:5px;outline:none;cursor:pointer}.loomFolderTree-title{min-width:0;flex:1}.loomVfsNodeTitle{height:28px;min-width:0;display:flex;align-items:center;gap:6px}.loomVfsNodeIcon{width:14px;height:14px;flex:none;color:var(--dsw-alias-label-tertiary)}.loomVfsNodeName{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.loomFolderTree-treenode-active{outline:1px solid var(--dsw-alias-state-business-primary)}
.loomVfsMessage{padding:8px 9px 10px;color:var(--dsw-alias-label-tertiary);font-size:11px;line-height:17px}.loomVfsMessageError{color:var(--dsw-alias-state-error-primary)}
.loomVfsDrawer{box-sizing:border-box;position:fixed;z-index:1000;top:68px;right:20px;bottom:20px;width:min(720px,calc(100vw - 40px));border:1px solid var(--dsw-alias-border-l2);border-radius:12px;background:var(--dsw-alias-bg-base);color:var(--dsw-alias-label-primary);box-shadow:0 18px 52px rgba(0,0,0,.24);display:flex;flex-direction:column;overflow:hidden;pointer-events:auto}.loomVfsDrawerHeader{height:44px;flex:none;border-bottom:1px solid var(--dsw-alias-border-l2);display:flex;align-items:center;gap:8px;padding:0 10px 0 14px}.loomVfsDrawerTitle{min-width:0;flex:1;font-size:13px;font-weight:600;white-space:nowrap;text-overflow:ellipsis;overflow:hidden}.loomVfsDrawerClose{width:30px;height:30px;border:0;border-radius:7px;background:transparent;color:var(--dsw-alias-label-tertiary);font:20px/1 inherit;cursor:pointer}.loomVfsDrawerClose:hover{background:var(--dsw-alias-interactive-bg-hover)}
.loomVfsMeta{min-height:52px;flex:none;border-bottom:1px solid var(--dsw-alias-border-l2);display:flex;flex-direction:column;justify-content:center;gap:2px;padding:7px 16px}.loomVfsPath{font-size:13px;font-weight:500;white-space:nowrap;text-overflow:ellipsis;overflow:hidden}.loomVfsCoordinates{color:var(--dsw-alias-label-tertiary);font:10px/15px var(--dsh-font-mono,monospace);white-space:nowrap;text-overflow:ellipsis;overflow:hidden}.loomVfsPreview{position:relative;min-height:0;flex:1;display:flex;flex-direction:column;background:var(--dsw-alias-markdown-code-block);overflow:hidden}.loomVfsLanguage{position:absolute;z-index:1;right:14px;top:10px;border:1px solid var(--dsw-alias-border-l2);border-radius:999px;background:var(--dsw-alias-bg-base);color:var(--dsw-alias-label-tertiary);font:9px/18px var(--dsh-font-mono,monospace);padding:0 7px}.loomVfsContent{flex:1;min-height:0;margin:0;padding:18px 20px 28px;overflow:auto;color:var(--dsw-alias-label-secondary);font:var(--dsw-font-markdown-code-block-small);white-space:pre;tab-size:2}.loomVfsContent code{font:inherit}.loomVfsStatus{padding:20px;color:var(--dsw-alias-label-tertiary);font-size:12px}.loomVfsError{color:var(--dsw-alias-state-error-primary)}
.loomVfsContent .token.comment,.loomVfsContent .token.prolog,.loomVfsContent .token.doctype,.loomVfsContent .token.cdata{color:var(--loom-syntax-comment);font-style:italic}.loomVfsContent .token.punctuation{color:var(--loom-syntax-punctuation)}.loomVfsContent .token.property,.loomVfsContent .token.tag,.loomVfsContent .token.constant,.loomVfsContent .token.symbol{color:var(--loom-syntax-name)}.loomVfsContent .token.boolean,.loomVfsContent .token.number{color:var(--loom-syntax-number)}.loomVfsContent .token.string,.loomVfsContent .token.char,.loomVfsContent .token.builtin{color:var(--loom-syntax-string)}.loomVfsContent .token.atrule,.loomVfsContent .token.attr-value,.loomVfsContent .token.keyword{color:var(--loom-syntax-keyword)}.loomVfsContent .token.function,.loomVfsContent .token.class-name{color:var(--loom-syntax-function)}.loomVfsContent .token.regex,.loomVfsContent .token.variable{color:var(--loom-syntax-variable)}
@media(prefers-color-scheme:dark){.loomVfsSidebar,.loomVfsDrawer{--loom-syntax-comment:#6a9955;--loom-syntax-punctuation:#a8b0bd;--loom-syntax-name:#9cdcfe;--loom-syntax-number:#b5cea8;--loom-syntax-string:#ce9178;--loom-syntax-keyword:#c586c0;--loom-syntax-function:#dcdcaa;--loom-syntax-variable:#d16969}}
`;

function installStyle(): void {
  if (document.querySelector('style[data-plugin-css="dsh-loom/vfs-sidebar"]')) return;
  const style = document.createElement('style');
  style.dataset.pluginCss = 'dsh-loom/vfs-sidebar';
  style.textContent = css;
  document.head.append(style);
}

function FolderIcon(): React.ReactElement {
  return <svg className="loomVfsNodeIcon" aria-hidden="true" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.2"><path d="M1.75 4.25h4l1.2 1.4h7.3v6.6a1 1 0 0 1-1 1H2.75a1 1 0 0 1-1-1z" /><path d="M1.75 5.65v-2a1 1 0 0 1 1-1h3l1.2 1.6h6.3a1 1 0 0 1 1 1v.4" /></svg>;
}

function FileIcon(): React.ReactElement {
  return <svg className="loomVfsNodeIcon" aria-hidden="true" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.2"><path d="M3.25 1.75h6l3.5 3.5v9H3.25z" /><path d="M9.25 1.75v3.5h3.5" /></svg>;
}

function SectionIcon(): React.ReactElement {
  return <svg className="loomVfsSectionIcon" aria-hidden="true" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.2"><path d="M2 3.25h4l1.2 1.5H14v8H2z" /></svg>;
}

function buildTree(entries: LoomEntry[]): VfsTreeNode[] {
  const roots: VfsTreeNode[] = [];
  for (const entry of entries) {
    const parts = entry.path.split('/').filter(Boolean);
    let siblings = roots;
    for (let index = 0; index < parts.length; index += 1) {
      const title = parts[index];
      const nodePath = parts.slice(0, index + 1).join('/');
      const terminal = index === parts.length - 1;
      const leaf = terminal && entry.kind === 'file';
      let node = siblings.find((candidate) => candidate.title === title);
      if (!node) {
        node = {
          key: nodePath,
          title,
          path: nodePath,
          kind: leaf ? 'file' : 'directory',
          isLeaf: leaf,
          ...(leaf ? { entry } : { children: [] }),
        };
        siblings.push(node);
      } else if (leaf) {
        node.kind = 'file';
        node.isLeaf = true;
        node.entry = entry;
        delete node.children;
      }
      siblings = node.children ?? [];
    }
  }
  const sort = (nodes: VfsTreeNode[]): void => {
    nodes.sort((left, right) => left.kind === right.kind ? left.title.localeCompare(right.title) : left.kind === 'directory' ? -1 : 1);
    for (const node of nodes) if (node.children) sort(node.children);
  };
  sort(roots);
  return roots;
}

async function responseJson<T>(response: Response): Promise<T> {
  const body = await response.json() as T & ErrorEnvelope;
  if (!response.ok) throw new Error(body.error?.message ?? `request failed (${response.status})`);
  return body;
}

function listUrl(cwd: string, load: boolean, directory = '', continuation = ''): string {
  const params = new URLSearchParams({ cwd });
  if (load) params.set('load', '1');
  if (directory) params.set('directory', directory);
  if (continuation) params.set('continuation', continuation);
  params.set('limit', '1000');
  return `${API}?${params}`;
}

function replaceChildren(nodes: VfsTreeNode[], target: string, children: VfsTreeNode[]): VfsTreeNode[] {
  return nodes.map((node) => node.path === target ? { ...node, children } : node.children ? { ...node, children: replaceChildren(node.children, target, children) } : node);
}

function VfsNavigation({ useSessions }: { useSessions: UseSessions }): React.ReactElement {
  const cwd = useSessions((state) => state.current ? state.byId[state.current]?.cwd : undefined);
  const [expanded, setExpanded] = useState(false);
  const [listing, setListing] = useState<LoomListResponse>();
  const [loading, setLoading] = useState(false);
  const [expandedKeys, setExpandedKeys] = useState<React.Key[]>([]);
  const [treeData, setTreeData] = useState<VfsTreeNode[]>([]);
  const [selected, setSelected] = useState<LoomEntry>();
  const [file, setFile] = useState<LoomReadResponse>();
  const [reading, setReading] = useState(false);
  const [error, setError] = useState<string>();

  const load = async (includeTree: boolean): Promise<void> => {
    if (!cwd) {
      setListing(undefined);
      return;
    }
    setLoading(true);
    setError(undefined);
    try {
      const response = await responseJson<LoomListResponse>(await fetch(listUrl(cwd, includeTree)));
      setListing(response);
      setTreeData(includeTree ? buildTree(response.vfs.entries) : []);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    setExpanded(false);
    setSelected(undefined);
    setFile(undefined);
    setExpandedKeys([]);
    setTreeData([]);
    void load(false);
  // load deliberately follows only the host Workspace cwd.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [cwd]);

  const enabled = listing?.vfs?.enabled === true;
  const loadChildren = async (node: VfsTreeNode): Promise<void> => {
    if (!cwd || node.kind !== 'directory') return;
    let continuation = '';
    const entries: LoomEntry[] = [];
    do {
      const response = await responseJson<LoomListResponse>(await fetch(listUrl(cwd, true, node.path, continuation)));
      entries.push(...response.vfs.entries);
      continuation = response.vfs.continuation ?? '';
    } while (continuation);
    const prefix = `${node.path}/`;
    const children = entries.map((entry) => ({
      key: entry.path, title: entry.path.slice(prefix.length), path: entry.path,
      kind: entry.kind, isLeaf: entry.kind === 'file', entry: entry.kind === 'file' ? entry : undefined,
      ...(entry.kind === 'directory' ? { children: [] } : {}),
    } satisfies VfsTreeNode));
    setTreeData((current) => replaceChildren(current, node.path, children));
  };

  const toggleSection = (): void => {
    if (!enabled) return;
    const next = !expanded;
    setExpanded(next);
    if (next && listing?.vfs?.state !== 'ready') void load(true);
  };

  const toggleEnabled = async (event: React.MouseEvent): Promise<void> => {
    event.stopPropagation();
    if (!cwd || listing?.state === 'unbound') return;
    const next = !enabled;
    setLoading(true);
    setError(undefined);
    try {
      await responseJson(await fetch(API, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ action: 'set-vfs-enabled', cwd, enabled: next }),
      }));
      setExpanded(next);
      setSelected(undefined);
      setFile(undefined);
      await load(next);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
      setLoading(false);
    }
  };

  const openFile = async (entry: LoomEntry): Promise<void> => {
    if (!cwd || !listing?.pin) return;
    setSelected(entry);
    setFile(undefined);
    setReading(true);
    setError(undefined);
    try {
      const params = new URLSearchParams({ cwd, path: entry.path, pin: JSON.stringify(listing.pin) });
      setFile(await responseJson<LoomReadResponse>(await fetch(`${API}?${params}`)));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setReading(false);
    }
  };

  let message: string | undefined;
  if (!cwd || listing?.state === 'unbound') message = '当前 DSH Workspace 尚未连接 Catalog Workspace。';
  else if (listing?.vfs?.state === 'not-configured') message = '这个 Workspace 没有配置 VFS mount。';
  else if (listing?.vfs?.state === 'unavailable') message = listing.vfs.error?.message ?? 'VFS 暂不可用。';
  else if (listing?.vfs?.state === 'ready' && treeData.length === 0) message = 'VFS 为空。';

  return <>
    <section className="loomVfsSidebar" aria-label="Workspace VFS">
      <div
        className="loomVfsSectionHeader"
        role="button"
        tabIndex={0}
        onClick={toggleSection}
        onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') toggleSection(); }}
        aria-expanded={enabled && expanded}
      >
        <span className="loomVfsDisclosure" data-expanded={enabled && expanded}>▶</span>
        <SectionIcon />
        <span className="loomVfsSectionTitle">VFS {listing?.workspace ? <span className="loomVfsSectionCount">· {listing.workspace}</span> : null}</span>
        <button
          className="loomVfsSwitch"
          type="button"
          role="switch"
          aria-label="Enable VFS for this Workspace"
          aria-checked={enabled}
          disabled={!cwd || listing?.state === 'unbound' || !listing?.vfs || loading}
          onClick={(event) => { void toggleEnabled(event); }}
        />
      </div>
      {enabled && expanded ? <div className="loomVfsBody">
        <div className="loomVfsToolbar">
          <span className="loomVfsWorkspace" title={cwd}>{listing?.catalog ?? 'default catalog'}</span>
          <button className="loomVfsRefresh" type="button" aria-label="Refresh VFS" disabled={loading} onClick={() => { void load(true); }}>↻</button>
        </div>
        {loading && listing?.vfs?.state !== 'ready' ? <div className="loomVfsMessage">正在载入 VFS…</div> : null}
        {!loading && message ? <div className={`loomVfsMessage${listing?.vfs?.state === 'unavailable' || error ? ' loomVfsMessageError' : ''}`}>{error ?? message}</div> : null}
        {!loading && !message && listing?.vfs?.state === 'ready' ? <RcTree<VfsTreeNode>
          prefixCls="loomFolderTree"
          treeData={treeData}
          height={360}
          itemHeight={28}
          virtual
          focusable
          selectedKeys={selected ? [selected.path] : []}
          expandedKeys={expandedKeys}
          loadData={loadChildren}
          onExpand={(keys: React.Key[]) => setExpandedKeys(keys)}
          onSelect={(_keys: React.Key[], info: TreeSelectInfo) => {
            const node = info.node as VfsTreeNode;
            if (node.kind === 'file' && node.entry) void openFile(node.entry);
          }}
          switcherIcon={({ expanded: nodeExpanded, isLeaf }: TreeNodeProps) => isLeaf ? null : <span aria-hidden="true">{nodeExpanded ? '⌄' : '›'}</span>}
          titleRender={(node: VfsTreeNode) => <span className="loomVfsNodeTitle" title={node.path}>
            {node.kind === 'directory' ? <FolderIcon /> : <FileIcon />}
            <span className="loomVfsNodeName">{node.title}</span>
          </span>}
        /> : null}
      </div> : null}
    </section>
    {selected ? <aside className="loomVfsDrawer" role="dialog" aria-label={`Preview ${selected.path}`}>
      <header className="loomVfsDrawerHeader">
        <span className="loomVfsDrawerTitle">VFS preview</span>
        <button className="loomVfsDrawerClose" type="button" aria-label="Close preview" onClick={() => { setSelected(undefined); setFile(undefined); }}>×</button>
      </header>
      <div className="loomVfsMeta">
        <div className="loomVfsPath">{selected.path}</div>
        <div className="loomVfsCoordinates">{file?.repository ?? selected.repository} · {(file?.commit ?? selected.commit).slice(0, 12)}{file ? ` · ${file.size} bytes` : ''}</div>
      </div>
      <main className="loomVfsPreview">
        {file && !file.binary && file.content !== undefined ? <>
          <SyntaxPreview content={file.content} path={file.path} />
        </> : <div className={`loomVfsStatus${error ? ' loomVfsError' : ''}`}>{error ?? (reading ? '正在读取文件…' : file?.binary ? '二进制文件不提供文本预览。' : '没有可预览的内容。')}</div>}
      </main>
    </aside> : null}
  </>;
}

function SidebarVfsPortal({ useSessions }: { useSessions: UseSessions }): React.ReactElement | null {
  const [target, setTarget] = useState<Element | null>(null);
  const [wide, setWide] = useState(false);

  useEffect(() => {
    let watched: Node = document.body;
    const locate = (): void => {
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
    const measure = (): void => setWide(parent.getBoundingClientRect().width > 100);
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(parent);
    return () => observer.disconnect();
  }, [target]);

  return target && wide ? createPortal(<VfsNavigation useSessions={useSessions} />, target) : null;
}

export function apply(ctx: ClientContext): void {
  installStyle();
  ctx.slots.inject('shell.overlay', () => ctx.slots.register({
    name: 'shell.overlay',
    id: 'loom-vfs-sidebar',
    order: 40,
  }, SidebarVfsPortal));
}
