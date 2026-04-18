import test from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';

import { chromium } from 'playwright';

import { startBrowserTestServer } from './browser-test-server.mjs';

const WEB_ROOT = path.resolve(import.meta.dirname, '..');
const GENERATED_DOCS_PATH = path.resolve(WEB_ROOT, 'src/generated/docs-data.js');
const FIXTURE_DOCS = [
  {
    slug: 'setup-guide',
    title: 'Setup guide',
    order: 10,
    section: 'Basics',
    sectionKey: 'docs.section.basics',
    summary: 'Fixture entry point for public docs navigation.',
    access: 'public',
    content: '# Setup guide\n\nOpen [Language support](/docs/getting-started#language-support) after onboarding.\n',
    headings: [
      {
        level: 1,
        text: 'Setup guide',
        id: 'setup-guide'
      }
    ],
    searchText: 'setup guide language support'
  },
  {
    slug: 'getting-started',
    title: 'Getting started',
    order: 20,
    section: 'Basics',
    sectionKey: 'docs.section.basics',
    summary: 'Fixture destination doc.',
    access: 'public',
    content: '# Getting started\n\n## Language support\n\nOperators can switch locales from the docs shell.\n',
    headings: [
      {
        level: 1,
        text: 'Getting started',
        id: 'getting-started'
      },
      {
        level: 2,
        text: 'Language support',
        id: 'language-support'
      }
    ],
    searchText: 'getting started language support'
  }
];

function createDocsFixturePlugin() {
  const virtualId = '\0docs-browser-fixture';
  const source = `export const APP_DOCS = ${JSON.stringify(FIXTURE_DOCS)};`;

  return {
    name: 'docs-browser-fixture',
    enforce: 'pre',
    resolveId(id) {
      if (
        id === '@/generated/docs-data' ||
        id === GENERATED_DOCS_PATH ||
        id.endsWith('/src/generated/docs-data.js')
      ) {
        return virtualId;
      }
      return null;
    },
    load(id) {
      const normalizedId = id.split('?')[0];
      if (
        normalizedId === virtualId ||
        normalizedId === GENERATED_DOCS_PATH ||
        normalizedId.endsWith('/src/generated/docs-data.js')
      ) {
        return source;
      }
      return null;
    }
  };
}

async function startFixtureServer() {
  return startBrowserTestServer({
    root: WEB_ROOT,
    extraPlugins: [createDocsFixturePlugin()]
  });
}

async function startAppServer() {
  return startBrowserTestServer({
    root: WEB_ROOT
  });
}

test('public docs shell preserves heading hashes for cross-doc links', async () => {
  const { server, baseUrl } = await startFixtureServer();
  const browser = await chromium.launch();
  const page = await browser.newPage();
  const pageErrors = [];

  page.on('pageerror', (error) => {
    pageErrors.push(error);
  });

  try {
    await page.goto(`${baseUrl}/docs/setup-guide`, { waitUntil: 'domcontentloaded' });
    await page.getByRole('heading', { name: 'Setup guide' }).waitFor();

    await page.getByRole('link', { name: 'Language support' }).click();
    await page.waitForURL((url) => {
      return url.pathname === '/docs/getting-started' && url.hash === '#language-support';
    });

    const currentUrl = new URL(page.url());
    assert.equal(currentUrl.pathname, '/docs/getting-started');
    assert.equal(currentUrl.hash, '#language-support');
    await page.getByRole('heading', { name: 'Getting started' }).waitFor();
    await page.getByText('Operators can switch locales from the docs shell.').waitFor();
    assert.deepEqual(pageErrors, []);
  } finally {
    await page.close();
    await browser.close();
    await server.close();
  }
});

test('setup guide renders the five-task flow and preserves the getting-started deep link', async () => {
  const { server, baseUrl } = await startAppServer();
  const browser = await chromium.launch();
  const page = await browser.newPage();
  const pageErrors = [];

  page.on('pageerror', (error) => {
    pageErrors.push(error);
  });

  try {
    await page.goto(`${baseUrl}/docs/setup-guide`, { waitUntil: 'domcontentloaded' });
    await page.getByRole('heading', { name: 'Setup guide' }).waitFor();
    await page.getByRole('heading', { name: 'Task 1: Change the admin password' }).waitFor();
    await page.getByRole('heading', { name: 'Task 5: Save the OCI launch target' }).waitFor();

    await page.getByRole('link', { name: 'Revisit Settings after setup' }).click();
    await page.waitForURL((url) => {
      return url.pathname === '/docs/getting-started' && url.hash === '#revisit-settings-after-setup';
    });

    const currentUrl = new URL(page.url());
    assert.equal(currentUrl.pathname, '/docs/getting-started');
    assert.equal(currentUrl.hash, '#revisit-settings-after-setup');
    await page.getByRole('heading', { name: 'Revisit Settings after setup' }).waitFor();
    assert.deepEqual(pageErrors, []);
  } finally {
    await page.close();
    await browser.close();
    await server.close();
  }
});
