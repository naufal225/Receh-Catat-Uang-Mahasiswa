import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import App, { ModuleProviders } from './App';
import { ConfirmProvider, ToastProvider } from './components/ui';
import { AppProvider } from './lib/app';
import { AuthProvider } from './lib/auth';
import './styles.css';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ToastProvider>
      <ConfirmProvider>
        <AuthProvider>
          <AppProvider>
            <ModuleProviders>
              <App />
            </ModuleProviders>
          </AppProvider>
        </AuthProvider>
      </ConfirmProvider>
    </ToastProvider>
  </StrictMode>,
);
