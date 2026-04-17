import test from 'node:test';
import assert from 'node:assert/strict';

import { buildDocsHref, buildDocsPath, createHeadingIdGenerator, parseDocsHref, parseDocsPath, slugifyHeading } from '../src/lib/docs.js';

test('buildDocsPath and parseDocsPath round-trip clean slugs', () => {
  assert.equal(buildDocsPath('setup-guide'), '/docs/setup-guide');
  assert.deepEqual(parseDocsPath('/docs/setup-guide'), { isDocsRoute: true, slug: 'setup-guide' });
  assert.deepEqual(parseDocsPath('/overview'), { isDocsRoute: false, slug: '' });
});

test('parseDocsHref extracts slug and heading id', () => {
  assert.deepEqual(parseDocsHref('/docs/setup-guide#runtime-target'), {
    slug: 'setup-guide',
    headingId: 'runtime-target'
  });
});

test('buildDocsHref preserves heading ids for cross-doc navigation', () => {
  assert.equal(buildDocsHref('setup-guide', 'runtime-target'), '/docs/setup-guide#runtime-target');
  assert.equal(buildDocsHref('setup-guide', '#runtime-target'), '/docs/setup-guide#runtime-target');
  assert.equal(buildDocsHref('setup-guide'), '/docs/setup-guide');
});

test('slugifyHeading and createHeadingIdGenerator stay stable', () => {
  const nextId = createHeadingIdGenerator();
  assert.equal(slugifyHeading('Runtime target'), 'runtime-target');
  assert.equal(nextId('Runtime target'), 'runtime-target');
  assert.equal(nextId('Runtime target'), 'runtime-target-2');
});
