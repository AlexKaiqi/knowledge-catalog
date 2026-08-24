/** Browser surface for dsh-loom's read-only Workspace VFS explorer. */

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { createPortal } from 'react-dom';

export const inject = ['slots'];

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
}

const API = '/api/loom/vfs';

const css = `
.loomVfsRoot{width:100%;height:42px;margin-top:8px;display:flex;align-items:center}
.loomVfsRoot[data-rail=true]{width:36px;height:36px;margin:0}
.loomVfsTrigger{width:calc(100% + 4px);height:42px;margin:0 -2px;padding:0 10px 0 8px;border:0;border-radius:12px;background:transparent;color:var(--dsw-alias-label-primary);font:inherit;font-size:14px;display:flex;align-items:center;gap:8px;cursor:pointer;overflow:hidden}
.loomVfsRoot[data-rail=true] .loomVfsTrigger{width:36px;height:36px;margin:0;padding:0;border-radius:50%;justify-content:center}
.loomVfsTrigger:hover,.loomVfsTrigger[data-active=true]{background:var(--dsw-alias-interactive-bg-hover)}
.loomVfsTriggerLabel{min-width:0;white-space:nowrap;text-overflow:ellipsis;overflow:hidden}
.loomVfsBackdrop{position:fixed;inset:0;z-index:80;background:rgba(0,0,0,.34);display:flex;align-items:center;justify-content:center;padding:28px}
.loomVfsPanel{width:min(1120px,calc(100vw - 56px));height:min(760px,calc(100vh - 56px));border:1px solid var(--dsw-alias-border-inverted);border-radius:16px;background:var(--dsw-alias-bg-base);box-shadow:var(--dsw-shadow-lv3);color:var(--dsw-alias-label-primary);display:flex;flex-direction:column;overflow:hidden}
.loomVfsHeader{height:58px;flex:none;border-bottom:1px solid var(--dsw-alias-border-l2);display:flex;align-items:center;gap:12px;padding:0 18px}
.loomVfsTitle{font-size:16px;font-weight:600}.loomVfsContext{min-width:0;color:var(--dsw-alias-label-tertiary);font-size:12px;white-space:nowrap;text-overflow:ellipsis;overflow:hidden}
.loomVfsHeaderActions{margin-left:auto;display:flex;align-items:center;gap:8px}
.loomVfsButton{height:30px;padding:0 11px;border:1px solid var(--dsw-alias-border-l2);border-radius:8px;background:transparent;color:var(--dsw-alias-label-secondary);font:inherit;cursor:pointer}.loomVfsButton:hover{background:var(--dsw-alias-interactive-bg-hover)}.loomVfsButton:disabled{opacity:.5;cursor:default}
.loomVfsClose{width:30px;padding:0;font-size:19px;line-height:1}
.loomVfsBody{flex:1;min-height:0;display:grid;grid-template-columns:minmax(280px,36%) minmax(0,1fr)}
.loomVfsNav{min-width:0;border-right:1px solid var(--dsw-alias-border-l2);display:flex;flex-direction:column}
.loomVfsSearchWrap{padding:12px;border-bottom:1px solid var(--dsw-alias-border-l2)}
.loomVfsSearch{box-sizing:border-box;width:100%;height:34px;border:1px solid var(--dsw-alias-border-l2);border-radius:9px;background:transparent;color:var(--dsw-alias-label-primary);font:inherit;padding:0 10px;outline:none}.loomVfsSearch:focus{border-color:var(--dsw-alias-state-business-primary)}
.loomVfsMountSection{flex:none;max-height:178px;border-bottom:1px solid var(--dsw-alias-border-l2);padding:9px 12px 10px;overflow:auto}.loomVfsMountTitle{color:var(--dsw-alias-label-caption);text-transform:uppercase;letter-spacing:.05em;margin-bottom:5px;font-size:10px;font-weight:600}.loomVfsMount{border-radius:7px;padding:5px 7px}.loomVfsMount:hover{background:var(--dsw-alias-interactive-bg-hover)}.loomVfsMountHead{display:flex;align-items:center;gap:7px}.loomVfsMountPath{color:var(--dsw-alias-label-primary);font:12px/17px var(--dsh-font-mono,monospace);font-weight:600}.loomVfsMountRepo{min-width:0;color:var(--dsw-alias-state-business-primary);font-size:11px;white-space:nowrap;text-overflow:ellipsis;overflow:hidden}.loomVfsMountCoord{color:var(--dsw-alias-label-caption);font:10px/15px var(--dsh-font-mono,monospace);white-space:nowrap;text-overflow:ellipsis;overflow:hidden}
.loomVfsTree{flex:1;min-height:0;padding:8px;overflow:auto}.loomVfsRow{box-sizing:border-box;width:100%;height:29px;border:0;border-radius:7px;background:transparent;color:var(--dsw-alias-label-secondary);font:inherit;font-size:13px;text-align:left;display:flex;align-items:center;gap:6px;padding-right:8px;cursor:pointer}.loomVfsRow:hover{background:var(--dsw-alias-interactive-bg-hover)}.loomVfsRow[data-selected=true]{background:var(--dsw-alias-button-ghost-active-fill);color:var(--dsw-alias-state-business-primary)}
.loomVfsChevron{width:12px;flex:none;text-align:center;color:var(--dsw-alias-label-caption)}.loomVfsFileIcon{width:14px;flex:none;color:var(--dsw-alias-label-tertiary)}.loomVfsName{min-width:0;white-space:nowrap;text-overflow:ellipsis;overflow:hidden}
.loomVfsMountBadge{max-width:46%;margin-left:auto;border-radius:999px;background:var(--dsw-alias-button-ghost-active-fill);color:var(--dsw-alias-state-business-primary);padding:1px 6px;font-size:10px;white-space:nowrap;text-overflow:ellipsis;overflow:hidden}
.loomVfsEmpty{padding:20px;color:var(--dsw-alias-label-tertiary);font-size:13px;line-height:20px}
.loomVfsPreview{min-width:0;display:flex;flex-direction:column;background:var(--dsw-alias-markdown-code-block)}
.loomVfsMeta{min-height:58px;flex:none;border-bottom:1px solid var(--dsw-alias-border-l2);background:var(--dsw-alias-bg-base);display:flex;flex-direction:column;justify-content:center;gap:3px;padding:8px 16px}.loomVfsPath{font-size:14px;font-weight:500;white-space:nowrap;text-overflow:ellipsis;overflow:hidden}.loomVfsCoordinates{color:var(--dsw-alias-label-tertiary);font:11px/16px var(--dsh-font-mono,monospace);white-space:nowrap;text-overflow:ellipsis;overflow:hidden}
.loomVfsContent{flex:1;min-height:0;margin:0;padding:16px;overflow:auto;color:var(--dsw-alias-label-secondary);font:var(--dsw-font-markdown-code-block-small);white-space:pre;tab-size:2}.loomVfsStatus{padding:20px;color:var(--dsw-alias-label-tertiary);font-size:13px}.loomVfsError{color:var(--dsw-alias-state-error-primary)}
@media(max-width:720px){.loomVfsBackdrop{padding:10px}.loomVfsPanel{width:calc(100vw - 20px);height:calc(100vh - 20px)}.loomVfsBody{grid-template-columns:42% minmax(0,1fr)}.loomVfsContext{display:none}}
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
        {node.mount && <span className="loomVfsMountBadge" title={node.mount.repository}>{node.mount.repository.split('/').at(-1)}</span>}
      </button>
      {node.type === 'directory' && open && <TreeRows nodes={node.children} depth={depth + 1} expanded={expanded} selected={selected} onToggle={onToggle} onSelect={onSelect} />}
    </React.Fragment>;
  })}</>;
}

function VfsExplorer({ onClose }: { onClose(): void }): React.ReactElement {
  const [listing, setListing] = useState<LoomListResponse>();
  const [file, setFile] = useState<LoomReadResponse>();
  const [selected, setSelected] = useState<string>();
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [query, setQuery] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(undefined);
    try {
      const next = await getJson<LoomListResponse>(API);
      setListing(next);
      const topDirectories = buildTree(next.entries, next.mounts).filter((node) => node.type === 'directory').map((node) => node.path);
      setExpanded((current) => current.size > 0 ? current : new Set(topDirectories));
      if (selected) {
        if (next.entries.some((entry) => entry.path === selected)) {
          setFile(await getJson<LoomReadResponse>(`${API}?path=${encodeURIComponent(selected)}`));
        } else {
          setSelected(undefined);
          setFile(undefined);
        }
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setLoading(false);
    }
  }, [selected]);

  useEffect(() => { void refresh(); }, []);
  useEffect(() => {
    const key = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose(); };
    document.addEventListener('keydown', key);
    return () => document.removeEventListener('keydown', key);
  }, [onClose]);

  const select = useCallback(async (path: string) => {
    setSelected(path);
    setFile(undefined);
    setError(undefined);
    try {
      setFile(await getJson<LoomReadResponse>(`${API}?path=${encodeURIComponent(path)}`));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }, []);

  const tree = useMemo(() => buildTree(listing?.entries ?? [], listing?.mounts ?? []), [listing]);
  const matches = useMemo(() => {
    const value = query.trim().toLocaleLowerCase();
    return value ? (listing?.entries ?? []).filter((entry) => entry.path.toLocaleLowerCase().includes(value)) : [];
  }, [listing, query]);
  const toggle = (path: string) => setExpanded((current) => {
    const next = new Set(current);
    if (next.has(path)) next.delete(path); else next.add(path);
    return next;
  });

  return createPortal(<div className="loomVfsBackdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
    <section className="loomVfsPanel" role="dialog" aria-modal="true" aria-label="Catalog VFS">
      <header className="loomVfsHeader">
        <IconTree />
        <span className="loomVfsTitle">Catalog VFS</span>
        <span className="loomVfsContext">Workspace: {listing?.workspace ?? '…'}{listing?.catalog ? ` · ${listing.catalog}` : ''} · {listing?.entries.length ?? 0} files · {listing?.mounts.length ?? 0} mounts</span>
        <div className="loomVfsHeaderActions">
          <button type="button" className="loomVfsButton" disabled={loading} onClick={() => void refresh()}>{loading ? '刷新中…' : '刷新'}</button>
          <button type="button" className="loomVfsButton loomVfsClose" aria-label="关闭" onClick={onClose}>×</button>
        </div>
      </header>
      <div className="loomVfsBody">
        <nav className="loomVfsNav" aria-label="VFS files">
          <div className="loomVfsSearchWrap"><input className="loomVfsSearch" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="过滤路径…" aria-label="过滤 VFS 路径" /></div>
          <div className="loomVfsMountSection">
            <div className="loomVfsMountTitle">Mounts</div>
            {(listing?.mounts ?? []).map((mount) => <div className="loomVfsMount" key={`${mount.path}:${mount.repository}`} title={`${mount.repository} @ ${mount.commit}`}>
              <div className="loomVfsMountHead"><span className="loomVfsMountPath">{mount.path ? `/${mount.path}` : '/ (root)'}</span><span className="loomVfsMountRepo">→ {mount.repository}</span></div>
              <div className="loomVfsMountCoord">{mount.selector}{mount.subPath ? ` · subPath ${mount.subPath}` : ''} · {mount.commit.slice(0, 12)}</div>
            </div>)}
          </div>
          <div className="loomVfsTree">
            {query.trim() ? matches.map((entry) => <button key={entry.path} type="button" className="loomVfsRow" data-selected={selected === entry.path} title={entry.path} onClick={() => void select(entry.path)}><span className="loomVfsChevron" /><span className="loomVfsFileIcon">·</span><span className="loomVfsName">{entry.path}</span></button>)
              : tree.length > 0 ? <TreeRows nodes={tree} depth={0} expanded={expanded} selected={selected} onToggle={toggle} onSelect={(path) => void select(path)} />
                : <div className="loomVfsEmpty">{loading ? '正在读取 Workspace…' : 'Workspace 中没有可浏览的文件。'}</div>}
          </div>
        </nav>
        <main className="loomVfsPreview">
          {file ? <>
            <div className="loomVfsMeta"><div className="loomVfsPath">{file.path} · {formatBytes(file.size)}{file.truncated ? ' · 预览已截断' : ''}</div><div className="loomVfsCoordinates">{file.repository} @ {file.commit}</div></div>
            {file.binary ? <div className="loomVfsStatus">二进制文件，不提供文本预览。</div> : <pre className="loomVfsContent">{file.content}</pre>}
          </> : <div className={`loomVfsStatus${error ? ' loomVfsError' : ''}`}>{error ?? '从左侧选择一个文件。这里显示的是本次读取的 Repository 与 commit 坐标。'}</div>}
        </main>
      </div>
    </section>
  </div>, document.body);
}

function VfsFooterAction({ wide }: { wide: boolean }): React.ReactElement {
  const [open, setOpen] = useState(false);
  return <div className="loomVfsRoot" data-rail={!wide}>
    <button type="button" className="loomVfsTrigger" data-active={open} title="Catalog VFS" aria-label="打开 Catalog VFS" onClick={() => setOpen(true)}>
      <IconTree />
      {wide && <span className="loomVfsTriggerLabel">Catalog VFS</span>}
    </button>
    {open && <VfsExplorer onClose={() => setOpen(false)} />}
  </div>;
}

export function apply(ctx: ClientContext): void {
  installStyle();
  ctx.slots.inject('sidebar.footer.action', () => ctx.slots.register({
    name: 'sidebar.footer.action',
    id: 'loom-vfs-browser',
  }, VfsFooterAction));
}
