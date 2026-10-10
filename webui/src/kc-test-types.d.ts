import 'react-dom/client';

// testing-library 16 names this React 18 createRoot parameter as Container.
// Keep the pinned runtime and derive the alias from its actual declaration.
declare module 'react-dom/client' {
    type Container = Parameters<typeof createRoot>[0];
}
