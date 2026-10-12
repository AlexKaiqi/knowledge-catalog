import React from 'react';
import { createRoot } from 'react-dom/client';
// styles
import 'bootstrap/dist/css/bootstrap.css';
import './styles/globals.css';

// Extend the vendored lakeFS application with KC Dataset routes.
import KCApp from './extendable/kcApp';

const container = document.getElementById('root');
if (!container) throw new Error("Failed to find root element!");

const root = createRoot(container);
root.render(<KCApp />);