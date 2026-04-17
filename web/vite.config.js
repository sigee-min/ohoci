import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { fileURLToPath, URL } from 'node:url';

const defaultAllowedHosts = ['localhost', '127.0.0.1', '.ngrok-free.dev', '.ngrok.app'];

function readAllowedHosts() {
  const extraHosts = (process.env.OHOCI_VITE_ALLOWED_HOSTS ?? '')
    .split(',')
    .map((value) => value.trim())
    .filter(Boolean);
  return Array.from(new Set([...defaultAllowedHosts, ...extraHosts]));
}

export function createAppViteConfig(options = {}) {
  const {
    extraPlugins = [],
    reactOptions = {},
    server: serverOverrides = {},
    ...configOverrides
  } = options;

  return defineConfig({
    plugins: [react(reactOptions), tailwindcss(), ...extraPlugins],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url))
      }
    },
    build: {
      outDir: 'dist',
      emptyOutDir: true
    },
    server: {
      port: 5173,
      allowedHosts: readAllowedHosts(),
      proxy: {
        '/api': 'http://localhost:8080'
      },
      ...serverOverrides
    },
    ...configOverrides
  });
}

export default createAppViteConfig();
