import react from '@vitejs/plugin-react';
import path from 'path';
import { defineConfig } from 'vite';

// https://vite.dev/config/
const backendPort = process.env.NEBI_SERVER_PORT || '8460';

export default defineConfig({
	plugins: [react()],
	resolve: {
		alias: {
			'@': path.resolve(__dirname, './src'),
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
});
