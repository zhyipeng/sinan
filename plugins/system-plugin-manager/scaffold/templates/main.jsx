import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './App.jsx';
import { ready } from './bridge.js';
import './style.css';

// Navigation and detach controls belong to the host, not this business view.
// Keep the bridge handshake outside React effects: it must be sent exactly once.
createRoot(document.getElementById('root')).render(<App />);
ready();
