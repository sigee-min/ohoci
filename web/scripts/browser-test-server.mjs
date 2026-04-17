import fs from 'node:fs/promises';
import { createServer as createHttpServer } from 'node:http';
import os from 'node:os';
import path from 'node:path';

import { createServer as createViteServer } from 'vite';

import { createAppViteConfig } from '../vite.config.js';

const DEFAULT_WEB_ROOT = path.resolve(import.meta.dirname, '..');

export async function startBrowserTestServer(options = {}) {
  const {
    root = DEFAULT_WEB_ROOT,
    extraPlugins = []
  } = options;

  const cacheDir = await fs.mkdtemp(path.join(os.tmpdir(), 'ohoci-vite-browser-test-'));
  const httpServer = createHttpServer();
  const vite = await createViteServer(createAppViteConfig({
    root,
    cacheDir,
    logLevel: 'error',
    extraPlugins,
    reactOptions: {
      fastRefresh: false
    },
    server: {
      middlewareMode: true,
      hmr: false
    }
  }));

  httpServer.on('request', (request, response) => {
    vite.middlewares(request, response, (error) => {
      response.statusCode = 500;
      response.setHeader('Content-Type', 'text/plain; charset=utf-8');
      response.end(error instanceof Error ? error.stack || error.message : String(error));
    });
  });

  await new Promise((resolve, reject) => {
    httpServer.once('error', reject);
    httpServer.listen(0, '127.0.0.1', () => resolve());
  });

  const address = httpServer.address();
  if (!address || typeof address === 'string') {
    await new Promise((resolve) => httpServer.close(() => resolve()));
    await vite.close();
    await fs.rm(cacheDir, { recursive: true, force: true });
    throw new Error('Could not determine test server port.');
  }

  return {
    server: {
      async close() {
        await new Promise((resolve, reject) => {
          httpServer.close((error) => {
            if (error) {
              reject(error);
              return;
            }
            resolve();
          });
        });
        await vite.close();
        await fs.rm(cacheDir, { recursive: true, force: true });
      }
    },
    baseUrl: `http://127.0.0.1:${address.port}`
  };
}
