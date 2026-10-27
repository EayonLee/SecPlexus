import { fileURLToPath, URL } from 'node:url';
import react from '@vitejs/plugin-react';
import { defineConfig, loadEnv } from 'vite';
import svgr from 'vite-plugin-svgr';
import { vitePluginForArco } from '@arco-plugins/vite-react';
import setting from './src/settings.json' with { type: 'json' };
import { themeTokens } from './src/theme.ts';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  return {
    plugins: [
      react(),
      svgr({ include: '**/*.svg' }),
      vitePluginForArco({
        modifyVars: { ...themeTokens, 'arcoblue-6': setting.themeColor },
      }),
    ],
    resolve: {
      alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
    },
    css: {
      preprocessorOptions: {
        less: { javascriptEnabled: true, math: 'always' },
      },
    },
    server: {
      host: '127.0.0.1',
      port: 5173,
      strictPort: true,
      proxy: env.VITE_API_PROXY_TARGET
        ? { '/api': { target: env.VITE_API_PROXY_TARGET, changeOrigin: true } }
        : undefined,
    },
    preview: { host: '127.0.0.1', port: 4173, strictPort: true },
  };
});
