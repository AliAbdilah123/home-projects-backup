import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  base: '/projects/omnichannel-chat-hub/',
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api/v1': 'http://127.0.0.1:8080'
    }
  }
});
