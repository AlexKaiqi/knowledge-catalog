import react from '@vitejs/plugin-react-swc';
import eslintPlugin from 'vite-plugin-eslint';
import replace from '@rollup/plugin-replace';
import { writeFileSync } from 'node:fs';
import type { ConfigEnv, Connect, Plugin, ProxyOptions } from 'vite';

const canonicalUIBase: Connect.NextHandleFunction = (request, response, next) => {
  const url = request.url || '';
  if ((request.method === 'GET' || request.method === 'HEAD') && (url === '/ui' || url.startsWith('/ui?'))) {
    response.statusCode = 308;
    response.setHeader('Location', `/ui/${url.slice(3)}`);
    response.end();
    return;
  }
  next();
};
const canonicalUIBasePlugin: Plugin = {
  name: 'kc-ui-canonical-base',
  configureServer(server) { server.middlewares.use(canonicalUIBase); },
  configurePreviewServer(server) { server.middlewares.use(canonicalUIBase); },
};

// https://vitejs.dev/config/
export default ({ command }: ConfigEnv) => {
  const target = process.env.KC_UI_SERVER || 'http://127.0.0.1:7380';
  const authTarget = process.env.KC_UI_AUTH_SERVER;
  const nativeAuthProxy: ProxyOptions = {
    target: authTarget, changeOrigin: false,
    configure(proxy) {
      proxy.on('proxyRes', response => {
        const location = response.headers.location;
        // Keep native browser navigation and external IdP redirects intact.
        // Only lakeFS's root-relative login page moves under the UI basename.
        if (location && /^\/auth\/login(?:[?#]|$)/.test(location)) response.headers.location = `/ui${location}`;
      });
    },
  };
  const proxy = {
    ...Object.fromEntries(['/identity/v1', '/snapshot/v1', '/knowledge/v1', '/admin/v1']
      .map((prefix) => [prefix, {target, changeOrigin: false}] as const)),
    '/api/v1': {target, changeOrigin: false},
    ...(authTarget ? {'/logout': nativeAuthProxy, '/oidc': nativeAuthProxy} : {}),
    // Every data request uses the product gateway; only original auth callbacks go native.
    '^/(?!ui(?:/|$)|identity/v1|snapshot/v1|knowledge/v1|admin/v1|api/v1|logout|oidc)': {target, changeOrigin: false},
  };
  const baseConfig = {
    base: '/ui/',
    test: {
      environment: 'happy-dom',
      setupFiles: ['./test/setup.ts'],
      exclude: ["./test/e2e/**/*", "./node_modules/**/*"],
    },
    plugins: [
      canonicalUIBasePlugin,
      replace({
          preventAssignment: true,
          include: ['src/**/*.jsx', 'src/**/*.js'],
          values: {
            __buildVersion: process.env.VERSION || 'dev',
          }
      }),
      react(),
      eslintPlugin({
        include: ['src/**/*.jsx', 'src/**/*.js', 'src/**/*.ts', 'src/**/*.tsx']
      }),
    ],
    publicDir: './pub',
    preview: {port: 3000, proxy},
  };

  // in development
  if (command === 'serve') {
    return {
      ...baseConfig,
      server: {
        port: 3000,
        proxy
      },
      build: {
        sourcemap: 'inline',
        outDir: '../webapp',
      },
    };
  }
  // while building
  return {
    ...baseConfig,
    // Snapshot Store owns the embedded console: go:embed can only
    // reference paths inside the package, so the bundles land in snapshot/console/webapp.
    plugins: [...baseConfig.plugins, {
      name: 'kc-embed-placeholder',
      closeBundle() { writeFileSync(new URL('../webapp/.gitkeep', import.meta.url), ''); },
    }],
    build: {
      outDir: '../webapp',
      emptyOutDir: true,
    },
  };
};
