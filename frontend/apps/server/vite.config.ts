import path from 'node:path';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

const backendPort = process.env.NEBI_SERVER_PORT || '8460';

export default defineConfig({
  plugins: [react()],
  envDir: '../..',
  publicDir: '../../public',
  resolve: { alias: { '@': path.resolve(__dirname, './src') } },
  build: { outDir: '../../dist/server', emptyOutDir: true },
  server: {
    port: 8461,
    proxy: {
      '/api': { target: `http://localhost:${backendPort}`, changeOrigin: true },
    },
  },
});
