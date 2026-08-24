export default {
  input: 'src/browser.tsx',
  external: [
    'react',
    'react/jsx-runtime',
    'react-dom',
  ],
  output: {
    file: 'dist/web-client.js',
    format: 'cjs',
    banner: `window.__ModuleLoader__.load({
  id: "dsh-loom",
  factory: (require) => {
    var module = { exports: {} };
    var exports = module.exports;`,
    footer: `    return module.exports;
  }
});`,
    sourcemap: true,
  },
};
