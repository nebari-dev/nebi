import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

const backendPort = process.env.NEBI_SERVER_PORT || '8460'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 8461,
    proxy: {
      '/api': {
        target: `http://localhost:${backendPort}`,
        changeOrigin: true,
      },
    },
  },
})
