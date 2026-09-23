import React from 'react';
import { createRoot } from 'react-dom/client';
// styles
import 'bootstrap/dist/css/bootstrap.css';
import './styles/globals.css';

// KC console app. The vendored lakeFS entry (extendable/lakefsApp.tsx) and its
// pages stay in the tree for reference but are not routed; see KC-VENDOR.md.
import KCApp from './extendable/kcApp';

const container = document.getElementById('root');
if (!container) throw new Error("Failed to find root element!");

const root = createRoot(container);
root.render(<KCApp />);