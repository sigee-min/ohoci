import React from 'react';
import ReactDOM from 'react-dom/client';
import { AppToaster } from '@/components/ui/toaster';
import { TooltipProvider } from '@/components/ui/tooltip';
import { I18nProvider } from '@/i18n';
import { App } from './App.jsx';
import './styles.css';

ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <I18nProvider>
      <TooltipProvider>
        <App />
        <AppToaster />
      </TooltipProvider>
    </I18nProvider>
  </React.StrictMode>
);
