// The pinned plugin's package exports omit its existing declaration file.
declare module 'vite-plugin-eslint' {
    import type { Plugin } from 'vite';
    type Options = { include?: string | string[]; exclude?: string | string[] };
    export default function eslintPlugin(options?: Options): Plugin;
}
