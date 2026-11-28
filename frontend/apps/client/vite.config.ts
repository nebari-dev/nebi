import { readFileSync } from 'node:fs';
import type { IncomingMessage } from 'node:http';
import os from 'node:os';
import path from 'node:path';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

const backendPort = process.env.NEBI_SERVER_PORT || '8460';
const devPort = 8461;

// nebi-web serves the client API on 127.0.0.1 only, refuses requests for any
// other Host, and needs its session token (#680). The proxy sends the token
// from owner.json in nebi-web's data directory, which nebi-web rewrites with
// a new token each time it starts.
function dataDir(): string {
  if (process.env.NEBI_DATA_DIR) return process.env.NEBI_DATA_DIR;
  const home = os.homedir();
  switch (process.platform) {
    case 'darwin':
      return path.join(home, 'Library', 'Application Support', 'nebi');
    case 'win32':
      return path.join(
        process.env.APPDATA || path.join(home, 'AppData', 'Roaming'),
        'nebi',
      );
    default:
      return path.join(home, '.local', 'share', 'nebi');
  }
}

function sessionToken(): string | undefined {
  try {
    const owner = JSON.parse(
      readFileSync(path.join(dataDir(), 'owner.json'), 'utf8'),
    );
    // Only the process listening where the proxy sends requests gets it.
    if (
      owner.address === `127.0.0.1:${backendPort}` &&
      typeof owner.token === 'string'
    ) {
      return owner.token;
    }
  } catch {
    // nebi-web is not running yet: the API answers 401 until it is.
  }
  return undefined;
}

// Only requests from the dev server's own pages get the token, so other
// web pages cannot use the proxy to reach the API.
function fromDevServer(req: IncomingMessage): boolean {
  const site = req.headers['sec-fetch-site'];
  if (site !== undefined && site !== 'same-origin' && site !== 'none')
    return false;
  const origin = req.headers.origin;
  return (
    origin === undefined ||
    origin === `http://localhost:${devPort}` ||
    origin === `http://127.0.0.1:${devPort}`
  );
}

export default defineConfig({
  plugins: [react()],
  envDir: '../..',
  publicDir: '../../public',
  resolve: { alias: { '@': path.resolve(__dirname, './src') } },
  build: { outDir: '../../dist/client', emptyOutDir: true },
  server: {
    port: devPort,
    proxy: {
      '/api': {
        target: `http://127.0.0.1:${backendPort}`,
        changeOrigin: true,
        configure: (proxy) => {
          proxy.on('proxyReq', (proxyReq, req) => {
            const token = fromDevServer(req) ? sessionToken() : undefined;
            if (token) {
              proxyReq.setHeader('Authorization', `Bearer ${token}`);
            } else {
              proxyReq.removeHeader('Authorization');
            }
          });
        },
      },
    },
  },
});
