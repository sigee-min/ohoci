import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';

import { collectAppDocs } from './generate-docs.mjs';

async function withTempDocs(files, fn) {
  const tempDir = await fs.mkdtemp(path.join(os.tmpdir(), 'ohoci-docs-'));
  try {
    for (const [name, content] of Object.entries(files)) {
      await fs.writeFile(path.join(tempDir, name), content, 'utf8');
    }
    await fn(tempDir);
  } finally {
    await fs.rm(tempDir, { recursive: true, force: true });
  }
}

test('collectAppDocs ignores unflagged docs and extracts stable headings', async () => {
  await withTempDocs({
    'guide.md': `---
app_docs: true
access: public
title: Start here
slug: start-here
order: 10
section: Basics
summary: Primary operator guide
---
# Start here

## Repeat
Body

## Repeat
More body
`,
    'internal.md': `# Internal only`
  }, async (docsDir) => {
    const docs = await collectAppDocs({ docsDir });
    assert.equal(docs.length, 1);
    assert.equal(docs[0].slug, 'start-here');
    assert.equal(docs[0].section, 'Basics');
    assert.equal(docs[0].sectionKey, 'docs.section.basics');
    assert.deepEqual(docs[0].headings, [
      { level: 1, text: 'Start here', id: 'start-here' },
      { level: 2, text: 'Repeat', id: 'repeat' },
      { level: 2, text: 'Repeat', id: 'repeat-2' }
    ]);
  });
});

test('collectAppDocs preserves ordering, raw section metadata, and section keys', async () => {
  await withTempDocs({
    'late.md': `---
app_docs: true
access: public
title: Late
slug: late
order: 30
section: Operations
---
# Late
`,
    'early.md': `---
app_docs: true
access: public
title: Early
slug: early
order: 10
section: Basics
---
# Early
`,
    'middle.md': `---
app_docs: true
access: public
title: Middle
slug: middle
order: 20
section: Basics
---
# Middle
`
  }, async (docsDir) => {
    const docs = await collectAppDocs({ docsDir });
    assert.deepEqual(
      docs.map((doc) => ({ slug: doc.slug, section: doc.section, sectionKey: doc.sectionKey })),
      [
        { slug: 'early', section: 'Basics', sectionKey: 'docs.section.basics' },
        { slug: 'middle', section: 'Basics', sectionKey: 'docs.section.basics' },
        { slug: 'late', section: 'Operations', sectionKey: 'docs.section.operations' }
      ]
    );
  });
});

test('collectAppDocs assigns the guides section key when frontmatter omits section', async () => {
  await withTempDocs({
    'guide.md': `---
app_docs: true
access: public
title: Guide
slug: guide
order: 10
---
# Guide
`
  }, async (docsDir) => {
    const docs = await collectAppDocs({ docsDir });
    assert.equal(docs[0].section, 'Guides');
    assert.equal(docs[0].sectionKey, 'docs.section.guides');
  });
});

test('collectAppDocs fails on duplicate slugs', async () => {
  await withTempDocs({
    'one.md': `---
app_docs: true
access: public
title: One
slug: duplicate
order: 10
---
# One
`,
    'two.md': `---
app_docs: true
access: public
title: Two
slug: duplicate
order: 20
---
# Two
`
  }, async (docsDir) => {
    await assert.rejects(() => collectAppDocs({ docsDir }), /Duplicate doc slug/);
  });
});

test('collectAppDocs fails on missing required frontmatter', async () => {
  await withTempDocs({
    'broken.md': `---
app_docs: true
access: public
title: Broken
order: 10
---
# Broken
`
  }, async (docsDir) => {
    await assert.rejects(() => collectAppDocs({ docsDir }), /Missing required frontmatter "slug"/);
  });
});

test('collectAppDocs rewrites flagged markdown links and rejects hidden doc links', async () => {
  await withTempDocs({
    'alpha.md': `---
app_docs: true
access: public
title: Alpha
slug: alpha
order: 10
---
Go to [Beta](./beta.md).
`,
    'beta.md': `---
app_docs: true
access: public
title: Beta
slug: beta
order: 20
---
# Beta
`,
    'hidden.md': `---
title: Hidden
slug: hidden
order: 30
---
# Hidden
`,
    'broken.md': `---
app_docs: true
access: public
title: Broken
slug: broken
order: 40
---
Go to [Hidden](./hidden.md).
`
  }, async (docsDir) => {
    const docs = await collectAppDocs({ docsDir: path.join(docsDir) }).catch((error) => error);
    assert.match(String(docs.message || docs), /unflagged or missing markdown file/);
  });
});

test('collectAppDocs rewrites links between flagged docs to docs routes', async () => {
  await withTempDocs({
    'alpha.md': `---
app_docs: true
access: public
title: Alpha
slug: alpha
order: 10
---
Go to [Beta](./beta.md#Deep Dive).
`,
    'beta.md': `---
app_docs: true
access: public
title: Beta
slug: beta
order: 20
---
# Beta

## Deep Dive
`
  }, async (docsDir) => {
    const docs = await collectAppDocs({ docsDir });
    assert.match(docs[0].content, /\[Beta\]\(\/docs\/beta#deep-dive\)/);
  });
});
