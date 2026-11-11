import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';
import IsolationLab from './IsolationLab';
import './style.css';
import {startTheme} from './theme';
startTheme();
createRoot(document.getElementById('root')!).render(<React.StrictMode>{new URLSearchParams(location.search).has('isolation') ? <IsolationLab /> : <App />}</React.StrictMode>);
