import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

// In development Vite serves the UI and forwards /api to the Go server, so the
// browser sees one origin and the session cookie just works.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: { '/api': { target: 'http://localhost:8080', changeOrigin: false } },
  },
});
